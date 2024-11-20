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
	"strings"

	"github.com/omec-project/openapi/Npcf_PolicyAuthorization"
	"github.com/omec-project/openapi/models"
	smf_context "github.com/omec-project/smf/context"
	"github.com/omec-project/smf/logger"
)

// SendPolicyAuthorizationSubscribeRequest sends a policy authorization subscribe request to the PCF
func SendPolicyAuthorizationSubscribeRequest(smContext *smf_context.SMContext) (*models.UpdateEventsSubscResponse, int, error) {
	configuration := Npcf_PolicyAuthorization.NewConfiguration()
	client := Npcf_PolicyAuthorization.NewAPIClient(configuration)
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

	_, err := SendNFDiscoveryPCF()
	if err != nil {
		logger.ConsumerLog.Warnf("Policy Authorization Subscribe Request - Error discovering PCF")
	}

	logger.ConsumerLog.Infof("Policy Authorization Subscribe Request Body - ", policyAuthorizationData)

	localVarPath := configuration.BasePath() + "/app-sessions/{appSessionId}/events-subscription"
	localVarPath = strings.Replace(localVarPath, "{"+"appSessionId"+"}", fmt.Sprintf("%v", appSessionId), -1)
	logger.ConsumerLog.Infof("Policy Authorization Request URL: %s", localVarPath)
	logger.ConsumerLog.Infof("Policy Authorization Host before: %s", configuration.Host())
	configuration.SetHost("10.42.0.39")
	logger.ConsumerLog.Infof("Policy Authorization Host after: %s", configuration.Host())

	// Send the request to PCF using the SMPolicyClient
	res, httpResp, localErr := client.EventsSubscriptionDocumentApi.UpdateEventsSubsc(context.Background(), appSessionId, policyAuthorizationData)

	if localErr != nil {
		if httpResp != nil {
			logger.ConsumerLog.Errorf("Policy Authorization Subscribe Request failed with status %d: %s", httpResp.StatusCode, localErr.Error())
			return nil, httpResp.StatusCode, fmt.Errorf("setup policy authorization failed: %s", localErr.Error())
		}
		logger.ConsumerLog.Errorf("Policy Authorization Request Subscribe failed with no response: %s", localErr.Error())
		return nil, http.StatusInternalServerError, fmt.Errorf("server no response")
	}

	localVarPath = configuration.BasePath() + "/app-sessions/{appSessionId}/events-subscription"
	localVarPath = strings.Replace(localVarPath, "{"+"appSessionId"+"}", fmt.Sprintf("%v", appSessionId), -1)
	logger.ConsumerLog.Infof("Policy Authorization Request URL After: %s", localVarPath)

	localVarPath = strings.Replace(localVarPath, "example.com", "10.42.0.39:29507", 1)

	logger.ConsumerLog.Infof("Policy Authorization Request URL After replace: %s", localVarPath)

	// Send the request to PCF using the SMPolicyClient
	res, httpResp, localErr = client.EventsSubscriptionDocumentApi.UpdateEventsSubsc(context.Background(), appSessionId, policyAuthorizationData)

	if localErr != nil {
		if httpResp != nil {
			logger.ConsumerLog.Errorf("Policy Authorization Subscribe Request failed with status %d: %s", httpResp.StatusCode, localErr.Error())
			return nil, httpResp.StatusCode, fmt.Errorf("setup policy authorization failed: %s", localErr.Error())
		}
		logger.ConsumerLog.Errorf("Policy Authorization Request Subscribe failed with no response: %s", localErr.Error())
		return nil, http.StatusInternalServerError, fmt.Errorf("server no response")
	}

	return &res, httpResp.StatusCode, nil
}

// SendPolicyAuthorizationUnSubscribeRequest sends a policy authorization unsubscribe request to the PCF
func SendPolicyAuthorizationUnSubscribeRequest(smContext *smf_context.SMContext) (int, error) {
	uri := smContext.SmStatusNotifyUri
	configuration := Npcf_PolicyAuthorization.NewConfiguration()
	configuration.SetBasePath(uri)
	client := Npcf_PolicyAuthorization.NewAPIClient(configuration)

	appSessionId := strconv.Itoa(int(smContext.PDUSessionID))

	// Send the request to PCF using the SMPolicyClient
	httpResp, localErr := client.EventsSubscriptionDocumentApi.DeleteEventsSubsc(context.Background(), appSessionId)

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
