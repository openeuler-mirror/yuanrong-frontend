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

package asyncinvocation

import (
	"sync"
	"time"

	"frontend/pkg/frontend/types"
)

// AsyncConfig holds the configuration for async invocation.
type AsyncConfig struct {
	Enabled                bool          `json:"enabled"`
	MaxConcurrent          int           `json:"maxConcurrent"`
	ResultRetentionMinutes int           `json:"resultRetentionMinutes"`
	CleanupIntervalMinutes int           `json:"cleanupIntervalMinutes"`
	Webhook                WebhookConfig `json:"webhook"`
	Storage                StorageConfig `json:"storage"`
}

// WebhookConfig holds the webhook configuration.
type WebhookConfig struct {
	Enabled       bool        `json:"enabled"`
	TimeoutSecond int         `json:"timeoutSecond"`
	Retry         RetryConfig `json:"retry"`
}

// RetryConfig holds the retry configuration for webhook.
type RetryConfig struct {
	MaxAttempts    int `json:"maxAttempts"`
	InitialDelayMs int `json:"initialDelayMs"`
}

// StorageConfig holds the storage configuration.
type StorageConfig struct {
	Type  string      `json:"type"` // redis or memory
	Redis RedisConfig `json:"redis"`
}

// RedisConfig holds the Redis configuration.
type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

var (
	configInstance *AsyncConfig
	configOnce     sync.Once
	configMu       sync.RWMutex
)

// DefaultAsyncConfig returns the default async invocation configuration.
func DefaultAsyncConfig() *AsyncConfig {
	return &AsyncConfig{
		Enabled:                true,
		MaxConcurrent:          1000,
		ResultRetentionMinutes: 60,
		CleanupIntervalMinutes: 5,
		Webhook: WebhookConfig{
			Enabled:       false,
			TimeoutSecond: 10,
			Retry: RetryConfig{
				MaxAttempts:    3,
				InitialDelayMs: 1000,
			},
		},
		Storage: StorageConfig{
			Type: "memory",
			Redis: RedisConfig{
				Addr:     "localhost:6379",
				Password: "",
				DB:       0,
			},
		},
	}
}

// GetAsyncConfig returns the async invocation configuration.
func GetAsyncConfig() *AsyncConfig {
	configOnce.Do(func() {
		configInstance = DefaultAsyncConfig()
	})
	configMu.RLock()
	defer configMu.RUnlock()
	return configInstance
}

// SetAsyncConfig sets the async invocation configuration.
func SetAsyncConfig(cfg *AsyncConfig) {
	configMu.Lock()
	defer configMu.Unlock()
	configInstance = cfg
}

// LoadConfigFromMain loads async invocation config from the main frontend config.
func LoadConfigFromMain(cfg *types.Config) {
	if cfg == nil || cfg.AsyncInvocation == nil {
		return
	}

	asyncCfg := &AsyncConfig{
		Enabled:                cfg.AsyncInvocation.Enabled,
		MaxConcurrent:          cfg.AsyncInvocation.MaxConcurrent,
		ResultRetentionMinutes: cfg.AsyncInvocation.ResultRetentionMinutes,
		CleanupIntervalMinutes: cfg.AsyncInvocation.CleanupIntervalMinutes,
		Webhook: WebhookConfig{
			Enabled:       cfg.AsyncInvocation.Webhook.Enabled,
			TimeoutSecond: cfg.AsyncInvocation.Webhook.TimeoutSeconds,
			Retry: RetryConfig{
				MaxAttempts:    cfg.AsyncInvocation.Webhook.Retry.MaxAttempts,
				InitialDelayMs: cfg.AsyncInvocation.Webhook.Retry.InitialDelayMs,
			},
		},
		Storage: StorageConfig{
			Type: cfg.AsyncInvocation.Storage.Type,
			Redis: RedisConfig{
				Addr:     cfg.AsyncInvocation.Storage.Redis.Addr,
				Password: cfg.AsyncInvocation.Storage.Redis.Password,
				DB:       cfg.AsyncInvocation.Storage.Redis.DB,
			},
		},
	}

	// Apply defaults for zero values
	if asyncCfg.MaxConcurrent == 0 {
		asyncCfg.MaxConcurrent = 1000
	}
	if asyncCfg.ResultRetentionMinutes == 0 {
		asyncCfg.ResultRetentionMinutes = 60
	}
	if asyncCfg.CleanupIntervalMinutes == 0 {
		asyncCfg.CleanupIntervalMinutes = 5
	}
	if asyncCfg.Webhook.TimeoutSecond == 0 {
		asyncCfg.Webhook.TimeoutSecond = 10
	}
	if asyncCfg.Webhook.Retry.MaxAttempts == 0 {
		asyncCfg.Webhook.Retry.MaxAttempts = 3
	}
	if asyncCfg.Webhook.Retry.InitialDelayMs == 0 {
		asyncCfg.Webhook.Retry.InitialDelayMs = 1000
	}
	if asyncCfg.Storage.Type == "" {
		asyncCfg.Storage.Type = "memory"
	}
	if asyncCfg.Storage.Redis.Addr == "" {
		asyncCfg.Storage.Redis.Addr = "localhost:6379"
	}

	SetAsyncConfig(asyncCfg)
}

// GetResultRetention returns the result retention duration.
func (c *AsyncConfig) GetResultRetention() time.Duration {
	return time.Duration(c.ResultRetentionMinutes) * time.Minute
}

// GetCleanupInterval returns the cleanup interval duration.
func (c *AsyncConfig) GetCleanupInterval() time.Duration {
	return time.Duration(c.CleanupIntervalMinutes) * time.Minute
}
