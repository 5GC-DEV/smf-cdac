// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/omec-project/aper"
	"github.com/omec-project/ngap/ngapConvert"
	"github.com/omec-project/ngap/ngapType"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/smf/logger"
	"github.com/omec-project/smf/qos"
)

const DefaultNonGBR5QI = 9

func BuildPDUSessionResourceSetupRequestTransfer(ctx *SMContext) ([]byte, error) {
	ANUPF := ctx.Tunnel.DataPathPool.GetDefaultPath().FirstDPNode
	UpNode := ANUPF.UPF
	teidOct := make([]byte, 4)
	binary.BigEndian.PutUint32(teidOct, ANUPF.UpLinkTunnel.TEID)

	resourceSetupRequestTransfer := ngapType.PDUSessionResourceSetupRequestTransfer{}

	// PDU Session Aggregate Maximum Bit Rate
	// This IE is Conditional and shall be present when at least one NonGBR QoS flow is being setup.
	// TODO: should check if there is at least one NonGBR QoS flow
	ie := ngapType.PDUSessionResourceSetupRequestTransferIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDPDUSessionAggregateMaximumBitRate
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	sessRule := ctx.SelectedSessionRule()
	if sessRule == nil || sessRule.AuthSessAmbr == nil {
		return nil, fmt.Errorf("no PDU Session AMBR")
	}
	ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
		Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentPDUSessionAggregateMaximumBitRate,
		PDUSessionAggregateMaximumBitRate: &ngapType.PDUSessionAggregateMaximumBitRate{
			PDUSessionAggregateMaximumBitRateDL: ngapType.BitRate{
				Value: ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Downlink),
			},
			PDUSessionAggregateMaximumBitRateUL: ngapType.BitRate{
				Value: ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Uplink),
			},
		},
	}
	resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)

	// UL NG-U UP TNL Information
	ie = ngapType.PDUSessionResourceSetupRequestTransferIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDULNGUUPTNLInformation
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	logger.CtxLog.Infof("N3Interfaces count: %d", len(UpNode.N3Interfaces))
	// Possible cause: Physical interface connecting to UPF may be down
	if len(UpNode.N3Interfaces) == 0 {
		return nil, fmt.Errorf("N3Interfaces is empty for UPF: %v", UpNode.N3Interfaces)
	}
	if n3IP, err := UpNode.N3Interfaces[0].IP(ctx.SelectedPDUSessionType); err != nil {
		return nil, err
	} else {
		ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentULNGUUPTNLInformation,
			ULNGUUPTNLInformation: &ngapType.UPTransportLayerInformation{
				Present: ngapType.UPTransportLayerInformationPresentGTPTunnel,
				GTPTunnel: &ngapType.GTPTunnel{
					TransportLayerAddress: ngapType.TransportLayerAddress{
						Value: aper.BitString{
							Bytes:     n3IP,
							BitLength: uint64(len(n3IP) * 8),
						},
					},
					GTPTEID: ngapType.GTPTEID{Value: teidOct},
				},
			},
		}
	}

	resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)

	// PDU Session Type
	ie = ngapType.PDUSessionResourceSetupRequestTransferIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDPDUSessionType
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
		Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentPDUSessionType,
		PDUSessionType: &ngapType.PDUSessionType{
			Value: ngapType.PDUSessionTypePresentIpv4,
		},
	}
	resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)

	// Get Qos Flows
	var qosAddFlows map[string]*models.QosData

	// Initialise QosFlows with existing Ctxt QosFlows, if any
	if len(ctx.SmPolicyData.SmCtxtQosData.QosData) > 0 {
		qosAddFlows = ctx.SmPolicyData.SmCtxtQosData.QosData
	}

	// PCF has provided some update
	if len(ctx.SmPolicyUpdates) > 0 {
		smPolicyUpdates := ctx.SmPolicyUpdates[0]
		if smPolicyUpdates.QosFlowUpdate != nil && smPolicyUpdates.QosFlowUpdate.GetAddQosFlowUpdate() != nil {
			qosAddFlows = smPolicyUpdates.QosFlowUpdate.GetAddQosFlowUpdate()
		}
	}

	// QoS Flow Setup Request List
	if len(qosAddFlows) > 0 {
		ie = ngapType.PDUSessionResourceSetupRequestTransferIEs{}
		ie.Id.Value = ngapType.ProtocolIEIDQosFlowSetupRequestList
		ie.Criticality.Value = ngapType.CriticalityPresentReject

		var qosFlowsList []ngapType.QosFlowSetupRequestItem
		for _, qosFlow := range qosAddFlows {
			arpPreemptCap := ngapType.PreEmptionCapabilityPresentMayTriggerPreEmption
			if qosFlow.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
				arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
			}

			arpPreemptVul := ngapType.PreEmptionVulnerabilityPresentNotPreEmptable
			if qosFlow.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
				arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
			}

			qosFlowItem := ngapType.QosFlowSetupRequestItem{
				QosFlowIdentifier: ngapType.QosFlowIdentifier{Value: int64(qos.GetQosFlowIdFromQosId(qosFlow.QosId))},
				QosFlowLevelQosParameters: ngapType.QosFlowLevelQosParameters{
					QosCharacteristics: ngapType.QosCharacteristics{
						Present: ngapType.QosCharacteristicsPresentNonDynamic5QI,
						NonDynamic5QI: &ngapType.NonDynamic5QIDescriptor{
							FiveQI: ngapType.FiveQI{
								Value: int64(qosFlow.Var5qi),
							},
						},
					},
					AllocationAndRetentionPriority: ngapType.AllocationAndRetentionPriority{
						PriorityLevelARP: ngapType.PriorityLevelARP{
							Value: int64(qosFlow.Arp.PriorityLevel),
						},
						PreEmptionCapability: ngapType.PreEmptionCapability{
							Value: arpPreemptCap,
						},
						PreEmptionVulnerability: ngapType.PreEmptionVulnerability{
							Value: arpPreemptVul,
						},
					},
				},
			}
			qosFlowsList = append(qosFlowsList, qosFlowItem)
		}

		ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentQosFlowSetupRequestList,
			QosFlowSetupRequestList: &ngapType.QosFlowSetupRequestList{
				List: qosFlowsList,
			},
		}

		resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)
	}
	/*else {
		//Do not Delete- Might have to enable default Session rule based flow later

		// QoS Flow Setup Request List
		// Get QFI from PCF
		ie = ngapType.PDUSessionResourceSetupRequestTransferIEs{}
		ie.Id.Value = ngapType.ProtocolIEIDQosFlowSetupRequestList
		ie.Criticality.Value = ngapType.CriticalityPresentReject

		arpPreemptCap := ngapType.PreEmptionCapabilityPresentMayTriggerPreEmption
		if sessRule.AuthDefQos.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
			arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
		}

		arpPreemptVul := ngapType.PreEmptionVulnerabilityPresentNotPreEmptable
		if sessRule.AuthDefQos.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
			arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
		}
		//Default Session Rule
		ie.Value = ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentQosFlowSetupRequestList,
			QosFlowSetupRequestList: &ngapType.QosFlowSetupRequestList{

				List: []ngapType.QosFlowSetupRequestItem{
					{
						QosFlowIdentifier: ngapType.QosFlowIdentifier{
							Value: int64(sessRule.AuthDefQos.Var5qi), //DefaultNonGBR5QI,
						},
						QosFlowLevelQosParameters: ngapType.QosFlowLevelQosParameters{
							QosCharacteristics: ngapType.QosCharacteristics{
								Present: ngapType.QosCharacteristicsPresentNonDynamic5QI,
								NonDynamic5QI: &ngapType.NonDynamic5QIDescriptor{
									FiveQI: ngapType.FiveQI{
										Value: int64(sessRule.AuthDefQos.Var5qi), //DefaultNonGBR5QI,
									},
								},
							},
							AllocationAndRetentionPriority: ngapType.AllocationAndRetentionPriority{
								PriorityLevelARP: ngapType.PriorityLevelARP{
									Value: int64(sessRule.AuthDefQos.Arp.PriorityLevel), //15,
								},
								PreEmptionCapability: ngapType.PreEmptionCapability{
									Value: arpPreemptCap, //ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption,
								},
								PreEmptionVulnerability: ngapType.PreEmptionVulnerability{
									Value: arpPreemptVul, //ngapType.PreEmptionVulnerabilityPresentNotPreEmptable,
								},
							},
						},
					},
				},
			},
		}
		resourceSetupRequestTransfer.ProtocolIEs.List = append(resourceSetupRequestTransfer.ProtocolIEs.List, ie)
	}*/

	if buf, err := aper.MarshalWithParams(resourceSetupRequestTransfer, "valueExt"); err != nil {
		return nil, fmt.Errorf("encode resourceSetupRequestTransfer failed: %s", err)
	} else {
		return buf, nil
	}
}

