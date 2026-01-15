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

// Package metrics provides Prometheus metrics collection and HTTP endpoint
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"frontend/pkg/common/faas_common/logger/log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// promRegistry global Prometheus registry
	promRegistry *prometheus.Registry

	// metricsMap stores all registered metrics
	metricsMap sync.Map

	// httpServer HTTP server instance for Prometheus metrics
	httpServer *http.Server

	// serverMutex protects server start/stop operations
	serverMutex sync.Mutex
)

// MetricType metric type
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// MetricInfo metric information
type MetricInfo struct {
	Type        MetricType
	Counter     *prometheus.CounterVec
	Gauge       *prometheus.GaugeVec
	Histogram   *prometheus.HistogramVec
	Summary     *prometheus.SummaryVec
	Labels      []string
	Description string
}

func init() {
	promRegistry = prometheus.NewRegistry()
	// Register default Go runtime metrics
	promRegistry.MustRegister(collectors.NewGoCollector())
	promRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

// RegisterCounter registers a Counter metric
// name: metric name
// description: metric description
// labels: label names list
func RegisterCounter(name, description string, labels []string) error {
	if _, exists := metricsMap.Load(name); exists {
		return fmt.Errorf("metric %s already exists", name)
	}

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: description,
		},
		labels,
	)

	if err := promRegistry.Register(counter); err != nil {
		return fmt.Errorf("failed to register counter %s: %w", name, err)
	}

	metricsMap.Store(name, &MetricInfo{
		Type:        MetricTypeCounter,
		Counter:     counter,
		Labels:      labels,
		Description: description,
	})

	log.GetLogger().Infof("registered counter metric: %s", name)
	return nil
}

// RegisterGauge registers a Gauge metric
// name: metric name
// description: metric description
// labels: label names list
func RegisterGauge(name, description string, labels []string) error {
	if _, exists := metricsMap.Load(name); exists {
		return fmt.Errorf("metric %s already exists", name)
	}

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: description,
		},
		labels,
	)

	if err := promRegistry.Register(gauge); err != nil {
		return fmt.Errorf("failed to register gauge %s: %w", name, err)
	}

	metricsMap.Store(name, &MetricInfo{
		Type:        MetricTypeGauge,
		Gauge:       gauge,
		Labels:      labels,
		Description: description,
	})

	log.GetLogger().Infof("registered gauge metric: %s", name)
	return nil
}

// RegisterHistogram registers a Histogram metric
// name: metric name
// description: metric description
// labels: label names list
// buckets: histogram bucket configuration, uses default buckets if nil
func RegisterHistogram(name, description string, labels []string, buckets []float64) error {
	if _, exists := metricsMap.Load(name); exists {
		return fmt.Errorf("metric %s already exists", name)
	}

	opts := prometheus.HistogramOpts{
		Name: name,
		Help: description,
	}
	if buckets != nil {
		opts.Buckets = buckets
	}

	histogram := prometheus.NewHistogramVec(opts, labels)

	if err := promRegistry.Register(histogram); err != nil {
		return fmt.Errorf("failed to register histogram %s: %w", name, err)
	}

	metricsMap.Store(name, &MetricInfo{
		Type:        MetricTypeHistogram,
		Histogram:   histogram,
		Labels:      labels,
		Description: description,
	})

	log.GetLogger().Infof("registered histogram metric: %s", name)
	return nil
}

// RegisterSummary registers a Summary metric
// name: metric name
// description: metric description
// labels: label names list
// objectives: quantile objectives, uses default values if nil
func RegisterSummary(name, description string, labels []string, objectives map[float64]float64) error {
	if _, exists := metricsMap.Load(name); exists {
		return fmt.Errorf("metric %s already exists", name)
	}

	opts := prometheus.SummaryOpts{
		Name: name,
		Help: description,
	}
	if objectives != nil {
		opts.Objectives = objectives
	}

	summary := prometheus.NewSummaryVec(opts, labels)

	if err := promRegistry.Register(summary); err != nil {
		return fmt.Errorf("failed to register summary %s: %w", name, err)
	}

	metricsMap.Store(name, &MetricInfo{
		Type:        MetricTypeSummary,
		Summary:     summary,
		Labels:      labels,
		Description: description,
	})

	log.GetLogger().Infof("registered summary metric: %s", name)
	return nil
}

// IncrementCounter increments Counter metric value
// name: metric name
// labelValues: label values list, must match the order of labels when registered
func IncrementCounter(name string, labelValues ...string) error {
	value, exists := metricsMap.Load(name)
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	info := value.(*MetricInfo)
	if info.Type != MetricTypeCounter {
		return fmt.Errorf("metric %s is not a counter", name)
	}

	if len(labelValues) != len(info.Labels) {
		return fmt.Errorf("label values count mismatch for metric %s: expected %d, got %d",
			name, len(info.Labels), len(labelValues))
	}

	info.Counter.WithLabelValues(labelValues...).Inc()
	return nil
}

