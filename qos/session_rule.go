// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package qos

import (
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/smf/logger"
)

// Handle Session Rule related info
type SessRulesUpdate struct {
	add, mod, del  map[string]*models.SessionRule
	ActiveSessRule *models.SessionRule
	activeRuleName string
}

// Get Session rule changes delta
func GetSessionRulesUpdate(pcfSessRules, ctxtSessRules map[string]*models.SessionRule) *SessRulesUpdate {
	if len(pcfSessRules) == 0 {
		logger.PduSessLog.Infof("GetSessionRulesUpdate: No PCF session rules provided, returning nil")
		return nil
	}

	change := SessRulesUpdate{
		add: make(map[string]*models.SessionRule),
		mod: make(map[string]*models.SessionRule),
		del: make(map[string]*models.SessionRule),
	}
	logger.PduSessLog.Infof("GetSessionRulesUpdate: Processing %d PCF session rules", len(pcfSessRules))

	// TODO: Iterate through all session rules from PCF and check against ctxt session rules
	// Get only active session Rule for now
	for name, sessRule := range pcfSessRules {
		// Rules to be deleted
		if sessRule == nil {
			logger.PduSessLog.Warnf("GetSessionRulesUpdate: SessionRule[%s] marked for deletion (nil in PCF rules)", name)
			change.del[name] = sessRule // nil
			continue
		}

		// Rules to be added
		if ctxtSessRules[name] == nil {
			change.add[name] = sessRule

			// Activate last rule
			change.activeRuleName = name
			change.ActiveSessRule = sessRule
			logger.PduSessLog.Infof("GetSessionRulesUpdate: SessionRule[%s] marked for addition and set as active rule", name)
		} else {
			change.mod[name] = sessRule
			logger.PduSessLog.Infof("GetSessionRulesUpdate: SessionRule[%s] exists, marked for modification", name)
			// Rules to be modified
			// TODO
		}
	}
	logger.PduSessLog.Infof("GetSessionRulesUpdate: Completed - Added=%d, Modified=%d, Deleted=%d, ActiveRule=%s", len(change.add), len(change.mod), len(change.del), change.activeRuleName)
	return &change
}

func CommitSessionRulesUpdate(smCtxtPolData *SmCtxtPolicyData, update *SessRulesUpdate) {
	// Iterate through Add/Mod/Del rules

	// Add new Rules
	if len(update.add) > 0 {
		for name, rule := range update.add {
			smCtxtPolData.SmCtxtSessionRules.SessionRules[name] = rule
		}
	}

	// Mod rules
	// TODO

	// Del Rules
	if len(update.del) > 0 {
		for name := range update.del {
			delete(smCtxtPolData.SmCtxtSessionRules.SessionRules, name)
		}
	}

	// Set Active Rule
	smCtxtPolData.SmCtxtSessionRules.ActiveRule = update.ActiveSessRule
	smCtxtPolData.SmCtxtSessionRules.ActiveRuleName = update.activeRuleName
}
