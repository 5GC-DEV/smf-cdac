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
)

var (
	NRFCacheRemoveNfProfileFromNrfCache = nrfCache.RemoveNfProfileFromNrfCache
	SendRemoveSubscription              = consumer.SendRemoveSubscription
)

/*func HandleSMPolicyUpdateNotify(eventData interface{}) error {
	txn := eventData.(*transaction.Transaction)
	request := txn.Req.(models.SmPolicyNotification)
	smContext := txn.Ctxt.(*smfContext.SMContext)

	logger.PduSessLog.Infoln("In HandleSMPolicyUpdateNotify")
	pcfPolicyDecision := request.SmPolicyDecision

	if smContext.SMContextState != smfContext.SmStateActive {
		// Wait till the state becomes SmStateActive again
		// TODO: implement waiting in concurrent architecture
		logger.PduSessLog.Warnf("SMContext[%s-%02d] should be SmStateActive, but actual %s",
			smContext.Supi, smContext.PDUSessionID, smContext.SMContextState.String())
	}

	//TODO: Response data type -
	//[200 OK] UeCampingRep
	//[200 OK] array(PartialSuccessReport)
	//[400 Bad Request] ErrorReport

	// Derive QoS change(compare existing vs received Policy Decision)
	policyUpdates := qos.BuildSmPolicyUpdate(&smContext.SmPolicyData, pcfPolicyDecision)
	smContext.SmPolicyUpdates = append(smContext.SmPolicyUpdates, policyUpdates)

	httpResponse := httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	txn.Rsp = httpResponse

	// Form N1/N2 Msg based on QoS Change and Trigger N1/N2 Msg
	if err := BuildAndSendQosN1N2TransferMsg(smContext); err != nil {
		// smContext.CommitSmPolicyDecision(false)
		// Send error rsp to PCF
		httpResponse.Status = http.StatusBadRequest
		txn.Err = err
		return err
	}

	// Update UPF
	// TODO

	// Build `pfcpParam` using the dedicated function
	pfcpParam := BuildPfcpParam(smContext)

	// Send PFCP Session Modification Request
	if err := SendPfcpSessionModifyReq(smContext, pfcpParam); err != nil {
		logger.PduSessLog.Errorf("Failed to send PFCP session modification request: %v", err)
		httpResponse.Status = http.StatusInternalServerError
		txn.Err = err
		return err
	}

	// N1N2 and UPF update Success
	// Commit SM Policy Decision to SM Context
	// TODO
	// smContext.SMLock.Lock()
	// defer smContext.SMLock.Unlock()
	// smContext.CommitSmPolicyDecision(true)
	txn.Rsp = httpResponse
	return nil
}*/

