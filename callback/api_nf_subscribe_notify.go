// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package callback

import (
	"net/http"

	"github.com/5GC-DEV/openapi-cdac"
	"github.com/5GC-DEV/openapi-cdac/models"
	"github.com/5GC-DEV/util-cdac/httpwrapper"
	"github.com/gin-gonic/gin"
	"github.com/omec-project/smf/consumer"
	"github.com/omec-project/smf/logger"
	"github.com/omec-project/smf/producer"
)

const contentTypeJSON = "application/json"

func HTTPNfSubscriptionStatusNotify(c *gin.Context) {
	var nfSubscriptionStatusNotification models.NotificationData

	requestBody, err := c.GetRawData()
	if err != nil {
		logger.PduSessLog.Errorf("Get Request Body error: %+v", err)
		problemDetail := models.ProblemDetails{
			Title:  "System failure",
			Status: http.StatusInternalServerError,
			Detail: err.Error(),
			Cause:  "SYSTEM_FAILURE",
		}
		c.JSON(http.StatusInternalServerError, problemDetail)
		return
	}

	err = openapi.Deserialize(&nfSubscriptionStatusNotification, requestBody, contentTypeJSON)
	if err != nil {
		problemDetail := "[Request Body] " + err.Error()
		rsp := models.ProblemDetails{
			Title:  "Malformed request syntax",
			Status: http.StatusBadRequest,
			Detail: problemDetail,
		}
		logger.PduSessLog.Errorln(problemDetail)
		c.JSON(http.StatusBadRequest, rsp)
		return
	}

	req := httpwrapper.NewRequest(c.Request, nfSubscriptionStatusNotification)

	rsp := producer.HandleNfSubscriptionStatusNotify(req)

	responseBody, err := openapi.Serialize(rsp.Body, contentTypeJSON)
	if err != nil {
		logger.PduSessLog.Errorf("Error fetching response for HTTPNfSubscriptionStatusNotify : %+v\n", err)
		problemDetails := models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "SYSTEM_FAILURE",
			Detail: err.Error(),
		}
		c.JSON(http.StatusInternalServerError, problemDetails)
	} else {
		c.Data(rsp.Status, contentTypeJSON, responseBody)
		consumer.SendRemoveSubscriptionProcedure(nfSubscriptionStatusNotification)
	}
}
