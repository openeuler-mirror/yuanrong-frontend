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

// Package frontend the api of frontend
package frontend

import (
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/metrics"
	"frontend/pkg/frontend/serverstatus"
)

var (
	createMetricsOnce sync.Once
	invokeMetricsOnce sync.Once
	killMetricsOnce   sync.Once
)

func initCreateMetrics() {
	createMetricsOnce.Do(func() {
		// Register counter for create handler operations with function name and http code
		err := metrics.RegisterCounter("handler_create_operations_total", "Total number of create handler operations", []string{"function_name", "http_code"})
		if err != nil {
			log.GetLogger().Warnf("failed to register handler_create_operations_total metric: %v", err)
		}
		// Register histogram for create handler operation duration
		err = metrics.RegisterHistogram("handler_create_operation_duration_seconds", "Create handler operation duration in seconds", []string{"function_name"}, nil)
		if err != nil {
			log.GetLogger().Warnf("failed to register handler_create_operation_duration_seconds metric: %v", err)
		}
	})
}

func initInvokeMetrics() {
	invokeMetricsOnce.Do(func() {
		// Register counter for invoke handler operations with function name and http code
		err := metrics.RegisterCounter("handler_invoke_operations_total", "Total number of invoke handler operations", []string{"function_name", "http_code"})
		if err != nil {
			log.GetLogger().Warnf("failed to register handler_invoke_operations_total metric: %v", err)
		}
		// Register histogram for invoke handler operation duration
		err = metrics.RegisterHistogram("handler_invoke_operation_duration_seconds", "Invoke handler operation duration in seconds", []string{"function_name"}, nil)
		if err != nil {
			log.GetLogger().Warnf("failed to register handler_invoke_operation_duration_seconds metric: %v", err)
		}
	})
}

func initKillMetrics() {
	killMetricsOnce.Do(func() {
		// Register counter for kill handler operations with http code
		err := metrics.RegisterCounter("handler_kill_operations_total", "Total number of kill handler operations", []string{"http_code"})
		if err != nil {
			log.GetLogger().Warnf("failed to register handler_kill_operations_total metric: %v", err)
		}
		// Register histogram for kill handler operation duration
		err = metrics.RegisterHistogram("handler_kill_operation_duration_seconds", "Kill handler operation duration in seconds", []string{}, nil)
		if err != nil {
			log.GetLogger().Warnf("failed to register handler_kill_operation_duration_seconds metric: %v", err)
		}
	})
}

// CreateHandler the handler of create
func CreateHandler(ctx *gin.Context) {
	// Initialize metrics on first call
	initCreateMetrics()

	startTime := time.Now()
	var httpCode int
	hasError := false
	var functionName string

	// Use defer to ensure metrics are reported even if function returns early
	defer func() {
		if httpCode == 0 {
			if hasError {
				httpCode = http.StatusInternalServerError
			} else {
				httpCode = http.StatusOK
			}
		}
		httpCodeStr := strconv.Itoa(httpCode)

		// Report operation count
		if err := metrics.IncrementCounter("handler_create_operations_total", functionName, httpCodeStr); err != nil {
			log.GetLogger().Debugf("failed to report handler_create_operations_total metric: %v", err)
		}

		// Report operation duration
		duration := time.Since(startTime)
		if err := metrics.ObserveHistogram("handler_create_operation_duration_seconds", duration.Seconds(), functionName); err != nil {
			log.GetLogger().Debugf("failed to report handler_create_operation_duration_seconds metric: %v", err)
		}
	}()

	remoteClientID, traceID := getHeaderPrams(ctx)
	log.GetLogger().Infof("%s|receive instance create request, remoteClientID: %s", traceID, remoteClientID)
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to read request body error %s", err.Error())
		httpCode = http.StatusInternalServerError
		hasError = true
		SetCtxResponse(ctx, nil, httpCode)
		return
	}
	functionName = "unknown"
	resp, err := util.NewClient().CreateInstanceRaw(body)
	log.GetLogger().Debugf("receive instance create response, msg: %s", resp)
	if err != nil {
		httpCode = http.StatusBadRequest
		hasError = true
		SetCtxResponse(ctx, []byte(err.Error()), httpCode)
		return
	}
	httpCode = http.StatusOK
	SetCtxResponse(ctx, resp, httpCode)
}

