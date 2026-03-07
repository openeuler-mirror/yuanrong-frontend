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
	"sync/atomic"
)

// WorkerPool is a semaphore-based worker pool for limiting concurrent async invocations.
type WorkerPool struct {
	sem       chan struct{}
	maxCount  int64
	current   int64
	mu        sync.Mutex
}

// NewWorkerPool creates a new WorkerPool with the specified maximum concurrent count.
func NewWorkerPool(maxConcurrent int) *WorkerPool {
	return &WorkerPool{
		sem:      make(chan struct{}, maxConcurrent),
		maxCount: int64(maxConcurrent),
		current:  0,
	}
}

// Acquire acquires a slot from the pool. It blocks until a slot is available.
func (wp *WorkerPool) Acquire() {
	wp.sem <- struct{}{}
	atomic.AddInt64(&wp.current, 1)
}

// Release releases a slot back to the pool.
func (wp *WorkerPool) Release() {
	<-wp.sem
	atomic.AddInt64(&wp.current, -1)
}

// CurrentCount returns the current number of active workers.
func (wp *WorkerPool) CurrentCount() int64 {
	return atomic.LoadInt64(&wp.current)
}

// MaxCount returns the maximum number of concurrent workers.
func (wp *WorkerPool) MaxCount() int64 {
	return wp.maxCount
}

// workerPoolHolder holds the global worker pool.
var (
	globalWorkerPool *WorkerPool
	workerPoolOnce  sync.Once
)

// GetWorkerPool returns the global worker pool.
func GetWorkerPool() *WorkerPool {
	workerPoolOnce.Do(func() {
		cfg := GetAsyncConfig()
		globalWorkerPool = NewWorkerPool(cfg.MaxConcurrent)
	})
	return globalWorkerPool
}

// SetWorkerPool sets the global worker pool (for testing).
func SetWorkerPool(pool *WorkerPool) {
	globalWorkerPool = pool
}
