// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/omec-project/openapi/models"
	nrfCache "github.com/omec-project/openapi/nrfcache"
	"github.com/omec-project/smf/consumer"
	smfContext "github.com/omec-project/smf/context"
	smf_context "github.com/omec-project/smf/context"
	"github.com/omec-project/smf/logger"
	"github.com/omec-project/smf/qos"
	"github.com/omec-project/smf/transaction"
	"github.com/omec-project/util/httpwrapper"
	"github.com/omec-project/util/util_3gpp"
)

var (
	NRFCacheRemoveNfProfileFromNrfCache = nrfCache.RemoveNfProfileFromNrfCache
	SendRemoveSubscription              = consumer.SendRemoveSubscription
)

func HandleSMPolicyUpdateNotify(eventData interface{}) error {
	txn := eventData.(*transaction.Transaction)
	request := txn.Req.(models.SmPolicyNotification)
	smContext := txn.Ctxt.(*smfContext.SMContext)

	smContext.SMLock.Lock()
	defer smContext.SMLock.Unlock()

	logger.PduSessLog.Infoln("In HandleSMPolicyUpdateNotify")
	pcfPolicyDecision := request.SmPolicyDecision

	if smContext.SMContextState != smf_context.SmStateActive {
		logger.PduSessLog.Warnf("SMContext[%s-%02d] should be SmStateActive, but actual %s",
			smContext.Supi, smContext.PDUSessionID, smContext.SMContextState.String())
	}

	// Derive QoS change
	logger.PduSessLog.Infof("Building SM Policy Update for UE [%s], PDU Session ID [%d]",
		smContext.Supi, smContext.PDUSessionID)

	policyUpdates := qos.BuildSmPolicyUpdate(&smContext.SmPolicyData, pcfPolicyDecision)

	logger.PduSessLog.Infof("SM Policy Update built: %+v", policyUpdates)

	smContext.SmPolicyUpdates = append(smContext.SmPolicyUpdates[:0], policyUpdates)
	logger.PduSessLog.Infof("Appended SM Policy Update, total updates count: %d",
		len(smContext.SmPolicyUpdates))
	logger.PduSessLog.Infof("SmPolicyUpdates: %v", smContext.SmPolicyUpdates)

	// Set state to PFCP Modify before sending PFCP request
	smContext.ChangeState(smf_context.SmStatePfcpModify)
	var response models.UpdateSmContextResponse
	response.JsonData = new(models.SmContextUpdatedData)
	// Build PFCP parameters
	pfcpParam := BuildPfcpParam(smContext)

	if err := SendPfcpSessionModifyReq(smContext, pfcpParam); err != nil {
		// PFCP modify failed — revert state and return error
		smContext.SubCtxLog.Errorf("PFCP session modify error: %v", err)
		// smContext.ChangeState(prevState)
		logger.PduSessLog.Infof("SMContext[%s-%02d] state reverted to %s after PFCP error",
			smContext.Supi, smContext.PDUSessionID, smContext.SMContextState.String())

		// Build HTTP error response for the original transaction
		httpResponse := makePduCtxtModifyErrRsp(smContext, err.Error())
		txn.Err = err
		txn.Rsp = httpResponse
		return err
	}

	smContext.SubCtxLog.Infoln("SMContextState Change State:", smContext.SMContextState.String())
	logger.PduSessLog.Infof("PFCP modify successful for UE [%s], PDU Session ID [%d]",
		smContext.Supi, smContext.PDUSessionID)

	// Now send N1/N2 Msg after PFCP success
	if err := BuildAndSendQosN1N2TransferMsg(smContext); err != nil {
		logger.PduSessLog.Errorf("Failed to build/send N1/N2 QoS transfer message: %v", err)
		txn.Err = err
		return err
	}

	// Set response and change state to active
	smContext.ChangeState(smf_context.SmStateActive)
	smContext.SubCtxLog.Info("PFCP Modify success and N1N2 Msg sent, new state:", smContext.SMContextState.String())

	httpResponse := &httpwrapper.Response{
		Status: http.StatusOK,
		Body:   nil,
	}
	txn.Rsp = httpResponse

	return nil
}