/*func BuildPDUSessionResourceModifyRequestTransfer(ctx *SMContext) ([]byte, error) {
	ctx.SubPduSessLog.Infof("Building PDUSessionResourceModifyRequestTransfer for SUPI[%s], PDU Session ID[%d]", ctx.Supi, ctx.PDUSessionID)

	resourceModifyRequestTransfer := ngapType.PDUSessionResourceModifyRequestTransfer{}

	// Check if SM policy decision has nil/empty PCC rule ID - if so, only send QosFlowToReleaseList
	shouldSendReleaseOnly := false
	if len(ctx.SmPolicyUpdates) > 0 && ctx.SmPolicyUpdates[0].SmPolicyDecision.PccRules != nil {
		// Check if PccRules map is empty or contains nil/empty rule IDs
		if len(ctx.SmPolicyUpdates[0].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
		} else {
			// Check if any PCC rule has nil or empty ID
			for ruleId, rule := range ctx.SmPolicyUpdates[0].SmPolicyDecision.PccRules {
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					break
				}
			}
		}
	}

	if shouldSendReleaseOnly {
		ctx.SubPduSessLog.Info("PCC rule ID is nil, sending only QosFlowToReleaseList")

		// Get QFI from session rule for release
		sessRule := ctx.SelectedSessionRule()
		if sessRule == nil {
			ctx.SubPduSessLog.Error("SelectedSessionRule is nil")
			return nil, fmt.Errorf("sessRule is nil")
		}

		qfi := sessRule.AuthDefQos.Var5qi

		qosFlowToReleaseList := ngapType.QosFlowListWithCause{}
		qosFlowToReleaseList.List = append(qosFlowToReleaseList.List, ngapType.QosFlowWithCauseItem{
			QosFlowIdentifier: ngapType.QosFlowIdentifier{Value: int64(qfi)},
			Cause: ngapType.Cause{
				Present: ngapType.CausePresentNas,
				Nas:     &ngapType.CauseNas{Value: ngapType.CauseNasPresentNormalRelease},
			},
		})

		ie := ngapType.PDUSessionResourceModifyRequestTransferIEs{
			Id: ngapType.ProtocolIEID{
				Value: ngapType.ProtocolIEIDQosFlowToReleaseList,
			},
			Criticality: ngapType.Criticality{
				Value: ngapType.CriticalityPresentReject,
			},
			Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
				Present:              ngapType.PDUSessionResourceModifyRequestTransferIEsPresentQosFlowToReleaseList,
				QosFlowToReleaseList: &qosFlowToReleaseList,
			},
		}
		resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie)
		ctx.SubPduSessLog.Infof("Appended QosFlowToReleaseList with %d entries", len(qosFlowToReleaseList.List))

		// Skip AMBR and QoS flow modifications, go directly to encoding
		ctx.SubPduSessLog.Info("Encoding PDUSessionResourceModifyRequestTransfer structure (QosFlowToReleaseList only)")
		if buf, err := aper.MarshalWithParams(resourceModifyRequestTransfer, "valueExt"); err != nil {
			ctx.SubPduSessLog.Errorf("Failed to encode PDUSessionResourceModifyRequestTransfer: %v", err)
			return nil, fmt.Errorf("encode resourceModifyRequestTransfer failed: %w", err)
		} else {
			ctx.SubPduSessLog.Infof("Successfully built and encoded PDUSessionResourceModifyRequestTransfer (QosFlowToReleaseList only)")
			return buf, nil
		}
	}

	// Original logic continues here for normal cases (when PCC rule ID is not nil)

	// Step 1: PDU Session AMBR
	sessRule := ctx.SelectedSessionRule()
	if sessRule == nil {
		ctx.SubPduSessLog.Error("SelectedSessionRule is nil")
		return nil, fmt.Errorf("sessRule is nil")
	}

	if sessRule.AuthSessAmbr == nil {
		ctx.SubPduSessLog.Error("AuthSessAmbr is nil")
		return nil, fmt.Errorf("no PDU Session AMBR")
	}

	downlink := ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Downlink)
	uplink := ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Uplink)

	ctx.SubPduSessLog.Infof("Using AMBR: DL = %d bps, UL = %d bps", downlink, uplink)

	ie := ngapType.PDUSessionResourceModifyRequestTransferIEs{
		Id: ngapType.ProtocolIEID{
			Value: ngapType.ProtocolIEIDPDUSessionAggregateMaximumBitRate,
		},
		Criticality: ngapType.Criticality{
			Value: ngapType.CriticalityPresentReject,
		},
		Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceModifyRequestTransferIEsPresentPDUSessionAggregateMaximumBitRate,
			PDUSessionAggregateMaximumBitRate: &ngapType.PDUSessionAggregateMaximumBitRate{
				PDUSessionAggregateMaximumBitRateDL: ngapType.BitRate{Value: downlink},
				PDUSessionAggregateMaximumBitRateUL: ngapType.BitRate{Value: uplink},
			},
		},
	}
	resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie)

	// Step 2: Get QoS parameters from policy updates (if available) or fallback to session rule
	qfi := sessRule.AuthDefQos.Var5qi
	priority := sessRule.AuthDefQos.Arp.PriorityLevel
	arpPreemptCap := ngapType.PreEmptionCapabilityPresentMayTriggerPreEmption
	arpPreemptVul := ngapType.PreEmptionVulnerabilityPresentNotPreEmptable

	// Check for updated QoS data in policy updates
	if len(ctx.SmPolicyUpdates) > 0 {
		policyUpdate := ctx.SmPolicyUpdates[0]
		if policyUpdate != nil && policyUpdate.QosFlowUpdate != nil {
			ctx.SubPduSessLog.Infof("Found QoS flow updates in policy updates")

			// Check for modified QoS data first
			if len(policyUpdate.QosFlowUpdate.GetModified()) > 0 {
				for qosId, qosData := range policyUpdate.QosFlowUpdate.GetModified() {
					if qosData != nil {
						ctx.SubPduSessLog.Infof("Found modified QoS data for QosId[%s]: Var5QI=%d", qosId, qosData.Var5qi)
						qfi = qosData.Var5qi
						if qosData.PriorityLevel > 0 {
							priority = qosData.PriorityLevel
						}
						if qosData.Arp != nil {
							priority = qosData.Arp.PriorityLevel
							if qosData.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
								arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
							}
							if qosData.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
								arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
							}
						}
						break // Use the first modified QoS data found
					}
				}
			}

			// Check for added QoS data if no modifications found
			if len(policyUpdate.QosFlowUpdate.GetAdded()) > 0 {
				for qosId, qosData := range policyUpdate.QosFlowUpdate.GetAdded() {
					if qosData != nil {
						ctx.SubPduSessLog.Infof("Found added QoS data for QosId[%s]: Var5QI=%d", qosId, qosData.Var5qi)
						qfi = qosData.Var5qi
						if qosData.PriorityLevel > 0 {
							priority = qosData.PriorityLevel
						}
						if qosData.Arp != nil {
							priority = qosData.Arp.PriorityLevel
							if qosData.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
								arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
							}
							if qosData.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
								arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
							}
						}
						break // Use the first added QoS data found
					}
				}
			}
		}
	}

	// Apply ARP settings based on session rule if not overridden above
	if sessRule.AuthDefQos.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
		arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
	}
	if sessRule.AuthDefQos.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
		arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
	}

	ctx.SubPduSessLog.Infof("Final QoS Flow: QFI = %d, Priority = %d, PreemptCap = %d, PreemptVul = %d",
		qfi, priority, arpPreemptCap, arpPreemptVul)

	ie = ngapType.PDUSessionResourceModifyRequestTransferIEs{
		Id: ngapType.ProtocolIEID{
			Value: ngapType.ProtocolIEIDQosFlowAddOrModifyRequestList,
		},
		Criticality: ngapType.Criticality{
			Value: ngapType.CriticalityPresentReject,
		},
		Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceModifyRequestTransferIEsPresentQosFlowAddOrModifyRequestList,
			QosFlowAddOrModifyRequestList: &ngapType.QosFlowAddOrModifyRequestList{
				List: []ngapType.QosFlowAddOrModifyRequestItem{
					{
						QosFlowIdentifier: ngapType.QosFlowIdentifier{Value: int64(qfi)},
						QosFlowLevelQosParameters: &ngapType.QosFlowLevelQosParameters{
							QosCharacteristics: ngapType.QosCharacteristics{
								Present: ngapType.QosCharacteristicsPresentNonDynamic5QI,
								NonDynamic5QI: &ngapType.NonDynamic5QIDescriptor{
									FiveQI: ngapType.FiveQI{Value: int64(qfi)},
								},
							},
							AllocationAndRetentionPriority: ngapType.AllocationAndRetentionPriority{
								PriorityLevelARP: ngapType.PriorityLevelARP{Value: int64(priority)},
								PreEmptionCapability: ngapType.PreEmptionCapability{
									Value: arpPreemptCap,
								},
								PreEmptionVulnerability: ngapType.PreEmptionVulnerability{
									Value: arpPreemptVul,
								},
							},
						},
					},
				},
			},
		},
	}
	resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie)

	// Step 3: Encoding
	ctx.SubPduSessLog.Info("Encoding PDUSessionResourceModifyRequestTransfer structure")
	if buf, err := aper.MarshalWithParams(resourceModifyRequestTransfer, "valueExt"); err != nil {
		ctx.SubPduSessLog.Errorf("Failed to encode PDUSessionResourceModifyRequestTransfer: %v", err)
		return nil, fmt.Errorf("encode resourceModifyRequestTransfer failed: %w", err)
	} else {
		ctx.SubPduSessLog.Infof("Successfully built and encoded PDUSessionResourceModifyRequestTransfer")
		return buf, nil
	}
} */

