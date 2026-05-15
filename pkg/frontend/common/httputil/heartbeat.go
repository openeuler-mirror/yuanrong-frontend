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

package httputil

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/frontend/config"
)

const defaultProcessingHeartbeatInterval = 30 // seconds

// StartProcessingHeartbeat sends periodic HTTP 102 Processing responses to keep the
// connection alive through intermediate proxies (e.g., VIP) that have read timeouts.
// Returns a stop function that must be called before writing the final response.
//
// It extracts the underlying http.ResponseWriter via Unwrap() to bypass gin's
// WriteHeader tracking, then periodically calls WriteHeader(102). Go 1.20+ supports
// multiple 1xx WriteHeader calls before the final status code.
func StartProcessingHeartbeat(ctx *gin.Context, logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
}) func() {
	interval := config.GetConfig().HTTPConfig.ProcessingHeartbeatInterval
	if interval <= 0 {
		interval = defaultProcessingHeartbeatInterval
	}

	// Extract the underlying http.ResponseWriter to bypass gin's WriteHeader tracking.
	// gin's concrete responseWriter has Unwrap() even though the interface doesn't declare it.
	type unwrapper interface {
		Unwrap() http.ResponseWriter
	}
	uw, ok := ctx.Writer.(unwrapper)
	if !ok {
		logger.Infof("processing heartbeat: Unwrap not supported, heartbeat disabled")
		return func() {}
	}
	rawWriter := uw.Unwrap()

	done := make(chan struct{})
	var mu sync.Mutex // serialize 102 writes with the final response

	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mu.Lock()
				// Go 1.20+ http.ResponseWriter supports multiple WriteHeader(1xx)
				// calls before the final status code.
				rawWriter.WriteHeader(http.StatusProcessing) // 102
				if f, ok := rawWriter.(http.Flusher); ok {
					f.Flush()
				}
				mu.Unlock()
			}
		}
	}()

	return func() {
		close(done)
		// Ensure any in-flight 102 write completes before the caller writes the final response.
		mu.Lock()
		mu.Unlock()
	}
}
