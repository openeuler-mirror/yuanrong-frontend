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
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "frontend/pkg/common/faas_common/logger/log"
)

var (
	invocationCounter   *prometheus.CounterVec
	invocationHistogram *prometheus.HistogramVec
	invocationGauge     prometheus.Gauge
	webhookCounter      *prometheus.CounterVec

	metricsOnce sync.Once

	// concurrentCount atomically tracks the current concurrent invocations
	concurrentCount int64
)

func initMetrics() {
	metricsOnce.Do(func() {
		// async_invocation_total{status, function_name}
		invocationCounter = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "async_invocation_total",
				Help: "Total number of async invocations",
			},
			[]string{"status", "function_name"},
		)

		// async_invocation_duration_seconds{function_name}
		invocationHistogram = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "async_invocation_duration_seconds",
				Help:    "Async invocation duration in seconds",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"function_name"},
		)

		// async_invocation_concurrent
		invocationGauge = promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "async_invocation_concurrent",
				Help: "Current number of concurrent async invocations",
			},
		)

		// async_webhook_total{status}
		webhookCounter = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "async_webhook_total",
				Help: "Total number of webhook notifications",
			},
			[]string{"status"},
		)

		log.GetLogger().Info("Async invocation metrics initialized")
	})
}

// RecordInvocation records an async invocation event.
func RecordInvocation(status, functionName string) {
	initMetrics()
	invocationCounter.WithLabelValues(status, functionName).Inc()
}

// RecordInvocationDuration records the async invocation duration.
func RecordInvocationDuration(functionName string, duration time.Duration) {
	initMetrics()
	invocationHistogram.WithLabelValues(functionName).Observe(duration.Seconds())
}

// RecordConcurrent sets the current concurrent invocation count.
func RecordConcurrent(count int64) {
	initMetrics()
	invocationGauge.Set(float64(count))
	atomic.StoreInt64(&concurrentCount, count)
}

// IncConcurrent increments the concurrent count.
func IncConcurrent() {
	initMetrics()
	invocationGauge.Inc()
	atomic.AddInt64(&concurrentCount, 1)
}

// DecConcurrent decrements the concurrent count.
func DecConcurrent() {
	initMetrics()
	invocationGauge.Dec()
	atomic.AddInt64(&concurrentCount, -1)
}

// RecordWebhook records a webhook notification event.
func RecordWebhook(status string) {
	initMetrics()
	webhookCounter.WithLabelValues(status).Inc()
}

// GetConcurrentCount returns the current concurrent count.
func GetConcurrentCount() int64 {
	return atomic.LoadInt64(&concurrentCount)
}

// ObserveInvocationDuration records duration with string function name.
func ObserveInvocationDuration(functionName string, startTime time.Time) {
	duration := time.Since(startTime)
	RecordInvocationDuration(functionName, duration)
}

// NewMetricLabels creates metric labels from function name.
func NewMetricLabels(functionName string) map[string]string {
	// Truncate function name if too long (Prometheus label limit is 1024)
	if len(functionName) > 256 {
		functionName = functionName[:256]
	}
	return map[string]string{"function_name": functionName}
}

// LabelsFromMap converts a map to Prometheus label values.
func LabelsFromMap(labels map[string]string) (status, functionName string) {
	status = labels["status"]
	functionName = labels["function_name"]
	if functionName == "" {
		functionName = "unknown"
	}
	if status == "" {
		status = "unknown"
	}
	return status, functionName
}

// ParseStatusFromString parses status string for metrics.
func ParseStatusFromString(statusStr string) string {
	switch statusStr {
	case StatusPending, StatusRunning, StatusCompleted, StatusFailed:
		return statusStr
	default:
		return "unknown"
	}
}

// FormatStatusForMetrics formats status for Prometheus labels.
func FormatStatusForMetrics(statusCode int, hasError bool) string {
	if hasError {
		return StatusFailed
	}
	switch {
	case statusCode >= 200 && statusCode < 300:
		return StatusCompleted
	case statusCode >= 400:
		return StatusFailed
	default:
		return StatusRunning
	}
}

// strconv for metrics - helper to avoid allocations.
func formatInt64(n int64) string {
	return strconv.FormatInt(n, 10)
}
