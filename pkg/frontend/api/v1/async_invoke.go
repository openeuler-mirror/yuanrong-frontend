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

package v1

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/uuid"
	"frontend/pkg/frontend/asyncinvocation"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/types"
)

// AsyncInvokeHandler handles asynchronous function invocations.
// It immediately returns HTTP 202 with a requestId and processes the invocation in the background.
func AsyncInvokeHandler(ctx *gin.Context) {
	traceID := httputil.InitTraceID(ctx)
	logger := log.GetLogger()

	// Acquire worker slot (for concurrency control)
	workerPool := asyncinvocation.GetWorkerPool()
	workerPool.Acquire()

	// Update concurrent gauge
	asyncinvocation.IncConcurrent()

	// Determine if this is a short URL invocation by checking path pattern
	// Short URL: /invocations/:tenant-id/:namespace/:function/
	// Long URL: /serverless/v1/functions/:urn/invocations
	isShortUrl := ctx.Param("tenant-id") != "" && ctx.Param("namespace") != "" && ctx.Param("function") != ""

	var processCtx *types.InvokeProcessContext
	var err error
	if isShortUrl {
		processCtx, err = buildShortProcessContext(ctx, traceID)
	} else {
		processCtx, err = buildProcessContext(ctx, traceID)
	}
	if err != nil {
		logger.Errorf("async invoke: failed to build processCtx, traceID: %s, error: %s", traceID, err.Error())
		writeHTTPResponse(ctx, processCtx)
		return
	}

	requestID := uuid.New().String()
	startTime := time.Now()

	// Get webhook URL from request header
	webhookURL := ctx.GetHeader("X-Webhook-Url")

	// Use new storage backend
	storage := asyncinvocation.GetStorage()
	now := time.Now()
	storage.Store(context.Background(), requestID, &asyncinvocation.AsyncResult{
		RequestID:  requestID,
		Status:     asyncinvocation.StatusPending,
		CreatedAt:  now,
		InstanceID: getInstanceID(),
	})

	// Record invocation start
	asyncinvocation.RecordInvocation(asyncinvocation.StatusPending, processCtx.FuncKey)

	go func() {
		// Release worker slot and decrement concurrent count when done
		defer func() {
			workerPool.Release()
			asyncinvocation.DecConcurrent()
		}()

		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("async invoke: panic recovered, requestID: %s, panic: %v", requestID, r)
				now := time.Now()
				result, ok, _ := storage.Load(context.Background(), requestID)
				if ok {
					result.Status = asyncinvocation.StatusFailed
					result.Error = "internal panic"
					result.CompletedAt = &now
					storage.Store(context.Background(), requestID, result)
				}
				asyncinvocation.RecordInvocation(asyncinvocation.StatusFailed, processCtx.FuncKey)
			}
		}()

		// Update status to running
		result, ok, err := storage.Load(context.Background(), requestID)
		if !ok || err != nil {
			logger.Errorf("async invoke: failed to load result, requestID: %s, error: %v", requestID, err)
			return
		}
		result.Status = asyncinvocation.StatusRunning
		if err := storage.Store(context.Background(), requestID, result); err != nil {
			logger.Errorf("async invoke: failed to store result, requestID: %s, error: %v", requestID, err)
			return
		}
		asyncinvocation.RecordInvocation(asyncinvocation.StatusRunning, processCtx.FuncKey)

		// Execute the invocation
		invokeStart := time.Now()
		invokeErr := middleware.Invoker.Handle(processCtx)
		if invokeErr != nil {
			logger.Errorf("async invoke: invocation failed, requestID: %s, error: %s", requestID, invokeErr.Error())
		}

		// Record duration
		asyncinvocation.ObserveInvocationDuration(processCtx.FuncKey, invokeStart)

		now = time.Now()
		
		// Fix Critical #3: Check both invoke error and status code
		statusCode := processCtx.StatusCode
		if invokeErr != nil || statusCode == 0 || statusCode >= http.StatusBadRequest {
			result.Status = asyncinvocation.StatusFailed
			if invokeErr != nil {
				result.Error = invokeErr.Error()
			}
			asyncinvocation.RecordInvocation(asyncinvocation.StatusFailed, processCtx.FuncKey)
		} else {
			result.Status = asyncinvocation.StatusCompleted
			asyncinvocation.RecordInvocation(asyncinvocation.StatusCompleted, processCtx.FuncKey)
		}
		
		result.StatusCode = statusCode
		result.RespBody = processCtx.RespBody
		result.RespHeaders = processCtx.RespHeader
		result.CompletedAt = &now

		if err := storage.Store(context.Background(), requestID, result); err != nil {
			logger.Errorf("async invoke: failed to store final result, requestID: %s, error: %v", requestID, err)
			return
		}

		// Send webhook notification if configured
		if webhookURL != "" {
			payload := asyncinvocation.NewWebhookPayload(result)
			go func() {
				err := asyncinvocation.SendWebhook(context.Background(), webhookURL, payload)
				if err != nil {
					logger.Errorf("async invoke: webhook failed, requestID: %s, error: %v", requestID, err)
					asyncinvocation.RecordWebhook("failed")
				} else {
					asyncinvocation.RecordWebhook("success")
				}
			}()
		}

		totalDuration := time.Since(startTime)
		logger.Infof("async invoke: completed, requestID: %s, status: %s, duration: %v",
			requestID, result.Status, totalDuration)
	}()

	ctx.JSON(http.StatusAccepted, gin.H{"requestId": requestID})
}

// GetAsyncResultHandler returns the result of an asynchronous invocation.
func GetAsyncResultHandler(ctx *gin.Context) {
	requestID := ctx.Param("request-id")
	storage := asyncinvocation.GetStorage()

	result, ok, err := storage.Load(context.Background(), requestID)
	// Fix High #2: Properly handle storage errors
	if err != nil {
		log.GetLogger().Errorf("async result load error: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "failed to load async result: " + err.Error(),
		})
		return
	}
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "async result not found"})
		return
	}

	if result.Status == asyncinvocation.StatusPending || result.Status == asyncinvocation.StatusRunning {
		ctx.JSON(http.StatusOK, gin.H{
			"requestId": result.RequestID,
			"status":    result.Status,
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// getInstanceID returns the current instance ID.
func getInstanceID() string {
	// Try INSTANCE_ID first (K8s), then POD_NAME, fallback to hostname
	if id := os.Getenv("INSTANCE_ID"); id != "" {
		return id
	}
	if id := os.Getenv("POD_NAME"); id != "" {
		return id
	}
	// Fallback to hostname
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return "unknown"
}