func BuildPDUSessionResourceModifyRequestTransfer(ctx *SMContext) ([]byte, error) {
	ctx.SubPduSessLog.Infof("Building PDUSessionResourceModifyRequestTransfer for SUPI[%s], PDU Session ID[%d]", ctx.Supi, ctx.PDUSessionID)

	resourceModifyRequestTransfer := ngapType.PDUSessionResourceModifyRequestTransfer{}

	// Check if SM policy decision has nil/empty PCC rule ID - if so, only send QosFlowToReleaseList
	shouldSendReleaseOnly := false
	/*if len(ctx.SmPolicyUpdates) > 0 && ctx.SmPolicyUpdates[1].SmPolicyDecision.PccRules != nil {
		// Check if PccRules map is empty or contains nil/empty rule IDs
		if len(ctx.SmPolicyUpdates[1].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
		} else {
			// Check if any PCC rule has nil or empty ID
			for ruleId, rule := range ctx.SmPolicyUpdates[1].SmPolicyDecision.PccRules {
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					break
				}
			}
		}
	} */
	if len(ctx.SmPolicyUpdates) > 0 && ctx.SmPolicyUpdates[0].SmPolicyDecision.PccRules != nil {
		// Check if PccRules map is empty
		if len(ctx.SmPolicyUpdates[0].SmPolicyDecision.PccRules) == 0 {
			shouldSendReleaseOnly = true
			logger.PduSessLog.Warnln("PccRules map is empty, setting shouldSendReleaseOnly = true")
		} else {
			// Check if any PCC rule has nil or empty ID
			for ruleId, rule := range ctx.SmPolicyUpdates[0].SmPolicyDecision.PccRules {
				if ruleId == "" || rule == nil || rule.PccRuleId == "" {
					shouldSendReleaseOnly = true
					logger.PduSessLog.Warnf("Invalid PCC Rule found (ruleId='%s'), setting shouldSendReleaseOnly = true", ruleId)
					break
				}
			}
		}
	}

	if shouldSendReleaseOnly {
		ctx.SubPduSessLog.Info("PCC rule ID is nil, sending only QosFlowToReleaseList")

		// Get QFI from session rule for release
		sessRule := ctx.SelectedSessionRule()
		if sessRule == nil {
			ctx.SubPduSessLog.Error("SelectedSessionRule is nil")
			return nil, fmt.Errorf("sessRule is nil")
		}

		qfi := sessRule.AuthDefQos.Var5qi

		qosFlowToReleaseList := ngapType.QosFlowListWithCause{}
		qosFlowToReleaseList.List = append(qosFlowToReleaseList.List, ngapType.QosFlowWithCauseItem{
			QosFlowIdentifier: ngapType.QosFlowIdentifier{Value: int64(qfi)},
			Cause: ngapType.Cause{
				Present: ngapType.CausePresentNas,
				Nas:     &ngapType.CauseNas{Value: ngapType.CauseNasPresentNormalRelease},
			},
		})

		ie := ngapType.PDUSessionResourceModifyRequestTransferIEs{
			Id: ngapType.ProtocolIEID{
				Value: ngapType.ProtocolIEIDQosFlowToReleaseList,
			},
			Criticality: ngapType.Criticality{
				Value: ngapType.CriticalityPresentReject,
			},
			Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
				Present:              ngapType.PDUSessionResourceModifyRequestTransferIEsPresentQosFlowToReleaseList,
				QosFlowToReleaseList: &qosFlowToReleaseList,
			},
		}
		resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie)
		ctx.SubPduSessLog.Infof("Appended QosFlowToReleaseList with %d entries", len(qosFlowToReleaseList.List))

		// Skip AMBR and QoS flow modifications, go directly to encoding
		ctx.SubPduSessLog.Info("Encoding PDUSessionResourceModifyRequestTransfer structure (QosFlowToReleaseList only)")
		if buf, err := aper.MarshalWithParams(resourceModifyRequestTransfer, "valueExt"); err != nil {
			ctx.SubPduSessLog.Errorf("Failed to encode PDUSessionResourceModifyRequestTransfer: %v", err)
			return nil, fmt.Errorf("encode resourceModifyRequestTransfer failed: %w", err)
		} else {
			ctx.SubPduSessLog.Infof("Successfully built and encoded PDUSessionResourceModifyRequestTransfer (QosFlowToReleaseList only)")
			return buf, nil
		}
	}

	// Original logic continues here for normal cases (when PCC rule ID is not nil)

	// Step 1: PDU Session AMBR
	/*sessRule := ctx.SelectedSessionRule()
	if sessRule == nil {
		ctx.SubPduSessLog.Error("SelectedSessionRule is nil")
		return nil, fmt.Errorf("sessRule is nil")
	}

	if sessRule.AuthSessAmbr == nil {
		ctx.SubPduSessLog.Error("AuthSessAmbr is nil")
		return nil, fmt.Errorf("no PDU Session AMBR")
	}

	downlink := ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Downlink)
	uplink := ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Uplink)

	ctx.SubPduSessLog.Infof("Using AMBR: DL = %d bps, UL = %d bps", downlink, uplink)

	ie := ngapType.PDUSessionResourceModifyRequestTransferIEs{
		Id: ngapType.ProtocolIEID{
			Value: ngapType.ProtocolIEIDPDUSessionAggregateMaximumBitRate,
		},
		Criticality: ngapType.Criticality{
			Value: ngapType.CriticalityPresentReject,
		},
		Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceModifyRequestTransferIEsPresentPDUSessionAggregateMaximumBitRate,
			PDUSessionAggregateMaximumBitRate: &ngapType.PDUSessionAggregateMaximumBitRate{
				PDUSessionAggregateMaximumBitRateDL: ngapType.BitRate{Value: downlink},
				PDUSessionAggregateMaximumBitRateUL: ngapType.BitRate{Value: uplink},
			},
		},
	}
	resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie) */

	sessRule := ctx.SelectedSessionRule()
	if sessRule == nil {
		ctx.SubPduSessLog.Error("SelectedSessionRule is nil")
		return nil, fmt.Errorf("sessRule is nil")
	}

	if sessRule.AuthSessAmbr == nil {
		ctx.SubPduSessLog.Error("AuthSessAmbr is nil")
		return nil, fmt.Errorf("no PDU Session AMBR")
	}

	// Take AMBR first (session level)
	downlink := ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Downlink)
	uplink := ngapConvert.UEAmbrToInt64(sessRule.AuthSessAmbr.Uplink)
	qfi := sessRule.AuthDefQos.Var5qi
	priority := sessRule.AuthDefQos.Arp.PriorityLevel
	qi := sessRule.AuthDefQos.Var5qi // initialize qi with default Var5QI

	// Now check if SM Policy Decision has QosData for this session rule
	var policyDecision *qos.SmCtxtPolicyData
	// policyDecision := ctx.SmPolicyData
	if policyDecision != nil {
		for _, qos := range policyDecision.SmCtxtQosData.QosData {
			// Example: log GBR/MBR values
			ctx.SubPduSessLog.Infof(
				"QoSId=%s, Var5QI=%d, GBR: UL=%s, DL=%s, MBR: UL=%s, DL=%s",
				qos.QosId, qos.Var5qi, qos.GbrUl, qos.GbrDl, qos.MaxbrUl, qos.MaxbrDl,
			)

			// If GBR is available, override AMBR for this flow
			if qos.GbrDl != "" {
				if val, err := StringToBitRate(qos.GbrDl); err == nil {
					downlink = int64(val)
				}
			}
			if qos.GbrUl != "" {
				if val, err := StringToBitRate(qos.GbrUl); err == nil {
					uplink = int64(val)
				}
			}
			// Convert QosId (string) -> int32
			/*if qfiVal, err := strconv.Atoi(qos.QosId); err == nil {
				qfi = int32(qfiVal)
			} else {
				ctx.SubPduSessLog.Errorf("Invalid QosId string: %s", qos.QosId)
			}

			// Assign ARP PriorityLevel properly
			if qos.Arp != nil {
				priority = qos.Arp.PriorityLevel
			}*/
		}
	}

	ctx.SubPduSessLog.Infof("Using QoS: DL = %d bps, UL = %d bps, qfi = %d , arp = %d ", downlink, uplink, qfi, priority)

	// Build NGAP IE
	ie := ngapType.PDUSessionResourceModifyRequestTransferIEs{
		Id: ngapType.ProtocolIEID{
			Value: ngapType.ProtocolIEIDPDUSessionAggregateMaximumBitRate,
		},
		Criticality: ngapType.Criticality{
			Value: ngapType.CriticalityPresentReject,
		},
		Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceModifyRequestTransferIEsPresentPDUSessionAggregateMaximumBitRate,
			PDUSessionAggregateMaximumBitRate: &ngapType.PDUSessionAggregateMaximumBitRate{
				PDUSessionAggregateMaximumBitRateDL: ngapType.BitRate{Value: downlink},
				PDUSessionAggregateMaximumBitRateUL: ngapType.BitRate{Value: uplink},
			},
		},
	}
	resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie)

	// Step 2: Get QoS parameters from policy updates (if available) or fallback to session rule

	arpPreemptCap := ngapType.PreEmptionCapabilityPresentMayTriggerPreEmption
	arpPreemptVul := ngapType.PreEmptionVulnerabilityPresentNotPreEmptable

	// Check for updated QoS data in policy updates
	if len(ctx.SmPolicyUpdates) > 0 {
		policyUpdate := ctx.SmPolicyUpdates[0]
		if policyUpdate != nil && policyUpdate.QosFlowUpdate != nil {
			ctx.SubPduSessLog.Infof("Found QoS flow updates in policy updates")

			// Check for modified QoS data first
			if len(policyUpdate.QosFlowUpdate.GetModified()) > 0 {
				for qosId, qosData := range policyUpdate.QosFlowUpdate.GetModified() {
					if qosData != nil {
						ctx.SubPduSessLog.Infof("Found modified QoS data for QosId[%s]: Var5QI=%d", qosId, qosData.Var5qi)
						// Convert QosId string -> int32
						if qfiVal, err := strconv.Atoi(qosData.QosId); err == nil {
							qfi = int32(qfiVal)
						} else {
							ctx.SubPduSessLog.Errorf("Invalid QosId string: %s", qosData.QosId)
						}
						qi = qosData.Var5qi
						if qosData.PriorityLevel > 0 {
							priority = qosData.PriorityLevel
						}
						if qosData.Arp != nil {
							priority = qosData.Arp.PriorityLevel
							if qosData.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
								arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
							}
							if qosData.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
								arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
							}
						}
						break // Use the first modified QoS data found
					}
				}
			}

			// Check for added QoS data if no modifications found
			if len(policyUpdate.QosFlowUpdate.GetAdded()) > 0 {
				for qosId, qosData := range policyUpdate.QosFlowUpdate.GetAdded() {
					if qosData != nil {
						ctx.SubPduSessLog.Infof("Found added QoS data for QosId[%s]: Var5QI=%d", qosId, qosData.Var5qi)

						// Convert QosId string -> int32
						if qfiVal, err := strconv.Atoi(qosData.QosId); err == nil {
							qfi = int32(qfiVal)
						} else {
							ctx.SubPduSessLog.Errorf("Invalid QosId string: %s", qosData.QosId)
						}
						qi = qosData.Var5qi
						if qosData.PriorityLevel > 0 {
							priority = qosData.PriorityLevel
						}
						if qosData.Arp != nil {
							priority = qosData.Arp.PriorityLevel
							if qosData.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
								arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
							}
							if qosData.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
								arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
							}
						}
						break // Use the first added QoS data found
					}
				}
			}
		}
	}

	// Apply ARP settings based on session rule if not overridden above
	if sessRule.AuthDefQos.Arp.PreemptCap == models.PreemptionCapability_NOT_PREEMPT {
		arpPreemptCap = ngapType.PreEmptionCapabilityPresentShallNotTriggerPreEmption
	}
	if sessRule.AuthDefQos.Arp.PreemptVuln == models.PreemptionVulnerability_PREEMPTABLE {
		arpPreemptVul = ngapType.PreEmptionVulnerabilityPresentPreEmptable
	}

	ctx.SubPduSessLog.Infof("Final QoS Flow: QFI = %d, Priority = %d, PreemptCap = %d, PreemptVul = %d",
		qfi, priority, arpPreemptCap, arpPreemptVul)

	ie = ngapType.PDUSessionResourceModifyRequestTransferIEs{
		Id: ngapType.ProtocolIEID{
			Value: ngapType.ProtocolIEIDQosFlowAddOrModifyRequestList,
		},
		Criticality: ngapType.Criticality{
			Value: ngapType.CriticalityPresentReject,
		},
		Value: ngapType.PDUSessionResourceModifyRequestTransferIEsValue{
			Present: ngapType.PDUSessionResourceModifyRequestTransferIEsPresentQosFlowAddOrModifyRequestList,
			QosFlowAddOrModifyRequestList: &ngapType.QosFlowAddOrModifyRequestList{
				List: []ngapType.QosFlowAddOrModifyRequestItem{
					{
						QosFlowIdentifier: ngapType.QosFlowIdentifier{Value: int64(qfi)},
						QosFlowLevelQosParameters: &ngapType.QosFlowLevelQosParameters{
							QosCharacteristics: ngapType.QosCharacteristics{
								Present: ngapType.QosCharacteristicsPresentNonDynamic5QI,
								NonDynamic5QI: &ngapType.NonDynamic5QIDescriptor{
									FiveQI: ngapType.FiveQI{Value: int64(qi)},
								},
							},
							AllocationAndRetentionPriority: ngapType.AllocationAndRetentionPriority{
								PriorityLevelARP: ngapType.PriorityLevelARP{Value: int64(priority)},
								PreEmptionCapability: ngapType.PreEmptionCapability{
									Value: arpPreemptCap,
								},
								PreEmptionVulnerability: ngapType.PreEmptionVulnerability{
									Value: arpPreemptVul,
								},
							},
						},
					},
				},
			},
		},
	}
	resourceModifyRequestTransfer.ProtocolIEs.List = append(resourceModifyRequestTransfer.ProtocolIEs.List, ie)

	// Step 3: Encoding
	ctx.SubPduSessLog.Info("Encoding PDUSessionResourceModifyRequestTransfer structure")
	if buf, err := aper.MarshalWithParams(resourceModifyRequestTransfer, "valueExt"); err != nil {
		ctx.SubPduSessLog.Errorf("Failed to encode PDUSessionResourceModifyRequestTransfer: %v", err)
		return nil, fmt.Errorf("encode resourceModifyRequestTransfer failed: %w", err)
	} else {
		ctx.SubPduSessLog.Infof("Successfully built and encoded PDUSessionResourceModifyRequestTransfer")
		return buf, nil
	}
}