// InvokeHandler the handler of invoke
func InvokeHandler(ctx *gin.Context) {
	// Initialize metrics on first call
	initInvokeMetrics()

	startTime := time.Now()
	var httpCode int
	hasError := false
	var functionName string

	// Use defer to ensure metrics are reported even if function returns early
	defer func() {
		if httpCode == 0 {
			if hasError {
				httpCode = http.StatusInternalServerError
			} else {
				httpCode = http.StatusOK
			}
		}
		httpCodeStr := strconv.Itoa(httpCode)

		// Report operation count
		if err := metrics.IncrementCounter("handler_invoke_operations_total", functionName, httpCodeStr); err != nil {
			log.GetLogger().Debugf("failed to report handler_invoke_operations_total metric: %v", err)
		}

		// Report operation duration
		duration := time.Since(startTime)
		if err := metrics.ObserveHistogram("handler_invoke_operation_duration_seconds", duration.Seconds(), functionName); err != nil {
			log.GetLogger().Debugf("failed to report handler_invoke_operation_duration_seconds metric: %v", err)
		}
	}()

	remoteClientID, traceID := getHeaderPrams(ctx)
	log.GetLogger().Infof("%s|receive instance invoke request, remoteClientID: %s", traceID, remoteClientID)

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to read request body error %s", err.Error())
		httpCode = http.StatusInternalServerError
		hasError = true
		SetCtxResponse(ctx, nil, httpCode)
		return
	}
	functionName = "unknown"
	notify, err := util.NewClient().InvokeInstanceRaw(body)
	log.GetLogger().Debugf("receive instance invoke response, msg: %s", notify)
	if err != nil {
		httpCode = http.StatusBadRequest
		hasError = true
		SetCtxResponse(ctx, []byte(err.Error()), httpCode)
		return
	}
	httpCode = http.StatusOK
	SetCtxResponse(ctx, notify, httpCode)
}

// KillHandler the handler of kill
func KillHandler(ctx *gin.Context) {
	// Initialize metrics on first call
	initKillMetrics()

	startTime := time.Now()
	var httpCode int
	hasError := false

	// Use defer to ensure metrics are reported even if function returns early
	defer func() {
		if httpCode == 0 {
			if hasError {
				httpCode = http.StatusInternalServerError
			} else {
				httpCode = http.StatusOK
			}
		}
		httpCodeStr := strconv.Itoa(httpCode)

		// Report operation count
		if err := metrics.IncrementCounter("handler_kill_operations_total", httpCodeStr); err != nil {
			log.GetLogger().Debugf("failed to report handler_kill_operations_total metric: %v", err)
		}

		// Report operation duration
		duration := time.Since(startTime)
		if err := metrics.ObserveHistogram("handler_kill_operation_duration_seconds", duration.Seconds()); err != nil {
			log.GetLogger().Debugf("failed to report handler_kill_operation_duration_seconds metric: %v", err)
		}
	}()

	remoteClientID, traceID := getHeaderPrams(ctx)
	log.GetLogger().Infof("%s|receives instance kill request, remoteClientID: %s", traceID, remoteClientID)
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to read request body error %s", err.Error())
		httpCode = http.StatusInternalServerError
		hasError = true
		SetCtxResponse(ctx, nil, httpCode)
		return
	}
	resp, err := util.NewClient().KillRaw(body)
	log.GetLogger().Debugf("receive instance kill response, msg: %s", resp)
	if err != nil {
		httpCode = http.StatusBadRequest
		hasError = true
		SetCtxResponse(ctx, []byte(err.Error()), httpCode)
		return
	}
	httpCode = http.StatusOK
	SetCtxResponse(ctx, resp, httpCode)
}

func getHeaderPrams(ctx *gin.Context) (string, string) {
	remoteClientID := httputil.GetCompatibleGinHeader(ctx.Request, constant.HeaderRemoteClientId, "remoteClientId")
	traceID := httputil.GetCompatibleGinHeader(ctx.Request, constant.HeaderTraceID, "traceId")
	return remoteClientID, traceID
}

// SetCtxResponse set ctx response
func SetCtxResponse(ctx *gin.Context, body []byte, statusCode int) {
	if len(body) == 0 {
		log.GetLogger().Warnf("the body of ctx response is empty")
	}
	ctx.Writer.WriteHeader(statusCode)
	if serverstatus.IsShutdown() {
		ctx.Writer.Header().Set("Connection", "close")
	}
	if _, err := ctx.Writer.Write(body); err != nil {
		log.GetLogger().Errorf("failed to set response body in context error %s", err.Error())
	}
}
