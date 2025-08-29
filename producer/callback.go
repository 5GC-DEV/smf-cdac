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

/*
	func HandleSMPolicyUpdateNotify(eventData interface{}) error {
		txn := eventData.(*transaction.Transaction)
		request := txn.Req.(models.SmPolicyNotification)
		smContext := txn.Ctxt.(*smfContext.SMContext)

		smContext.SMLock.Lock()
		defer smContext.SMLock.Unlock()

		//smContext.ChangeState(smf_context.SmStatePfcpModify)
		smContext.SubCtxLog.Infof("SMContextState Change State:", smContext.SMContextState.String())
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, send PFCP Modification")

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

		// Build `pfcpParam` using the dedicated function
		pfcpParam := BuildPfcpParam(smContext)

		// Send PFCP Session Modification Request
		if err := SendPfcpSessionModifyReq(smContext, pfcpParam); err != nil {
			logger.PduSessLog.Errorf("Failed to send PFCP session modification request: %v", err)
			httpResponse.Status = http.StatusInternalServerError
			txn.Err = err
			return err
		} else {
			httpResponse = &httpwrapper.Response{
				Status: http.StatusOK,
				Body:   nil,
			}
		}
		txn.Rsp = httpResponse

		// Update state
		smContext.ChangeState(smf_context.SmStateActive)
		smContext.SubCtxLog.Info("PFCP Modify success, new state:", smContext.SMContextState.String())

		return nil

		// N1N2 and UPF update Success
		// Commit SM Policy Decision to SM Context
		// TODO
		//smContext.SMLock.Lock()
		//defer smContext.SMLock.Unlock()
		//smContext.CommitSmPolicyDecision(true)
		//txn.Rsp = httpResponse
		// return nil
	}
*/
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

	// isAppSessionDelete := false
	for ruleID, rule := range pcfPolicyDecision.PccRules {
		if rule == nil {
			logger.PduSessLog.Infof("PCF requested deletion of PCC Rule %s", ruleID)
			// isAppSessionDelete = true
		}
	}
	for tcID, tc := range pcfPolicyDecision.TraffContDecs {
		if tc == nil {
			logger.PduSessLog.Infof("PCF requested deletion of Traffic Control Decision %s", tcID)
			// isAppSessionDelete = true
		}
	}

	/* if isAppSessionDelete {
		// Handle PDU session release instead of PFCP modify
		if err := HandleSmPolicyTermination(smContext); err != nil {
			txn.Err = err
			return err
		}
		httpResponse := &httpwrapper.Response{
			Status: http.StatusOK,
			Body:   nil,
		}
		txn.Rsp = httpResponse
		return nil
	} */

	// Derive QoS change
	logger.PduSessLog.Infof("Building SM Policy Update for UE [%s], PDU Session ID [%d]",
		smContext.Supi, smContext.PDUSessionID)

	policyUpdates := qos.BuildSmPolicyUpdate(&smContext.SmPolicyData, pcfPolicyDecision)

	logger.PduSessLog.Infof("SM Policy Update built: %+v", policyUpdates)

	// smContext.SmPolicyUpdates = append(smContext.SmPolicyUpdates, policyUpdates)
	smContext.SmPolicyUpdates = append(smContext.SmPolicyUpdates[:0], policyUpdates)
	logger.PduSessLog.Infof("Appended SM Policy Update, total updates count: %d",
		len(smContext.SmPolicyUpdates))

	// Set state to PFCP Modify before sending PFCP request
	smContext.ChangeState(smf_context.SmStatePfcpModify)

	// Build PFCP parameters
	pfcpParam := BuildPfcpParam(smContext)

	// Send PFCP Session Modification Request
	if err := SendPfcpSessionModifyReq(smContext, pfcpParam); err != nil {
		logger.PduSessLog.Errorf("Failed to send PFCP session modification request: %v", err)
		txn.Err = err
		return err
	}

	// Now send N1/N2 Msg after PFCP success
	if err := BuildAndSendQosN1N2TransferMsg(smContext); err != nil {
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

/*func BuildPfcpParam(smContext *smfContext.SMContext) *pfcpParam {
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
} */

/*func BuildPfcpParam(smContext *smfContext.SMContext) *pfcpParam {
	pfcpParam := &pfcpParam{
		pdrList:   []*smf_context.PDR{},
		farList:   []*smf_context.FAR{},
		barList:   []*smf_context.BAR{},
		qerList:   []*smf_context.QER{},
		removePDR: []*smf_context.PDR{}, // Add for teardown
		removeFAR: []*smf_context.FAR{},
		removeQER: []*smf_context.QER{},
	}

	smContext.PendingUPF = make(smfContext.PendingUPF)

	logger.PduSessLog.Infof("SMContext: %v", smContext)
	logger.PduSessLog.Infof("SMContext SmPolicyUpdates: %v", smContext.SmPolicyUpdates)

	shouldSendReleaseOnly := false
	if len(smContext.SmPolicyUpdates) > 0 && smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules != nil {
		// Check if PccRules map is empty or contains nil/empty rule IDs
		if len(smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
		} else {
			// Check if any PCC rule has nil or empty ID
			for ruleId, rule := range smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules {
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					break
				}
			}
		}
	}

	for _, dataPath := range smContext.Tunnel.DataPathPool {
		if !dataPath.Activated {
			continue
		}

		ANUPF := dataPath.FirstDPNode
		for _, dlPDR := range ANUPF.DownLinkTunnel.PDR {
			// If PCC rule is nil, this is a signal to remove rules
			if shouldSendReleaseOnly == true {
				logger.PduSessLog.Infof("Removing PDR ID: %v", dlPDR.PDRID)

				pfcpParam.removePDR = append(pfcpParam.removePDR, dlPDR)
				if dlPDR.FAR != nil {
					pfcpParam.removeFAR = append(pfcpParam.removeFAR, dlPDR.FAR)
				}
				if dlPDR.QER != nil {
					pfcpParam.removeQER = append(pfcpParam.removeQER, dlPDR.QER...)
				}
				continue
			}

			// Otherwise, treat it as update (existing logic)
			dlPDR.FAR.ApplyAction = smfContext.ApplyAction{
				Buff: false, Drop: false, Dupl: false, Forw: true, Nocp: false,
			}
			dlPDR.FAR.ForwardingParameters = &smfContext.ForwardingParameters{
				OuterHeaderCreation: dlPDR.FAR.ForwardingParameters.OuterHeaderCreation,
				DestinationInterface: smfContext.DestinationInterface{
					InterfaceValue: smfContext.DestinationInterfaceAccess,
				},
				NetworkInstance: []byte(smContext.Dnn),
			}

			dlPDR.State = smfContext.RULE_UPDATE
			dlPDR.FAR.State = smfContext.RULE_UPDATE

			pfcpParam.pdrList = append(pfcpParam.pdrList, dlPDR)
			pfcpParam.farList = append(pfcpParam.farList, dlPDR.FAR)

			if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
				smContext.PendingUPF[ANUPF.GetNodeIP()] = true
			}
		}
	}

	return pfcpParam
} */

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

	// Reset PendingUPF for new build
	smContext.PendingUPF = make(smfContext.PendingUPF)

	logger.PduSessLog.Infof("SMContext: %v", smContext)
	logger.PduSessLog.Infof("SmPolicyUpdates: %v", smContext.SmPolicyUpdates)

	// Check if only release should be sent
	shouldSendReleaseOnly := false
	if len(smContext.SmPolicyUpdates) > 0 && smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules != nil {
		if len(smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
		} else {
			for ruleId, rule := range smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules {
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					break
				}
			}
		}
	}
	/*if len(smContext.SmPolicyUpdates) > 1 && smContext.SmPolicyUpdates[1].SmPolicyDecision.PccRules != nil {
		// Case when there are at least 2 updates
		if len(smContext.SmPolicyUpdates[1].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
		} else {
			for ruleId, rule := range smContext.SmPolicyUpdates[0].SmPolicyDecision.PccRules {
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					break
				}
			}
		}
	} */

	// Iterate over datapaths
	for _, dataPath := range smContext.Tunnel.DataPathPool {
		if !dataPath.Activated {
			logger.PduSessLog.Infof("Skipping inactive DataPath: %+v", dataPath)
			continue
		}

		ANUPF := dataPath.FirstDPNode
		logger.PduSessLog.Infof("Processing DataPath with UPF Node: %s", ANUPF.GetNodeIP())

		for _, dlPDR := range ANUPF.DownLinkTunnel.PDR {
			if shouldSendReleaseOnly {
				// Mark PDR/FAR/QER for removal
				logger.PduSessLog.Infof("[ReleaseOnly] Removing PDR ID: %v", dlPDR.PDRID)

				pfcpParam.removePDR = append(pfcpParam.removePDR, dlPDR)
				if dlPDR.FAR != nil {
					logger.PduSessLog.Infof("[ReleaseOnly] Removing FAR ID: %v", dlPDR.FAR.FARID)
					pfcpParam.removeFAR = append(pfcpParam.removeFAR, dlPDR.FAR)
				}
				if dlPDR.QER != nil {
					for _, q := range dlPDR.QER {
						logger.PduSessLog.Infof("[ReleaseOnly] Removing QER ID: %v", q.QERID)
					}
					pfcpParam.removeQER = append(pfcpParam.removeQER, dlPDR.QER...)
				}
				continue
			}

			// ✅ FAR updates
			applyAction := smfContext.ApplyAction{Forw: true}
			if smContext.SmPolicyUpdates != nil &&
				len(smContext.SmPolicyUpdates) > 0 &&
				smContext.SmPolicyUpdates[0].SmPolicyDecision != nil {
				logger.PduSessLog.Debug("SmPolicyDecision present, mapping actions to FAR (TODO)")
			}

			dlPDR.FAR.ApplyAction = applyAction
			dlPDR.FAR.ForwardingParameters = &smfContext.ForwardingParameters{
				OuterHeaderCreation: dlPDR.FAR.ForwardingParameters.OuterHeaderCreation,
				DestinationInterface: smfContext.DestinationInterface{
					InterfaceValue: smfContext.DestinationInterfaceAccess,
				},
				NetworkInstance: []byte(smContext.Dnn),
			}
			logger.PduSessLog.Infof("Updated FAR for PDR ID [%d], DestinationInterface=Access, DNN=%s",
				dlPDR.PDRID, smContext.Dnn)

			// ✅ Update states
			oldPdrState := dlPDR.State
			/* if dlPDR.State == smfContext.RULE_INITIAL {
				dlPDR.State = smfContext.RULE_CREATE
			} else {
				dlPDR.State = smfContext.RULE_UPDATE
			}*/
			dlPDR.State = smfContext.RULE_INITIAL
			logger.PduSessLog.Infof("PDR ID [%d] state changed from %v → %v", dlPDR.PDRID, oldPdrState, dlPDR.State)

			oldFarState := dlPDR.FAR.State
			/*if dlPDR.FAR.State == smfContext.RULE_INITIAL {
				dlPDR.FAR.State = smfContext.RULE_CREATE
			} else {
				dlPDR.FAR.State = smfContext.RULE_UPDATE
			} */
			dlPDR.FAR.State = smfContext.RULE_INITIAL
			logger.PduSessLog.Infof("FAR ID [%d] state changed from %v → %v", dlPDR.FAR.FARID, oldFarState, dlPDR.FAR.State)

			// Collect PDR/FAR
			pfcpParam.pdrList = append(pfcpParam.pdrList, dlPDR)
			pfcpParam.farList = append(pfcpParam.farList, dlPDR.FAR)

			// ✅ QER handling
			if len(smContext.SmPolicyUpdates) > 0 &&
				smContext.SmPolicyUpdates[0].SmPolicyDecision.QosDecs != nil {

				logger.PduSessLog.Infof("Applying QoS from PolicyDecision for PDR ID [%d]", dlPDR.PDRID)

				pccRuleUpdate := smContext.SmPolicyUpdates[0].PccRuleUpdate
				if pccRuleUpdate != nil {
					addRules := pccRuleUpdate.GetAddPccRuleUpdate()
					upf := ANUPF.UPF
					for name, rule := range addRules {
						logger.PduSessLog.Infof("Installing PCC Rule [%s] on UPF [%s]", name, ANUPF.GetNodeIP())

						if pdr, err := upf.BuildCreatePdrFromPccRule(rule); err == nil {
							// QER from QoS
							if flowQer, err := ANUPF.CreatePccRuleQer(smContext, rule.RefQosData[0], rule.RefTcData[0]); err == nil {
								pdr.QER = append(pdr.QER, flowQer)
								flowQer.State = smfContext.RULE_INITIAL
								pfcpParam.qerList = append(pfcpParam.qerList, flowQer)
								logger.PduSessLog.Infof("Created QER ID [%d] for PCC Rule [%s]", flowQer.QERID, name)
							} else {
								logger.PduSessLog.Errorf("Failed to create QER for PCC Rule [%s]: %v", name, err)
							}

							// Track PDR in Tunnel
							ANUPF.UpLinkTunnel.PDR[name] = pdr
							pfcpParam.pdrList = append(pfcpParam.pdrList, pdr)
							if pdr.FAR != nil {
								pfcpParam.farList = append(pfcpParam.farList, pdr.FAR)
							}
							logger.PduSessLog.Infof("Created PDR ID [%d] for PCC Rule [%s]", pdr.PDRID, name)

						} else {
							logger.PduSessLog.Errorf("Failed to build PDR from PCC Rule [%s]: %v", name, err)
						}
					}
				}
			}

			// Track UPF pending update
			if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
				smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				logger.PduSessLog.Infof("Marked UPF [%s] as pending update", ANUPF.GetNodeIP())
			}
		}

		for _, dlPDR := range ANUPF.UpLinkTunnel.PDR {
			if shouldSendReleaseOnly {
				// Mark PDR/FAR/QER for removal
				logger.PduSessLog.Infof("[ReleaseOnly] Removing PDR ID: %v", dlPDR.PDRID)

				pfcpParam.removePDR = append(pfcpParam.removePDR, dlPDR)
				if dlPDR.FAR != nil {
					logger.PduSessLog.Infof("[ReleaseOnly] Removing FAR ID: %v", dlPDR.FAR.FARID)
					pfcpParam.removeFAR = append(pfcpParam.removeFAR, dlPDR.FAR)
				}
				if dlPDR.QER != nil {
					for _, q := range dlPDR.QER {
						logger.PduSessLog.Infof("[ReleaseOnly] Removing QER ID: %v", q.QERID)
					}
					pfcpParam.removeQER = append(pfcpParam.removeQER, dlPDR.QER...)
				}
				continue
			}

			// ✅ FAR updates
			applyAction := smfContext.ApplyAction{Forw: true}
			if smContext.SmPolicyUpdates != nil &&
				len(smContext.SmPolicyUpdates) > 0 &&
				smContext.SmPolicyUpdates[0].SmPolicyDecision != nil {
				logger.PduSessLog.Debug("SmPolicyDecision present, mapping actions to FAR (TODO)")
			}

			dlPDR.FAR.ApplyAction = applyAction
			dlPDR.FAR.ForwardingParameters = &smfContext.ForwardingParameters{
				OuterHeaderCreation: dlPDR.FAR.ForwardingParameters.OuterHeaderCreation,
				DestinationInterface: smfContext.DestinationInterface{
					InterfaceValue: smfContext.DestinationInterfaceAccess,
				},
				NetworkInstance: []byte(smContext.Dnn),
			}
			logger.PduSessLog.Infof("Updated FAR for PDR ID [%d], DestinationInterface=Access, DNN=%s",
				dlPDR.PDRID, smContext.Dnn)

			// ✅ Update states
			oldPdrState := dlPDR.State
			/* if dlPDR.State == smfContext.RULE_INITIAL {
				dlPDR.State = smfContext.RULE_CREATE
			} else {
				dlPDR.State = smfContext.RULE_UPDATE
			}*/
			dlPDR.State = smfContext.RULE_INITIAL
			logger.PduSessLog.Infof("PDR ID [%d] state changed from %v → %v", dlPDR.PDRID, oldPdrState, dlPDR.State)

			oldFarState := dlPDR.FAR.State
			/*if dlPDR.FAR.State == smfContext.RULE_INITIAL {
				dlPDR.FAR.State = smfContext.RULE_CREATE
			} else {
				dlPDR.FAR.State = smfContext.RULE_UPDATE
			} */
			dlPDR.FAR.State = smfContext.RULE_INITIAL
			logger.PduSessLog.Infof("FAR ID [%d] state changed from %v → %v", dlPDR.FAR.FARID, oldFarState, dlPDR.FAR.State)

			// Collect PDR/FAR
			pfcpParam.pdrList = append(pfcpParam.pdrList, dlPDR)
			pfcpParam.farList = append(pfcpParam.farList, dlPDR.FAR)

			// ✅ QER handling
			if len(smContext.SmPolicyUpdates) > 0 &&
				smContext.SmPolicyUpdates[0].SmPolicyDecision.QosDecs != nil {

				logger.PduSessLog.Infof("Applying QoS from PolicyDecision for PDR ID [%d]", dlPDR.PDRID)

				pccRuleUpdate := smContext.SmPolicyUpdates[0].PccRuleUpdate
				if pccRuleUpdate != nil {
					addRules := pccRuleUpdate.GetAddPccRuleUpdate()
					upf := ANUPF.UPF
					for name, rule := range addRules {
						logger.PduSessLog.Infof("Installing PCC Rule [%s] on UPF [%s]", name, ANUPF.GetNodeIP())

						if pdr, err := upf.BuildCreatePdrFromPccRule(rule); err == nil {
							// QER from QoS
							if flowQer, err := ANUPF.CreatePccRuleQer(smContext, rule.RefQosData[0], rule.RefTcData[0]); err == nil {
								pdr.QER = append(pdr.QER, flowQer)
								flowQer.State = smfContext.RULE_INITIAL
								pfcpParam.qerList = append(pfcpParam.qerList, flowQer)
								logger.PduSessLog.Infof("Created QER ID [%d] for PCC Rule [%s]", flowQer.QERID, name)
							} else {
								logger.PduSessLog.Errorf("Failed to create QER for PCC Rule [%s]: %v", name, err)
							}

							// Track PDR in Tunnel
							ANUPF.UpLinkTunnel.PDR[name] = pdr
							pfcpParam.pdrList = append(pfcpParam.pdrList, pdr)
							if pdr.FAR != nil {
								pfcpParam.farList = append(pfcpParam.farList, pdr.FAR)
							}
							logger.PduSessLog.Infof("Created PDR ID [%d] for PCC Rule [%s]", pdr.PDRID, name)

						} else {
							logger.PduSessLog.Errorf("Failed to build PDR from PCC Rule [%s]: %v", name, err)
						}
					}
				}
			}

			// Track UPF pending update
			if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
				smContext.PendingUPF[ANUPF.GetNodeIP()] = true
				logger.PduSessLog.Infof("Marked UPF [%s] as pending update", ANUPF.GetNodeIP())
			}
		}
	}

	return pfcpParam
}

/*func BuildPfcpParam(smContext *smfContext.SMContext) *pfcpParam {
	pfcpParam := &pfcpParam{
		pdrList:   []*smfContext.PDR{},
		farList:   []*smfContext.FAR{},
		barList:   []*smfContext.BAR{},
		qerList:   []*smfContext.QER{},
		removePDR: []*smfContext.PDR{},
		removeFAR: []*smfContext.FAR{},
		removeQER: []*smfContext.QER{},
	}


	return pfcpParam
} */

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