func StringToBitRate(s string) (uint64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasSuffix(s, "kbps") {
		val, err := strconv.ParseUint(strings.TrimSuffix(s, "kbps"), 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1000, nil
	}
	if strings.HasSuffix(s, "mbps") {
		val, err := strconv.ParseUint(strings.TrimSuffix(s, "mbps"), 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1000 * 1000, nil
	}
	return 0, nil
}

func BuildPDUSessionResourceReleaseCommandTransfer(ctx *SMContext) (buf []byte, err error) {
	resourceReleaseCommandTransfer := ngapType.PDUSessionResourceReleaseCommandTransfer{
		Cause: ngapType.Cause{
			Present: ngapType.CausePresentNas,
			Nas: &ngapType.CauseNas{
				Value: ngapType.CauseNasPresentNormalRelease,
			},
		},
	}
	buf, err = aper.MarshalWithParams(resourceReleaseCommandTransfer, "valueExt")
	if err != nil {
		return nil, err
	}
	return
}

// TS 38.413 9.3.4.9
func BuildPathSwitchRequestAcknowledgeTransfer(ctx *SMContext) ([]byte, error) {
	ANUPF := ctx.Tunnel.DataPathPool.GetDefaultPath().FirstDPNode
	UpNode := ANUPF.UPF
	teidOct := make([]byte, 4)
	binary.BigEndian.PutUint32(teidOct, ANUPF.UpLinkTunnel.TEID)

	pathSwitchRequestAcknowledgeTransfer := ngapType.PathSwitchRequestAcknowledgeTransfer{}

	// UL NG-U UP TNL Information(optional) TS 38.413 9.3.2.2
	pathSwitchRequestAcknowledgeTransfer.
		ULNGUUPTNLInformation = new(ngapType.UPTransportLayerInformation)

	ULNGUUPTNLInformation := pathSwitchRequestAcknowledgeTransfer.ULNGUUPTNLInformation
	ULNGUUPTNLInformation.Present = ngapType.UPTransportLayerInformationPresentGTPTunnel
	ULNGUUPTNLInformation.GTPTunnel = new(ngapType.GTPTunnel)

	if n3IP, err := UpNode.N3Interfaces[0].IP(ctx.SelectedPDUSessionType); err != nil {
		return nil, err
	} else {
		gtpTunnel := ULNGUUPTNLInformation.GTPTunnel
		gtpTunnel.GTPTEID.Value = teidOct
		gtpTunnel.TransportLayerAddress.Value = aper.BitString{
			Bytes:     n3IP,
			BitLength: uint64(len(n3IP) * 8),
		}
	}

	// Security Indication(optional) TS 38.413 9.3.1.27
	pathSwitchRequestAcknowledgeTransfer.SecurityIndication = new(ngapType.SecurityIndication)
	securityIndication := pathSwitchRequestAcknowledgeTransfer.SecurityIndication
	// TODO: use real value
	securityIndication.IntegrityProtectionIndication.Value = ngapType.IntegrityProtectionIndicationPresentNotNeeded
	// TODO: use real value
	securityIndication.ConfidentialityProtectionIndication.Value = ngapType.ConfidentialityProtectionIndicationPresentNotNeeded

	integrityProtectionInd := securityIndication.IntegrityProtectionIndication.Value
	if integrityProtectionInd == ngapType.IntegrityProtectionIndicationPresentRequired ||
		integrityProtectionInd == ngapType.IntegrityProtectionIndicationPresentPreferred {
		securityIndication.MaximumIntegrityProtectedDataRateUL = new(ngapType.MaximumIntegrityProtectedDataRate)
		// TODO: use real value
		securityIndication.MaximumIntegrityProtectedDataRateUL.Value = ngapType.MaximumIntegrityProtectedDataRatePresentBitrate64kbs
	}

	if buf, err := aper.MarshalWithParams(pathSwitchRequestAcknowledgeTransfer, "valueExt"); err != nil {
		return nil, err
	} else {
		return buf, nil
	}
}

func BuildPathSwitchRequestUnsuccessfulTransfer(causePresent int, causeValue aper.Enumerated) (buf []byte, err error) {
	pathSwitchRequestUnsuccessfulTransfer := ngapType.PathSwitchRequestUnsuccessfulTransfer{}

	pathSwitchRequestUnsuccessfulTransfer.Cause.Present = causePresent
	cause := &pathSwitchRequestUnsuccessfulTransfer.Cause

	switch causePresent {
	case ngapType.CausePresentRadioNetwork:
		cause.RadioNetwork = new(ngapType.CauseRadioNetwork)
		cause.RadioNetwork.Value = causeValue
	case ngapType.CausePresentTransport:
		cause.Transport = new(ngapType.CauseTransport)
		cause.Transport.Value = causeValue
	case ngapType.CausePresentNas:
		cause.Nas = new(ngapType.CauseNas)
		cause.Nas.Value = causeValue
	case ngapType.CausePresentProtocol:
		cause.Protocol = new(ngapType.CauseProtocol)
		cause.Protocol.Value = causeValue
	case ngapType.CausePresentMisc:
		cause.Misc = new(ngapType.CauseMisc)
		cause.Misc.Value = causeValue
	}

	buf, err = aper.MarshalWithParams(pathSwitchRequestUnsuccessfulTransfer, "valueExt")
	if err != nil {
		return nil, err
	}
	return
}

func BuildHandoverCommandTransfer(ctx *SMContext) ([]byte, error) {
	ANUPF := ctx.Tunnel.DataPathPool.GetDefaultPath().FirstDPNode
	UpNode := ANUPF.UPF
	teidOct := make([]byte, 4)
	binary.BigEndian.PutUint32(teidOct, ANUPF.UpLinkTunnel.TEID)
	handoverCommandTransfer := ngapType.HandoverCommandTransfer{}

	handoverCommandTransfer.DLForwardingUPTNLInformation = new(ngapType.UPTransportLayerInformation)
	handoverCommandTransfer.DLForwardingUPTNLInformation.Present = ngapType.UPTransportLayerInformationPresentGTPTunnel
	handoverCommandTransfer.DLForwardingUPTNLInformation.GTPTunnel = new(ngapType.GTPTunnel)

	if n3IP, err := UpNode.N3Interfaces[0].IP(ctx.SelectedPDUSessionType); err != nil {
		return nil, err
	} else {
		gtpTunnel := handoverCommandTransfer.DLForwardingUPTNLInformation.GTPTunnel
		gtpTunnel.GTPTEID.Value = teidOct
		gtpTunnel.TransportLayerAddress.Value = aper.BitString{
			Bytes:     n3IP,
			BitLength: uint64(len(n3IP) * 8),
		}
	}

	if buf, err := aper.MarshalWithParams(handoverCommandTransfer, "valueExt"); err != nil {
		return nil, err
	} else {
		return buf, nil
	}
}
