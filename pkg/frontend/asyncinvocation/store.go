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

// Package asyncinvocation -
package asyncinvocation

import (
	"sync"
	"time"

	log "frontend/pkg/common/faas_common/logger/log"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"

	defaultCleanupInterval = 5 * time.Minute
	defaultMaxAge          = 1 * time.Hour
)

// AsyncResult represents the result of an asynchronous invocation.
type AsyncResult struct {
	RequestID   string            `json:"requestId"`
	Status      string            `json:"status"`
	StatusCode  int               `json:"statusCode,omitempty"`
	RespHeaders map[string]string `json:"respHeaders,omitempty"`
	RespBody    []byte            `json:"respBody,omitempty"`
	Error       string            `json:"error,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	CompletedAt *time.Time       `json:"completedAt,omitempty"`
	InstanceID  string            `json:"instanceId,omitempty"` // For distributed tracking
}

// AsyncResultStore provides thread-safe storage for async invocation results.
type AsyncResultStore struct {
	results sync.Map
}

var (
	storeInstance *AsyncResultStore
	storeOnce    sync.Once
)

// GetAsyncResultStore returns the global singleton AsyncResultStore.
func GetAsyncResultStore() *AsyncResultStore {
	storeOnce.Do(func() {
		storeInstance = &AsyncResultStore{}
		storeInstance.StartCleanup(defaultCleanupInterval, defaultMaxAge)
	})
	return storeInstance
}

// Store saves an async result.
func (s *AsyncResultStore) Store(requestID string, result *AsyncResult) {
	s.results.Store(requestID, result)
}

// Load retrieves an async result by request ID.
func (s *AsyncResultStore) Load(requestID string) (*AsyncResult, bool) {
	val, ok := s.results.Load(requestID)
	if !ok {
		return nil, false
	}
	return val.(*AsyncResult), true
}

// Delete removes an async result by request ID.
func (s *AsyncResultStore) Delete(requestID string) {
	s.results.Delete(requestID)
}

// StartCleanup launches a background goroutine that periodically removes expired results.
func (s *AsyncResultStore) StartCleanup(interval, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			s.results.Range(func(key, value interface{}) bool {
				result := value.(*AsyncResult)
				if now.Sub(result.CreatedAt) > maxAge {
					s.results.Delete(key)
					log.GetLogger().Infof("Cleaned up expired async result: %s", key)
				}
				return true
			})
		}
	}()
}
