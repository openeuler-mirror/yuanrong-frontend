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

// Package v1 -
package v1

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/datasystemclient"
	"frontend/pkg/frontend/clusterhealth"
)

// 区分健康检查方式
const (
	HealthCheckType = "HEALTH_CHECK_TYPE"
	YrDataCache     = "yrdatacache"
)

// HealthzHandler -
func HealthzHandler(ctx *gin.Context) {
	enableStream := os.Getenv(constant.EnableStream)
	healthCheckType := os.Getenv(HealthCheckType)
	if strings.ToLower(enableStream) == "true" && healthCheckType == YrDataCache {
		if !checkLocalDataSystemStatusReady() {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    http.StatusServiceUnavailable,
				"message": "datasystem unavailabe",
			})
			return
		}
	}
	ctx.Writer.WriteHeader(http.StatusOK)
}

// ClusterHealthHandler -
func ClusterHealthHandler(ctx *gin.Context) {
	clusterhealth.CheckClusterHealth(ctx.Writer, ctx.Request)
	return
}

func checkLocalDataSystemStatusReady() bool {
	return datasystemclient.IsLocalDataSystemStatusReady()
}
