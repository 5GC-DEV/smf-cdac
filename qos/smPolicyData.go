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
	Supi string
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

func BuildSmPolicyUpdate(smCtxtPolData *SmCtxtPolicyData, smPolicyDecision *models.SmPolicyDecision) *PolicyUpdate {
	update := &PolicyUpdate{}

	// Derive correlation ID from SuppFeat (fallback to "N/A")
	corrID := "N/A"
	if smPolicyDecision != nil && smPolicyDecision.SuppFeat != "" {
		corrID = smPolicyDecision.SuppFeat
	}

	// Keep copy of SmPolicyDecision received from PCF
	update.SmPolicyDecision = smPolicyDecision
	logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] Received SmPolicyDecision from PCF", smCtxtPolData.Supi, corrID)

	// Qos Flows update
	update.QosFlowUpdate = GetQosFlowDescUpdate(smPolicyDecision.QosDecs, smCtxtPolData.SmCtxtQosData.QosData, smCtxtPolData.Supi)
	if update.QosFlowUpdate != nil {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] QoS Flow Update built", smCtxtPolData.Supi, corrID)
	} else {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] No QoS Flow Update", smCtxtPolData.Supi, corrID)
	}

	// Pcc Rules update
	update.PccRuleUpdate = GetPccRulesUpdate(smPolicyDecision.PccRules, smCtxtPolData.SmCtxtPccRules.PccRules, smCtxtPolData.Supi)
	if update.PccRuleUpdate != nil {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] PCC Rules Update built", smCtxtPolData.Supi, corrID)
	} else {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] No PCC Rules Update", smCtxtPolData.Supi, corrID)
	}

	// Session Rules update
	update.SessRuleUpdate = GetSessionRulesUpdate(smPolicyDecision.SessRules, smCtxtPolData.SmCtxtSessionRules.SessionRules, smCtxtPolData.Supi)
	if update.SessRuleUpdate != nil {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] Session Rules Update built", smCtxtPolData.Supi, corrID)
	} else {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] No Session Rules Update", smCtxtPolData.Supi, corrID)
	}

	// Traffic Control Data update
	update.TCUpdate = GetTrafficControlUpdate(smPolicyDecision.TraffContDecs, smCtxtPolData.SmCtxtTCData.TrafficControlData, smCtxtPolData.Supi)
	if update.TCUpdate != nil {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] Traffic Control Data Update built", smCtxtPolData.Supi, corrID)
	} else {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] No Traffic Control Data Update", smCtxtPolData.Supi, corrID)
	}

	// Condition Data update
	update.CondDataUpdate = GetConditionDataUpdate(smPolicyDecision.Conds, smCtxtPolData.SmCtxtCondData.CondData, smCtxtPolData.Supi)
	if update.CondDataUpdate != nil {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] Condition Data Update built", smCtxtPolData.Supi, corrID)
	} else {
		logger.PduSessLog.Infof("[SUPI=%s][CorrID=%s] No Condition Data Update", smCtxtPolData.Supi, corrID)
	}
	// ---- Final log before return ----
	logger.PduSessLog.Infof(
		"[SUPI=%s][CorrID=%s] Completed BuildSmPolicyUpdate: QosFlows=%d, PccRules=%d, SessRules=%d, TCUpdates=%d, CondData=%d",
		smCtxtPolData.Supi,
		corrID,
		len(update.QosFlowUpdate.add)+len(update.QosFlowUpdate.mod)+len(update.QosFlowUpdate.del),
		len(update.PccRuleUpdate.add)+len(update.PccRuleUpdate.mod)+len(update.PccRuleUpdate.del),
		len(update.SessRuleUpdate.add)+len(update.SessRuleUpdate.mod)+len(update.SessRuleUpdate.del),
		len(update.TCUpdate.add)+len(update.TCUpdate.mod)+len(update.TCUpdate.del),
		len(update.CondDataUpdate.add)+len(update.CondDataUpdate.mod)+len(update.CondDataUpdate.del),
	)
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