func HandleSMPolicyUpdateNotify(eventData interface{}) error {
	txn := eventData.(*transaction.Transaction)
	request := txn.Req.(models.SmPolicyNotification)
	smContext := txn.Ctxt.(*smfContext.SMContext)

	logger.PduSessLog.Infoln("In HandleSMPolicyUpdateNotify")
	logger.PduSessLog.Infof("Received SM Policy Notification for SUPI[%s], PDU Session ID[%d]", smContext.Supi, smContext.PDUSessionID)
	logger.PduSessLog.Infof("Full SM Policy Notification: %+v", request)

	pcfPolicyDecision := request.SmPolicyDecision
	logger.PduSessLog.Infof("Extracted SM Policy Decision: %+v", pcfPolicyDecision)

	if smContext.SMContextState != smfContext.SmStateActive {
		logger.PduSessLog.Warnf("SMContext[%s-%02d] should be SmStateActive, but actual %s",
			smContext.Supi, smContext.PDUSessionID, smContext.SMContextState.String())
		// Note: Decision pending state machine design if needed
	}

	// Derive QoS change (compare existing vs received Policy Decision)
	policyUpdates := qos.BuildSmPolicyUpdate(&smContext.SmPolicyData, pcfPolicyDecision)
	smContext.SmPolicyUpdates = append(smContext.SmPolicyUpdates, policyUpdates)

	logger.PduSessLog.Infof("Derived SM Policy Updates for SMContext[%s-%02d]: %+v",
		smContext.Supi, smContext.PDUSessionID, policyUpdates)

	httpResponse := httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	txn.Rsp = httpResponse

	// Form N1/N2 Msg based on QoS Change and Trigger N1/N2 Msg
	logger.PduSessLog.Infof("Triggering N1/N2 Transfer for SMContext[%s-%02d]", smContext.Supi, smContext.PDUSessionID)
	if err := BuildAndSendQosN1N2TransferMsg(smContext); err != nil {
		logger.PduSessLog.Errorf("Failed to send N1/N2 Transfer Message: %v", err)
		httpResponse.Status = http.StatusBadRequest
		txn.Err = err
		return err
	}
	logger.PduSessLog.Infof("Successfully sent N1/N2 Transfer Message for SMContext[%s-%02d]", smContext.Supi, smContext.PDUSessionID)

	// Build PFCP parameters
	pfcpParam := BuildPfcpParam(smContext)
	logger.PduSessLog.Infof("Built PFCP Parameters for SMContext[%s-%02d]: %+v", smContext.Supi, smContext.PDUSessionID, pfcpParam)

	// Send PFCP Session Modification Request
	logger.PduSessLog.Infof("Sending PFCP Session Modification Request for SMContext[%s-%02d]", smContext.Supi, smContext.PDUSessionID)
	if err := SendPfcpSessionModifyReq(smContext, pfcpParam); err != nil {
		logger.PduSessLog.Errorf("Failed to send PFCP session modification request: %v", err)
		httpResponse.Status = http.StatusInternalServerError
		txn.Err = err
		return err
	}
	logger.PduSessLog.Infof("Successfully sent PFCP Session Modification Request for SMContext[%s-%02d]", smContext.Supi, smContext.PDUSessionID)

	// Finalize
	logger.PduSessLog.Infof("SM Policy Update handled successfully for SMContext[%s-%02d]", smContext.Supi, smContext.PDUSessionID)
	txn.Rsp = httpResponse
	return nil
}

func BuildPfcpParam(smContext *smfContext.SMContext) *pfcpParam {
	pfcpParam := &pfcpParam{
		pdrList: []*smf_context.PDR{},
		farList: []*smf_context.FAR{},
		barList: []*smf_context.BAR{},
		qerList: []*smf_context.QER{},
	}

	smContext.PendingUPF = make(smfContext.PendingUPF)
	var pdrList []*smf_context.PDR
	var farList []*smf_context.FAR

	logger.PduSessLog.Infof("SMContext: %v", smContext)
	logger.PduSessLog.Infof("SMContext SmPolicyUpdates: %v", smContext.SmPolicyUpdates)

	// Iterate over the Data Path Pool
	for _, dataPath := range smContext.Tunnel.DataPathPool {
		if dataPath.Activated {
			ANUPF := dataPath.FirstDPNode
			for _, DLPDR := range ANUPF.DownLinkTunnel.PDR {
				// Update FAR actions
				DLPDR.FAR.ApplyAction = smfContext.ApplyAction{Buff: false, Drop: false, Dupl: false, Forw: true, Nocp: false}
				DLPDR.FAR.ForwardingParameters = &smfContext.ForwardingParameters{
					OuterHeaderCreation: DLPDR.FAR.ForwardingParameters.OuterHeaderCreation,
					DestinationInterface: smfContext.DestinationInterface{
						InterfaceValue: smfContext.DestinationInterfaceAccess,
					},
					NetworkInstance: []byte(smContext.Dnn),
				}

				// Mark rules as updated
				DLPDR.State = smfContext.RULE_UPDATE
				DLPDR.FAR.State = smfContext.RULE_UPDATE

				// Append to lists
				pdrList = append(pdrList, DLPDR)
				farList = append(farList, DLPDR.FAR)

				// Track UPF
				if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
					smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				}
			}
		}
	}

	// Append to pfcpParam
	pfcpParam.pdrList = append(pfcpParam.pdrList, pdrList...)
	pfcpParam.farList = append(pfcpParam.farList, farList...)

	return pfcpParam
}

