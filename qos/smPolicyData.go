// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package qos

import (
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/smf/logger"
)

// Define SMF Session-Rule/PccRule/Rule-Qos-Data
type PolicyUpdate struct {
	SessRuleUpdate *SessRulesUpdate
	PccRuleUpdate  *PccRulesUpdate
	QosFlowUpdate  *QosFlowsUpdate
	TCUpdate       *TrafficControlUpdate
	CondDataUpdate *CondDataUpdate

	// relevant SM Policy Decision from PCF
	SmPolicyDecision *models.SmPolicyDecision
}

type SmCtxtPolicyData struct {
	// maintain all session rule-info and current active sess rule
	SmCtxtPccRules     SmCtxtPccRulesInfo
	SmCtxtQosData      SmCtxtQosData
	SmCtxtTCData       SmCtxtTrafficControlData
	SmCtxtChargingData SmCtxtChargingData
	SmCtxtCondData     SmCtxtCondData
	SmCtxtSessionRules SmCtxtSessionRulesInfo
}

// maintain all session rule-info and current active sess rule
type SmCtxtSessionRulesInfo struct {
	ActiveRule     *models.SessionRule
	SessionRules   map[string]*models.SessionRule
	ActiveRuleName string
}

type SmCtxtPccRulesInfo struct {
	PccRules map[string]*models.PccRule
	// TODO:Rulename to RuleId Map
}

type SmCtxtQosData struct {
	QosData map[string]*models.QosData
}

type SmCtxtTrafficControlData struct {
	TrafficControlData map[string]*models.TrafficControlData
}

type SmCtxtChargingData struct {
	ChargingData map[string]*models.ChargingData
}

type SmCtxtCondData struct {
	CondData map[string]*models.ConditionData
}

func (obj *SmCtxtPolicyData) Initialize() {
	obj.SmCtxtSessionRules.SessionRules = make(map[string]*models.SessionRule)
	obj.SmCtxtPccRules.PccRules = make(map[string]*models.PccRule)
	obj.SmCtxtQosData.QosData = make(map[string]*models.QosData)
	obj.SmCtxtCondData.CondData = make(map[string]*models.ConditionData)
	obj.SmCtxtChargingData.ChargingData = make(map[string]*models.ChargingData)
	obj.SmCtxtTCData.TrafficControlData = make(map[string]*models.TrafficControlData)
}

/*
	func BuildSmPolicyUpdate(smCtxtPolData *SmCtxtPolicyData, smPolicyDecision *models.SmPolicyDecision) *PolicyUpdate {
		update := &PolicyUpdate{}

		// Keep copy of SmPolicyDecision received from PCF
		update.SmPolicyDecision = smPolicyDecision

		// Qos Flows update
		update.QosFlowUpdate = GetQosFlowDescUpdate(smPolicyDecision.QosDecs, smCtxtPolData.SmCtxtQosData.QosData)

		// Pcc Rules update
		update.PccRuleUpdate = GetPccRulesUpdate(smPolicyDecision.PccRules, smCtxtPolData.SmCtxtPccRules.PccRules)

		// Session Rules update
		update.SessRuleUpdate = GetSessionRulesUpdate(smPolicyDecision.SessRules, smCtxtPolData.SmCtxtSessionRules.SessionRules)

		// Traffic Control Data update
		update.TCUpdate = GetTrafficControlUpdate(smPolicyDecision.TraffContDecs, smCtxtPolData.SmCtxtTCData.TrafficControlData)

		// Condition Data update
		update.CondDataUpdate = GetConditionDataUpdate(smPolicyDecision.Conds, smCtxtPolData.SmCtxtCondData.CondData)

		return update
	}
*/
func BuildSmPolicyUpdate(smCtxtPolData *SmCtxtPolicyData, smPolicyDecision *models.SmPolicyDecision) *PolicyUpdate {
	logger.PduSessLog.Infoln("Building SM Policy Update from received SmPolicyDecision")

	update := &PolicyUpdate{}

	// Log incoming decision
	logger.PduSessLog.Infof("Received SmPolicyDecision: %+v", smPolicyDecision)

	// Keep copy of SmPolicyDecision received from PCF
	update.SmPolicyDecision = smPolicyDecision

	// QoS Flows update
	update.QosFlowUpdate = GetQosFlowDescUpdate(smPolicyDecision.QosDecs, smCtxtPolData.SmCtxtQosData.QosData)
	logger.PduSessLog.Infof("QoS Flow Update: %+v", update.QosFlowUpdate)

	// PCC Rules update
	update.PccRuleUpdate = GetPccRulesUpdate(smPolicyDecision.PccRules, smCtxtPolData.SmCtxtPccRules.PccRules)
	logger.PduSessLog.Infof("PCC Rule Update: %+v", update.PccRuleUpdate)

	// Session Rules update
	update.SessRuleUpdate = GetSessionRulesUpdate(smPolicyDecision.SessRules, smCtxtPolData.SmCtxtSessionRules.SessionRules)
	logger.PduSessLog.Infof("Session Rule Update: %+v", update.SessRuleUpdate)

	// Traffic Control Data update
	update.TCUpdate = GetTrafficControlUpdate(smPolicyDecision.TraffContDecs, smCtxtPolData.SmCtxtTCData.TrafficControlData)
	logger.PduSessLog.Infof("Traffic Control Update: %+v", update.TCUpdate)

	// Condition Data update
	update.CondDataUpdate = GetConditionDataUpdate(smPolicyDecision.Conds, smCtxtPolData.SmCtxtCondData.CondData)
	logger.PduSessLog.Infof("Condition Data Update: %+v", update.CondDataUpdate)

	logger.PduSessLog.Infoln("Completed building SM Policy Update")

	return update
}

func CommitSmPolicyDecision(smCtxtPolData *SmCtxtPolicyData, smPolicyUpdate *PolicyUpdate) error {
	// Update Qos Flows
	if smPolicyUpdate.QosFlowUpdate != nil {
		CommitQosFlowDescUpdate(smCtxtPolData, smPolicyUpdate.QosFlowUpdate)
	}

	// Update PCC Rules
	if smPolicyUpdate.PccRuleUpdate != nil {
		CommitPccRulesUpdate(smCtxtPolData, smPolicyUpdate.PccRuleUpdate)
	}

	// Update Session Rules
	if smPolicyUpdate.SessRuleUpdate != nil {
		CommitSessionRulesUpdate(smCtxtPolData, smPolicyUpdate.SessRuleUpdate)
	}

	// Update Traffic Control data
	if smPolicyUpdate.TCUpdate != nil {
		CommitTrafficControlUpdate(smCtxtPolData, smPolicyUpdate.TCUpdate)
	}

	// Update Condition Data
	if smPolicyUpdate.CondDataUpdate != nil {
		CommitConditionDataUpdate(smCtxtPolData, smPolicyUpdate.CondDataUpdate)
	}

	return nil
}
