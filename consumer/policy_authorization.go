// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/omec-project/openapi/models"
	smf_context "github.com/omec-project/smf/context"
	"github.com/omec-project/smf/logger"
)

// SendPolicyAuthorizationSubscribeRequest sends a policy authorization subscribe request to the PCF
func SendPolicyAuthorizationSubscribeRequest(smContext *smf_context.SMContext) (*models.UpdateEventsSubscResponse, int, error) {
	if smContext.PolicyAuthorizationClient == nil {
		logger.ConsumerLog.Errorf("smContext not selected PCF")
	}

	if err := smContext.PCFSelection(); err != nil {
		logger.ConsumerLog.Errorf("PolicyAuthorizationSubscribe, send NF Discovery Serving PCF Error[%v]", err)
		return nil, 500, fmt.Errorf("PcfError")
	}

	smPolicyID := fmt.Sprintf("%s-%d", smContext.Supi, smContext.PDUSessionID)

	// Construct the policy authorization request body
	policyAuthorizationData := models.EventsSubscReqData{
		Events: []models.AfEventSubscription{
			{
				Event:       models.AfEvent_ACCESS_TYPE_CHANGE,
				NotifMethod: models.AfNotifMethod_EVENT_DETECTION,
			},
		},
		NotifUri: fmt.Sprintf("%s://%s:%d/sm-policies/%s/update",
			smf_context.SMF_Self().URIScheme,
			smf_context.SMF_Self().RegisterIPv4,
			smf_context.SMF_Self().SBIPort,
			smPolicyID,
		),
		UsgThres: &models.UsageThreshold{
			Duration:       3600,
			TotalVolume:    1000000,
			DownlinkVolume: 500000,
			UplinkVolume:   500000,
		},
	}
	appSessionId := strconv.Itoa(int(smContext.PDUSessionID))

	// Send the request to PCF
	res, httpResp, localErr := smContext.PolicyAuthorizationClient.EventsSubscriptionDocumentApi.UpdateEventsSubsc(context.Background(), appSessionId, policyAuthorizationData)
	if localErr != nil {
		if httpResp != nil {
			logger.ConsumerLog.Errorf("Policy Authorization Subscribe Request failed with status %d: %s", httpResp.StatusCode, localErr.Error())
			return nil, httpResp.StatusCode, fmt.Errorf("setup policy authorization failed: %s", localErr.Error())
		}
		logger.ConsumerLog.Errorf("Policy Authorization Request Subscribe failed with no response: %s", localErr.Error())
		return nil, http.StatusInternalServerError, fmt.Errorf("server no response")
	} else {
		logger.ConsumerLog.Infof("Policy Authorization Request Subscribe success with response: %v", res)
	}

	return &res, httpResp.StatusCode, nil
}

// SendPolicyAuthorizationUnSubscribeRequest sends a policy authorization unsubscribe request to the PCF
func SendPolicyAuthorizationUnSubscribeRequest(smContext *smf_context.SMContext) (int, error) {
	if smContext.PolicyAuthorizationClient == nil {
		logger.ConsumerLog.Errorf("smContext not selected PCF")
	}

	appSessionId := strconv.Itoa(int(smContext.PDUSessionID))

	// Send the request to PCF
	httpResp, localErr := smContext.PolicyAuthorizationClient.EventsSubscriptionDocumentApi.DeleteEventsSubsc(context.Background(), appSessionId)

	if localErr != nil {
		if httpResp != nil {
			logger.ConsumerLog.Errorf("Policy Authorization Unsubscribe Request failed with status %d: %s", httpResp.StatusCode, localErr.Error())
			return httpResp.StatusCode, fmt.Errorf("setup policy authorization failed: %s", localErr.Error())
		}
		logger.ConsumerLog.Errorf("Policy Authorization Request Unsubscribe failed with no response: %s", localErr.Error())
		return http.StatusInternalServerError, fmt.Errorf("server no response")
	}

	return httpResp.StatusCode, nil
}