/*
	func BuildAndSendQosN1N2TransferMsg(smContext *smfContext.SMContext) error {
		// N1N2 Request towards AMF
		n1n2Request := models.N1N2MessageTransferRequest{}

		// N2 Container Info
		n2InfoContainer := models.N2InfoContainer{
			N2InformationClass: models.N2InformationClass_SM,
			SmInfo: &models.N2SmInformation{
				PduSessionId: smContext.PDUSessionID,
				N2InfoContent: &models.N2InfoContent{
					NgapIeType: models.NgapIeType_PDU_RES_SETUP_REQ,
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
*/
func BuildAndSendQosN1N2TransferMsg(smContext *smfContext.SMContext) error {
	smContext.SubPduSessLog.Infof("Starting QoS N1N2 Transfer for SUPI[%s], PDU Session ID[%d]", smContext.Supi, smContext.PDUSessionID)

	n1n2Request := models.N1N2MessageTransferRequest{}

	// N2 Container Info
	n2InfoContainer := models.N2InfoContainer{
		N2InformationClass: models.N2InformationClass_SM,
		SmInfo: &models.N2SmInformation{
			PduSessionId: smContext.PDUSessionID,
			N2InfoContent: &models.N2InfoContent{
				NgapIeType: models.NgapIeType_PDU_RES_SETUP_REQ,
				NgapData: &models.RefToBinaryData{
					ContentId: "N2SmInformation",
				},
			},
			SNssai: smContext.Snssai,
		},
	}
	smContext.SubPduSessLog.Infof("Built N2InfoContainer: %+v", n2InfoContainer)

	// N1 Container Info
	n1MsgContainer := models.N1MessageContainer{
		N1MessageClass:   "SM",
		N1MessageContent: &models.RefToBinaryData{ContentId: "GSM_NAS"},
	}

	// N1N2 Json Data
	n1n2Request.JsonData = &models.N1N2MessageTransferReqData{
		PduSessionId: smContext.PDUSessionID,
	}
	smContext.SubPduSessLog.Infof("Initialized N1N2MessageTransferReqData: %+v", n1n2Request.JsonData)

	// N1 Message
	smContext.SubPduSessLog.Infof("Building N1 GSM NAS message for SUPI[%s], PDU Session ID[%d]", smContext.Supi, smContext.PDUSessionID)
	if smNasBuf, err := smfContext.BuildGSMPDUSessionModificationCommand(smContext); err != nil {
		logger.PduSessLog.Errorf("BuildGSMPDUSessionModificationCommand failed: %s", err)
	} else {
		n1n2Request.BinaryDataN1Message = smNasBuf
		n1n2Request.JsonData.N1MessageContainer = &n1MsgContainer
		smContext.SubPduSessLog.Info("GSM NAS message built and added to N1N2 request")
	}

	// N2 Message
	smContext.SubPduSessLog.Infof("Building N2 PDU Session Resource Modify Request Transfer for SUPI[%s], PDU Session ID[%d]", smContext.Supi, smContext.PDUSessionID)
	n2Pdu, err := smfContext.BuildPDUSessionResourceModifyRequestTransfer(smContext)
	if err != nil {
		smContext.SubPduSessLog.Errorf("BuildPDUSessionResourceModifyRequestTransfer failed: %s", err.Error())
	} else {
		n1n2Request.BinaryDataN2Information = n2Pdu
		n1n2Request.JsonData.N2InfoContainer = &n2InfoContainer
		smContext.SubPduSessLog.Info("N2 message built and added to N1N2 request")
	}

	// Send to AMF
	smContext.SubPduSessLog.Infoln("Sending N1N2 Transfer message to AMF")
	rspData, _, err := smContext.CommunicationClient.
		N1N2MessageCollectionDocumentApi.
		N1N2MessageTransfer(context.Background(), smContext.Supi, n1n2Request)
	if err != nil {
		smContext.SubPfcpLog.Warnf("N1N2 Transfer failed for SUPI[%s], PDU Session ID[%d]: %v", smContext.Supi, smContext.PDUSessionID, err)
		return err
	}

	smContext.SubPduSessLog.Infof("Received N1N2 Transfer response: %+v", rspData)

	if rspData.Cause == models.N1N2MessageTransferCause_N1_MSG_NOT_TRANSFERRED {
		smContext.SubPfcpLog.Errorf("N1N2MessageTransfer failure for SUPI[%s], PDU Session ID[%d]: %v", smContext.Supi, smContext.PDUSessionID, rspData.Cause)
		return fmt.Errorf("N1N2MessageTransfer failure: %v", rspData.Cause)
	}

	smContext.SubPduSessLog.Infof("QoS N1N2 Transfer completed successfully for SUPI[%s], PDU Session ID[%d]", smContext.Supi, smContext.PDUSessionID)
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