func BuildPfcpParam(smContext *smfContext.SMContext) *pfcpParam {
	pfcpParam := &pfcpParam{
		pdrList:   []*smfContext.PDR{},
		farList:   []*smfContext.FAR{},
		barList:   []*smfContext.BAR{},
		qerList:   []*smfContext.QER{},
		removePDR: []*smfContext.PDR{},
		removeFAR: []*smfContext.FAR{},
		removeQER: []*smfContext.QER{},
	}

	smContext.PendingUPF = make(smfContext.PendingUPF)

	shouldSendReleaseOnly := false
	ruleid := "0"
	if len(smContext.SmPolicyUpdates) > 0 && smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules != nil {
		if len(smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
		} else {
			for ruleId, rule := range smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules {
				logger.PduSessLog.Infof("[BuildPfcpParam] Checking PCC RuleId=%s, Rule=%+v", ruleId, rule)
				ruleid = ruleId
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					break
				}
			}
		}
	}
	logger.PduSessLog.Infof("[BuildPfcpParam] Checking PCC RuleId=%s", ruleid)
	for dpIndex, dataPath := range smContext.Tunnel.DataPathPool {
		logger.PduSessLog.Infof("[BuildPfcpParam] Processing DataPath[%d], Activated=%v", dpIndex, dataPath.Activated)
		if !dataPath.Activated {
			logger.PduSessLog.Infof("Skipping inactive DataPath: %+v", dataPath)
			continue
		}

		ANUPF := dataPath.FirstDPNode
		var dedQER *smf_context.QER
		var err error
		logger.PduSessLog.Infof("Processing DataPath with UPF Node: %s", ANUPF.GetNodeIP())
		if !shouldSendReleaseOnly {
			dedQER, err = ANUPF.CreateDedicatedQosQer(smContext)
			if err != nil {
				logger.PduSessLog.Warnf("[BuildPfcpParam] CreateSessRuleQer failed: %v", err)
			} else {
				logger.PduSessLog.Infof("[BuildPfcpParam] Created default QER: %+v", dedQER)
			}

			if err := dataPath.ActivateUlDlTunnel(smContext); err != nil {
				logger.PduSessLog.Errorf("activate UL/DL tunnel error %v", err.Error())
			}
		}
		// ----------------------
		// Handle Downlink PDRs
		// ----------------------
		if dlPDR, ok := ANUPF.DownLinkTunnel.PDR[ruleid]; ok {
			logger.PduSessLog.Infof("[BuildPfcpParam] Checking DL PDR: Name=%s, ID=%d", ruleid, dlPDR.PDRID)
			if shouldSendReleaseOnly {
				logger.PduSessLog.Infof("[BuildPfcpParam] Marking DL PDR[%s] for removal", ruleid)
				pfcpParam.removePDR = append(pfcpParam.removePDR, dlPDR)
				if dlPDR.FAR != nil {
					pfcpParam.removeFAR = append(pfcpParam.removeFAR, dlPDR.FAR)
				}
				if dlPDR.QER != nil {
					for _, qer := range dlPDR.QER {
						if qer != nil {
							logger.PduSessLog.Infof(
								"[BuildPfcpParam] UL PDR[%s] has QER ID [%d], QFI=%d, State=%v",
								ruleid, qer.QERID, qer.QFI, qer.State,
							)
						}
					}
					pfcpParam.removeQER = append(pfcpParam.removeQER, dlPDR.QER...)
				}
				continue
			}

			logger.CtxLog.Infof("activate Downlink PDR[%v]:[%v]", ruleid, dlPDR)
			dlPDR.QER = []*smf_context.QER{dedQER}

			logger.PduSessLog.Infof("[BuildPfcpParam] Replaced DL PDR[%s] QERs with new QER: %+v", ruleid, dlPDR)

			if dlPDR.Precedence == 0 {
				dlPDR.Precedence = 1
			}
			dlPDR.PDI.SourceInterface = smf_context.SourceInterface{InterfaceValue: smf_context.SourceInterfaceCore}
			dlPDR.PDI.NetworkInstance = util_3gpp.Dnn(smContext.Dnn)
			logger.PduSessLog.Infof("[BuildPfcpParam] Final DL PDR[%s]: %+v", ruleid, dlPDR)

			dlFAR := dlPDR.FAR

			// FAR ApplyAction
			dlFAR.ApplyAction = smf_context.ApplyAction{
				Buff: true,
				Drop: false,
				Dupl: false,
				Forw: false,
				Nocp: true,
			}

			// Interface resolution
			logger.PduSessLog.Infof("Resolving UPF interface for DNN [%s], type [N6]", smContext.Dnn)

			pfcpParam.pdrList = append(pfcpParam.pdrList, dlPDR)
			if dlFAR != nil {
				pfcpParam.farList = append(pfcpParam.farList, dlFAR)
			}
			if dedQER != nil {
				pfcpParam.qerList = append(pfcpParam.qerList, dedQER)
			}

			smContext.PendingUPF[ANUPF.GetNodeIP()] = true
		}

		// ----------------------
		// Handle Uplink PDRs
		// ----------------------
		if ulPDR, ok := ANUPF.UpLinkTunnel.PDR[ruleid]; ok {
			if shouldSendReleaseOnly {
				logger.PduSessLog.Infof("[BuildPfcpParam] Marking UL PDR[%s] for removal", ruleid)
				pfcpParam.removePDR = append(pfcpParam.removePDR, ulPDR)
				if ulPDR.FAR != nil {
					pfcpParam.removeFAR = append(pfcpParam.removeFAR, ulPDR.FAR)
				}
				if ulPDR.QER != nil {
					for _, qer := range ulPDR.QER {
						if qer != nil {
							logger.PduSessLog.Infof(
								"[BuildPfcpParam] UL PDR[%s] has QER ID [%d], QFI=%d, State=%v",
								ruleid, qer.QERID, qer.QFI, qer.State,
							)
						}
					}
					pfcpParam.removeQER = append(pfcpParam.removeQER, ulPDR.QER...)
				}
				continue
			}
			ulPDR.QER = []*smf_context.QER{dedQER}
			logger.PduSessLog.Infof("[BuildPfcpParam] Replaced DL PDR[%s] QERs with new QER: %+v", ruleid, ulPDR)
			if ulPDR.Precedence == 0 {
				ulPDR.Precedence = 1
			}
			ulPDR.PDI.SourceInterface = smf_context.SourceInterface{InterfaceValue: smf_context.SourceInterfaceAccess}
			ulPDR.PDI.LocalFTeid = &smf_context.FTEID{Ch: true}
			ulPDR.PDI.NetworkInstance = util_3gpp.Dnn(smContext.Dnn)
			ulPDR.OuterHeaderRemoval = &smf_context.OuterHeaderRemoval{
				OuterHeaderRemovalDescription: smf_context.OuterHeaderRemovalGtpUUdpIpv4,
			}
			ulFAR := ulPDR.FAR
			ulFAR.ApplyAction = smf_context.ApplyAction{Forw: true}
			ulFAR.ForwardingParameters = &smf_context.ForwardingParameters{
				DestinationInterface: smf_context.DestinationInterface{
					InterfaceValue: smf_context.DestinationInterfaceCore,
				},
				NetworkInstance: []byte(smContext.Dnn),
			}

			pfcpParam.pdrList = append(pfcpParam.pdrList, ulPDR)
			if ulFAR != nil {
				pfcpParam.farList = append(pfcpParam.farList, ulFAR)
			}
			smContext.PendingUPF[ANUPF.GetNodeIP()] = true
			logger.CtxLog.Infof("activate UpLink PDR[%v]:[%v]", ruleid, ulPDR)
		}
	}

	return pfcpParam
}

