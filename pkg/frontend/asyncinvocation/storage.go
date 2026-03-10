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
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	log "frontend/pkg/common/faas_common/logger/log"
)

const (
	redisKeyPrefix = "async:result:"
)

// StorageBackend is the interface for async result storage.
type StorageBackend interface {
	Store(ctx context.Context, key string, result *AsyncResult) error
	Load(ctx context.Context, key string) (*AsyncResult, bool, error)
	Delete(ctx context.Context, key string) error
	Close() error
}

// MemoryBackend is an in-memory storage implementation using sync.Map.
type MemoryBackend struct {
	results    sync.Map
	maxAge     time.Duration
	cleanupCtx context.Context
	cancel     context.CancelFunc
}

// NewMemoryBackend creates a new MemoryBackend.
func NewMemoryBackend(maxAge time.Duration) *MemoryBackend {
	ctx, cancel := context.WithCancel(context.Background())
	backend := &MemoryBackend{
		maxAge:     maxAge,
		cleanupCtx: ctx,
		cancel:    cancel,
	}
	// Start cleanup goroutine
	go backend.cleanup()
	return backend
}

// cleanup periodically removes expired results
func (m *MemoryBackend) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.cleanupCtx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			m.results.Range(func(key, value interface{}) bool {
				if result, ok := value.(*AsyncResult); ok {
					if now.Sub(result.CreatedAt) > m.maxAge {
						m.results.Delete(key)
					}
				}
				return true
			})
		}
	}
}

// Close stops the cleanup goroutine
func (m *MemoryBackend) Close() error {
	m.cancel()
	return nil
}

// Store saves an async result to memory.
func (m *MemoryBackend) Store(ctx context.Context, key string, result *AsyncResult) error {
	// Fix Critical #2: Store a copy to avoid data race
	resultCopy := *result
	m.results.Store(key, &resultCopy)
	return nil
}

// Load retrieves an async result from memory.
func (m *MemoryBackend) Load(ctx context.Context, key string) (*AsyncResult, bool, error) {
	val, ok := m.results.Load(key)
	if !ok {
		return nil, false, nil
	}
	return val.(*AsyncResult), true, nil
}

// Delete removes an async result from memory.
func (m *MemoryBackend) Delete(ctx context.Context, key string) error {
	m.results.Delete(key)
	return nil
}

// RedisBackend is a Redis-based storage implementation.
type RedisBackend struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisBackend creates a new RedisBackend.
func NewRedisBackend(cfg RedisConfig, ttl time.Duration) *RedisBackend {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &RedisBackend{
		client: client,
		ttl:    ttl,
	}
}

// Store saves an async result to Redis.
func (r *RedisBackend) Store(ctx context.Context, key string, result *AsyncResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	return r.client.Set(ctx, redisKeyPrefix+key, data, r.ttl).Err()
}

// Load retrieves an async result from Redis.
func (r *RedisBackend) Load(ctx context.Context, key string) (*AsyncResult, bool, error) {
	data, err := r.client.Get(ctx, redisKeyPrefix+key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get result: %w", err)
	}
	var result AsyncResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal result: %w", err)
	}
	return &result, true, nil
}

// Delete removes an async result from Redis.
func (r *RedisBackend) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, redisKeyPrefix+key).Err()
}

// Close closes the Redis connection.
func (r *RedisBackend) Close() error {
	return r.client.Close()
}

// storageHolder holds the current storage backend.
var (
	storage      StorageBackend
	storageOnce  sync.Once
	storageClose func() error
)

// NewStorageBackend creates a new storage backend based on configuration.
func NewStorageBackend() StorageBackend {
	storageOnce.Do(func() {
		cfg := GetAsyncConfig()
		if cfg.Storage.Type == "redis" {
			log.GetLogger().Infof("Using Redis storage backend: %s", cfg.Storage.Redis.Addr)
			rb := NewRedisBackend(cfg.Storage.Redis, cfg.GetResultRetention())
			storage = rb
			storageClose = rb.Close
		} else {
			log.GetLogger().Info("Using memory storage backend")
			mb := NewMemoryBackend(cfg.GetResultRetention())
			storage = mb
			storageClose = mb.Close
		}
	})
	return storage
}

// GetStorage returns the current storage backend.
func GetStorage() StorageBackend {
	if storage == nil {
		return NewStorageBackend()
	}
	return storage
}

// CloseStorage closes the storage backend.
func CloseStorage() error {
	if storageClose != nil {
		return storageClose()
	}
	return nil
}
