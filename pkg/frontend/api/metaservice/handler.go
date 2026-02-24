/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2025. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package metaservice

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/config"
)

const (
	serverlessPrefix = "/serverless/v1/functions"
	adminPrefix      = "/admin/v1/functions"
)

// RegisterFunctionRoutes registers function-related APIs to be forwarded to meta_service.
func RegisterFunctionRoutes(r *gin.Engine) {
	group := r.Group(adminPrefix)
	group.POST("", forwardToMetaService)
	group.GET("", forwardToMetaService)
	group.POST("/:functionName/versions", forwardToMetaService)
	group.GET("/:functionName/versions", forwardToMetaService)
	group.GET("/:functionName", forwardToMetaService)
	group.PUT("/:functionName", forwardToMetaService)
	group.DELETE("/:functionName", forwardToMetaService)
	group.POST("/reserve-instance", forwardToMetaService)
	group.PUT("/reserve-instance", forwardToMetaService)
	group.DELETE("/reserve-instance", forwardToMetaService)
	group.GET("/reserve-instance", forwardToMetaService)
}

func forwardToMetaService(ctx *gin.Context) {
	metaServiceAddress := config.GetConfig().MetaServiceAddress
	if metaServiceAddress == "" {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"message": "meta service address is empty"})
		return
	}

	target := metaServiceAddress
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	targetURL := target + serverlessPrefix + strings.TrimPrefix(ctx.Request.URL.Path, adminPrefix)
	if ctx.Request.URL.RawQuery != "" {
		targetURL += "?" + ctx.Request.URL.RawQuery
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to read forward request body: %s", err.Error())
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	req, err := http.NewRequest(ctx.Request.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		log.GetLogger().Errorf("failed to create meta service request: %s", err.Error())
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": "failed to create forward request"})
		return
	}
	req.Header = ctx.Request.Header.Clone()
	req.Host = ""

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.GetLogger().Errorf("failed to forward request to meta service: %s", err.Error())
		ctx.JSON(http.StatusBadGateway, gin.H{"message": "failed to access meta service"})
		return
	}
	defer resp.Body.Close()

	for k, values := range resp.Header {
		for _, v := range values {
			ctx.Writer.Header().Add(k, v)
		}
	}
	ctx.Status(resp.StatusCode)
	if _, err = io.Copy(ctx.Writer, resp.Body); err != nil {
		log.GetLogger().Errorf("failed to write forward response body: %s", err.Error())
	}
}