func BuildAndSendQosN1N2TransferMsg(smContext *smfContext.SMContext) error {
	// N1N2 Request towards AMF
	n1n2Request := models.N1N2MessageTransferRequest{}

	// N2 Container Info
	n2InfoContainer := models.N2InfoContainer{
		N2InformationClass: models.N2InformationClass_SM,
		SmInfo: &models.N2SmInformation{
			PduSessionId: smContext.PDUSessionID,
			N2InfoContent: &models.N2InfoContent{
				NgapIeType: models.NgapIeType_PDU_RES_MOD_REQ,
				NgapData: &models.RefToBinaryData{
					ContentId: "N2SmInformation",
				},
			},
			SNssai: smContext.Snssai,
		},
	}

	// N1 Container Info
	n1MsgContainer := models.N1MessageContainer{
		N1MessageClass:   "SM",
		N1MessageContent: &models.RefToBinaryData{ContentId: "GSM_NAS"},
	}

	// N1N2 Json Data
	n1n2Request.JsonData = &models.N1N2MessageTransferReqData{PduSessionId: smContext.PDUSessionID}

	// N1 Msg
	if smNasBuf, err := smfContext.BuildGSMPDUSessionModificationCommand(smContext); err != nil {
		logger.PduSessLog.Errorf("build GSM BuildGSMPDUSessionModificationCommand failed: %s", err)
	} else {
		n1n2Request.BinaryDataN1Message = smNasBuf
		n1n2Request.JsonData.N1MessageContainer = &n1MsgContainer
	}

	// N2 Msg
	n2Pdu, err := smfContext.BuildPDUSessionResourceModifyRequestTransfer(smContext)
	if err != nil {
		smContext.SubPduSessLog.Errorf("SMPolicyUpdate, build PDUSession Resource Modify Request Transfer Error(%s)", err.Error())
	} else {
		n1n2Request.BinaryDataN2Information = n2Pdu
		n1n2Request.JsonData.N2InfoContainer = &n2InfoContainer
	}

	smContext.SubPduSessLog.Infoln("QoS N1N2 transfer initiated")
	rspData, _, err := smContext.
		CommunicationClient.
		N1N2MessageCollectionDocumentApi.
		N1N2MessageTransfer(context.Background(), smContext.Supi, n1n2Request)
	if err != nil {
		smContext.SubPfcpLog.Warnf("send N1N2Transfer failed, %v", err.Error())
		return err
	}
	if rspData.Cause == models.N1N2MessageTransferCause_N1_MSG_NOT_TRANSFERRED {
		smContext.SubPfcpLog.Errorf("N1N2MessageTransfer failure, %v", rspData.Cause)
		return fmt.Errorf("N1N2MessageTransfer failure, %v", rspData.Cause)
	}
	smContext.SubPduSessLog.Infoln("QoS N1N2 Transfer completed")
	return nil
}

