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

package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestRegisterCounter(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_counter")

	err := RegisterCounter("test_counter", "Test counter metric", []string{"label1", "label2"})
	assert.NoError(t, err)

	// Test duplicate registration
	err = RegisterCounter("test_counter", "Test counter metric", []string{"label1", "label2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Clean up
	metricsMap.Delete("test_counter")
}

func TestRegisterGauge(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_gauge")

	err := RegisterGauge("test_gauge", "Test gauge metric", []string{"label1"})
	assert.NoError(t, err)

	// Test duplicate registration
	err = RegisterGauge("test_gauge", "Test gauge metric", []string{"label1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Clean up
	metricsMap.Delete("test_gauge")
}

func TestRegisterHistogram(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_histogram")

	// Test with default buckets
	err := RegisterHistogram("test_histogram", "Test histogram metric", []string{"label1"}, nil)
	assert.NoError(t, err)

	// Test duplicate registration
	err = RegisterHistogram("test_histogram", "Test histogram metric", []string{"label1"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Clean up
	metricsMap.Delete("test_histogram")

	// Test with custom buckets
	err = RegisterHistogram("test_histogram_custom", "Test histogram with custom buckets", []string{"label1"}, []float64{0.1, 0.5, 1.0})
	assert.NoError(t, err)

	// Clean up
	metricsMap.Delete("test_histogram_custom")
}

func TestRegisterSummary(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_summary")

	// Test with default objectives
	err := RegisterSummary("test_summary", "Test summary metric", []string{"label1"}, nil)
	assert.NoError(t, err)

	// Test duplicate registration
	err = RegisterSummary("test_summary", "Test summary metric", []string{"label1"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Clean up
	metricsMap.Delete("test_summary")

	// Test with custom objectives
	objectives := map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	err = RegisterSummary("test_summary_custom", "Test summary with custom objectives", []string{"label1"}, objectives)
	assert.NoError(t, err)

	// Clean up
	metricsMap.Delete("test_summary_custom")
}

func TestIncrementCounter(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_counter_inc")

	// Register a counter
	err := RegisterCounter("test_counter_inc", "Test counter for increment", []string{"method", "status"})
	assert.NoError(t, err)

	// Test increment
	err = IncrementCounter("test_counter_inc", "GET", "200")
	assert.NoError(t, err)

	// Test increment again
	err = IncrementCounter("test_counter_inc", "GET", "200")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = IncrementCounter("test_counter_inc", "GET")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Test with non-existent metric
	err = IncrementCounter("non_existent", "GET", "200")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Clean up
	metricsMap.Delete("test_counter_inc")
}

func TestAddCounter(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_counter_add")

	// Register a counter
	err := RegisterCounter("test_counter_add", "Test counter for add", []string{"method"})
	assert.NoError(t, err)

	// Test add
	err = AddCounter("test_counter_add", 5.0, "POST")
	assert.NoError(t, err)

	// Test add again
	err = AddCounter("test_counter_add", 3.0, "POST")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = AddCounter("test_counter_add", 1.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Test with non-existent metric
	err = AddCounter("non_existent", 1.0, "GET")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Clean up
	metricsMap.Delete("test_counter_add")
}

func TestSetGauge(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_gauge_set")

	// Register a gauge
	err := RegisterGauge("test_gauge_set", "Test gauge for set", []string{"service"})
	assert.NoError(t, err)

	// Test set
	err = SetGauge("test_gauge_set", 10.0, "api")
	assert.NoError(t, err)

	// Test set again
	err = SetGauge("test_gauge_set", 20.0, "api")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = SetGauge("test_gauge_set", 15.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Test with non-existent metric
	err = SetGauge("non_existent", 10.0, "api")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test with wrong metric type
	err = SetGauge("test_counter_inc", 10.0, "GET", "200")
	if err == nil {
		// If test_counter_inc still exists from previous test, it should fail
		assert.Error(t, err)
	}

	// Clean up
	metricsMap.Delete("test_gauge_set")
}

func TestAddGauge(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_gauge_add")

	// Register a gauge
	err := RegisterGauge("test_gauge_add", "Test gauge for add", []string{"service"})
	assert.NoError(t, err)

	// Set initial value
	err = SetGauge("test_gauge_add", 10.0, "api")
	assert.NoError(t, err)

	// Test add
	err = AddGauge("test_gauge_add", 5.0, "api")
	assert.NoError(t, err)

	// Test add again
	err = AddGauge("test_gauge_add", -2.0, "api")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = AddGauge("test_gauge_add", 1.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Test with non-existent metric
	err = AddGauge("non_existent", 1.0, "api")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Clean up
	metricsMap.Delete("test_gauge_add")
}

func TestObserveHistogram(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_histogram_observe")

	// Register a histogram
	err := RegisterHistogram("test_histogram_observe", "Test histogram for observe", []string{"operation"}, nil)
	assert.NoError(t, err)

	// Test observe
	err = ObserveHistogram("test_histogram_observe", 0.5, "read")
	assert.NoError(t, err)

	// Test observe again
	err = ObserveHistogram("test_histogram_observe", 1.2, "read")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = ObserveHistogram("test_histogram_observe", 0.5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Test with non-existent metric
	err = ObserveHistogram("non_existent", 0.5, "read")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Clean up
	metricsMap.Delete("test_histogram_observe")
}

func TestObserveSummary(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_summary_observe")

	// Register a summary
	err := RegisterSummary("test_summary_observe", "Test summary for observe", []string{"operation"}, nil)
	assert.NoError(t, err)

	// Test observe
	err = ObserveSummary("test_summary_observe", 0.5, "write")
	assert.NoError(t, err)

	// Test observe again
	err = ObserveSummary("test_summary_observe", 1.2, "write")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = ObserveSummary("test_summary_observe", 0.5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Test with non-existent metric
	err = ObserveSummary("non_existent", 0.5, "write")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Clean up
	metricsMap.Delete("test_summary_observe")
}

func TestGetRegistry(t *testing.T) {
	registry := GetRegistry()
	assert.NotNil(t, registry)
	assert.IsType(t, &prometheus.Registry{}, registry)
}

func TestGetMetricsHandler(t *testing.T) {
	handler := GetMetricsHandler()
	assert.NotNil(t, handler)

	// Test that handler can serve HTTP requests
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "# HELP")
}

func TestMetricTypeErrors(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_counter_type")
	metricsMap.Delete("test_gauge_type")

	// Register counter
	err := RegisterCounter("test_counter_type", "Test counter", []string{"label1"})
	assert.NoError(t, err)

	// Try to use counter as gauge
	err = SetGauge("test_counter_type", 10.0, "value1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not a gauge")

	// Register gauge
	err = RegisterGauge("test_gauge_type", "Test gauge", []string{"label1"})
	assert.NoError(t, err)

	// Try to use gauge as counter
	err = IncrementCounter("test_gauge_type", "value1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not a counter")

	// Clean up
	metricsMap.Delete("test_counter_type")
	metricsMap.Delete("test_gauge_type")
}

func TestEmptyLabels(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_empty_labels")

	// Register counter with no labels
	err := RegisterCounter("test_empty_labels", "Test counter with no labels", []string{})
	assert.NoError(t, err)

	// Test increment with no labels
	err = IncrementCounter("test_empty_labels")
	assert.NoError(t, err)

	// Clean up
	metricsMap.Delete("test_empty_labels")
}

func TestMultipleLabelValues(t *testing.T) {
	// Clean up before test
	metricsMap.Delete("test_multi_labels")

	// Register counter with multiple labels
	err := RegisterCounter("test_multi_labels", "Test counter with multiple labels", []string{"method", "status", "endpoint"})
	assert.NoError(t, err)

	// Test increment with multiple label values
	err = IncrementCounter("test_multi_labels", "GET", "200", "/api/users")
	assert.NoError(t, err)

	// Test with wrong number of labels
	err = IncrementCounter("test_multi_labels", "GET", "200")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "label values count mismatch")

	// Clean up
	metricsMap.Delete("test_multi_labels")
}