// AddCounter adds Counter metric value (with specified increment)
// name: metric name
// value: increment value
// labelValues: label values list
func AddCounter(name string, value float64, labelValues ...string) error {
	metricValue, exists := metricsMap.Load(name)
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	info := metricValue.(*MetricInfo)
	if info.Type != MetricTypeCounter {
		return fmt.Errorf("metric %s is not a counter", name)
	}

	if len(labelValues) != len(info.Labels) {
		return fmt.Errorf("label values count mismatch for metric %s: expected %d, got %d",
			name, len(info.Labels), len(labelValues))
	}

	info.Counter.WithLabelValues(labelValues...).Add(value)
	return nil
}

// SetGauge sets Gauge metric value
// name: metric name
// value: metric value
// labelValues: label values list
func SetGauge(name string, value float64, labelValues ...string) error {
	metricValue, exists := metricsMap.Load(name)
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	info := metricValue.(*MetricInfo)
	if info.Type != MetricTypeGauge {
		return fmt.Errorf("metric %s is not a gauge", name)
	}

	if len(labelValues) != len(info.Labels) {
		return fmt.Errorf("label values count mismatch for metric %s: expected %d, got %d",
			name, len(info.Labels), len(labelValues))
	}

	info.Gauge.WithLabelValues(labelValues...).Set(value)
	return nil
}

// AddGauge adds Gauge metric value
// name: metric name
// value: increment value
// labelValues: label values list
func AddGauge(name string, value float64, labelValues ...string) error {
	metricValue, exists := metricsMap.Load(name)
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	info := metricValue.(*MetricInfo)
	if info.Type != MetricTypeGauge {
		return fmt.Errorf("metric %s is not a gauge", name)
	}

	if len(labelValues) != len(info.Labels) {
		return fmt.Errorf("label values count mismatch for metric %s: expected %d, got %d",
			name, len(info.Labels), len(labelValues))
	}

	info.Gauge.WithLabelValues(labelValues...).Add(value)
	return nil
}

// ObserveHistogram records Histogram metric value
// name: metric name
// value: observed value
// labelValues: label values list
func ObserveHistogram(name string, value float64, labelValues ...string) error {
	metricValue, exists := metricsMap.Load(name)
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	info := metricValue.(*MetricInfo)
	if info.Type != MetricTypeHistogram {
		return fmt.Errorf("metric %s is not a histogram", name)
	}

	if len(labelValues) != len(info.Labels) {
		return fmt.Errorf("label values count mismatch for metric %s: expected %d, got %d",
			name, len(info.Labels), len(labelValues))
	}

	info.Histogram.WithLabelValues(labelValues...).Observe(value)
	return nil
}

// ObserveSummary records Summary metric value
// name: metric name
// value: observed value
// labelValues: label values list
func ObserveSummary(name string, value float64, labelValues ...string) error {
	metricValue, exists := metricsMap.Load(name)
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	info := metricValue.(*MetricInfo)
	if info.Type != MetricTypeSummary {
		return fmt.Errorf("metric %s is not a summary", name)
	}

	if len(labelValues) != len(info.Labels) {
		return fmt.Errorf("label values count mismatch for metric %s: expected %d, got %d",
			name, len(info.Labels), len(labelValues))
	}

	info.Summary.WithLabelValues(labelValues...).Observe(value)
	return nil
}

// GetRegistry gets Prometheus registry (for advanced usage)
func GetRegistry() *prometheus.Registry {
	return promRegistry
}

// GetMetricsHandler returns HTTP handler for Prometheus metrics endpoint
// This can be used to register the metrics endpoint in existing HTTP servers
func GetMetricsHandler() http.Handler {
	return promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// StartPrometheusServer starts Prometheus HTTP server
// address: listening address in "host:port" format, e.g. ":9090"
// path: metrics path, defaults to "/metrics"
func StartPrometheusServer(address, path string) error {
	serverMutex.Lock()
	defer serverMutex.Unlock()

	if httpServer != nil {
		return fmt.Errorf("Prometheus server is already running")
	}

	if path == "" {
		path = "/metrics"
	}

	mux := http.NewServeMux()
	handler := promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	mux.Handle(path, handler)

	httpServer = &http.Server{
		Addr:    address,
		Handler: mux,
	}

	go func() {
		log.GetLogger().Infof("Starting Prometheus metrics server on %s%s", address, path)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.GetLogger().Errorf("Prometheus metrics server error: %v", err)
		}
	}()

	return nil
}

// StopPrometheusServer stops Prometheus HTTP server
func StopPrometheusServer(ctx context.Context) error {
	serverMutex.Lock()
	defer serverMutex.Unlock()

	if httpServer == nil {
		return fmt.Errorf("Prometheus server is not running")
	}

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown Prometheus server: %w", err)
	}

	httpServer = nil
	log.GetLogger().Infof("Prometheus metrics server stopped")
	return nil
}