func HandleNfSubscriptionStatusNotify(request *httpwrapper.Request) *httpwrapper.Response {
	logger.PduSessLog.Debugln("[SMF] Handle NF Status Notify")

	notificationData := request.Body.(models.NotificationData)

	problemDetails := NfSubscriptionStatusNotifyProcedure(notificationData)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

// NfSubscriptionStatusNotifyProcedure is handler method of notification procedure.
// According to event type retrieved in the notification data, it performs some actions.
// For example, if event type is deregistered, it deletes cached NF profile and performs an NF discovery.
func NfSubscriptionStatusNotifyProcedure(notificationData models.NotificationData) *models.ProblemDetails {
	logger.ProducerLog.Debugf("NfSubscriptionStatusNotify: %+v", notificationData)

	if notificationData.Event == "" || notificationData.NfInstanceUri == "" {
		problemDetails := &models.ProblemDetails{
			Status: http.StatusBadRequest,
			Cause:  "MANDATORY_IE_MISSING", // Defined in TS 29.510 6.1.6.2.17
			Detail: "Missing IE [Event]/[NfInstanceUri] in NotificationData",
		}
		return problemDetails
	}
	nfInstanceId := notificationData.NfInstanceUri[strings.LastIndex(notificationData.NfInstanceUri, "/")+1:]

	logger.ProducerLog.Infof("Received Subscription Status Notification from NRF: %v", notificationData.Event)
	// If nrf caching is enabled, go ahead and delete the entry from the cache.
	// This will force the PCF to do nf discovery and get the updated nf profile from the NRF.
	if notificationData.Event == models.NotificationEventType_DEREGISTERED {
		if smfContext.SMF_Self().EnableNrfCaching {
			ok := NRFCacheRemoveNfProfileFromNrfCache(nfInstanceId)
			logger.ProducerLog.Debugf("nfinstance %v deleted from cache: %v", nfInstanceId, ok)
		}
		if subscriptionId, ok := smfContext.SMF_Self().NfStatusSubscriptions.Load(nfInstanceId); ok {
			logger.ConsumerLog.Debugf("SubscriptionId of nfInstance %v is %v", nfInstanceId, subscriptionId.(string))
			problemDetails, err := SendRemoveSubscription(subscriptionId.(string))
			if problemDetails != nil {
				logger.ConsumerLog.Errorf("Remove NF Subscription Failed Problem[%+v]", problemDetails)
			} else if err != nil {
				logger.ConsumerLog.Errorf("Remove NF Subscription Error[%+v]", err)
			} else {
				logger.ConsumerLog.Infoln("Remove NF Subscription successful")
				smfContext.SMF_Self().NfStatusSubscriptions.Delete(nfInstanceId)
			}
		} else {
			logger.ProducerLog.Infof("nfinstance %v not found in map", nfInstanceId)
		}
	}

	return nil
}
