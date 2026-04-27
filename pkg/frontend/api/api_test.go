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

// Package api wraps different api versions, and can be easily switched between different versions
// API provides http handlers used by fast-http, the handlers should only do http context checking and should dispatch
// the actual logic to
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"frontend/pkg/frontend/config"
)

var cfg = `{
		"slaQuota":1000,
		"functionCapability":1,
		"authenticationEnable":false,
		"trafficLimitDisable":true,
		"http":{
		"resptimeout":5,
		"workerInstanceReadTimeOut":5,
		"maxRequestBodySize":6
		},
		"dataSystemConfig":{
		"uploadWriteMode":"NoneL2Cache",
		"executeWriteMode":"NoneL2Cache",
		"uploadTTLSec":86400,
		"executeTTLSec":1800,
		"timeoutMs":60000
		},
		"businessType":1
	}`

func TestInitRoute(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	type args struct {
		r *gin.Engine
	}
	tests := []struct {
		name string
		args args
	}{
		{"case1 init route caas", args{r: gin.New()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitRoute(tt.args.r)
		})
	}
}

func TestInitRouteRegistersPosixWebSocket(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	r := gin.New()
	InitRoute(r)

	req := httptest.NewRequest(http.MethodGet, "/serverless/v1/posix/ws", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("POSIX WebSocket route is not registered")
	}
}
