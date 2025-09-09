// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package qos

import (
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/smf/logger"
)

type CondDataUpdate struct {
	add, mod, del map[string]*models.ConditionData
}

func GetConditionDataUpdate(condData, ctxtCondData map[string]*models.ConditionData, supi string) *CondDataUpdate {
	change := CondDataUpdate{
		add: make(map[string]*models.ConditionData),
		mod: make(map[string]*models.ConditionData),
		del: make(map[string]*models.ConditionData),
	}
	logger.PduSessLog.Info("[SUPI=%s] CondData-QoS Flow Update Summary: add=%d, mod=%d, del=%d", supi, len(change.add), len(change.mod), len(change.del))
	// TODO
	return &change
}

func CommitConditionDataUpdate(smCtxtPolData *SmCtxtPolicyData, update *CondDataUpdate) {
	// TODO
}
