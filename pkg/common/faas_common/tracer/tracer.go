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

// Package tracer for init trace provider
package tracer

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"frontend/pkg/common/faas_common/logger/log"
)

const (
	EnableTraceEnvKey = "ENABLE_TRACE"
	TraceConfigEnvKey = "TRACE_CONFIG"
	// OtelGRPCEndpointEnvKey -
	OtelGRPCEndpointEnvKey = "OTEL_GRPC_ENDPOINT"
	// OtelGRPCTokenEnvKey -
	OtelGRPCTokenEnvKey = "OTEL_GRPC_TOKEN"
	// OtelServiceNameEnvKey -
	OtelServiceNameEnvKey = "OTEL_SERVICE_NAME"
	// OtelEnableSampleEnvKey -
	OtelEnableSampleEnvKey = "OTEL_ENABLE_SAMPLE"
)

var (
	hostIP           = os.Getenv("HOST_IP")
	hostName         = os.Getenv("HOSTNAME")
	otelGRPCEndpoint = os.Getenv(OtelGRPCEndpointEnvKey)
	otelGRPCToken    = os.Getenv(OtelGRPCTokenEnvKey)
	otelServiceName  = os.Getenv(OtelServiceNameEnvKey)
	enableOTELTracer = os.Getenv(OtelEnableSampleEnvKey) == "true"
)

type otlpGrpcExporterConfig struct {
	Enable   bool   `json:"enable"`
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type traceConfig struct {
	OTLPGrpcExporter otlpGrpcExporterConfig `json:"otlpGrpcExporter"`
}

func loadCommonTracerConfig(serviceName string) error {
	otelServiceName = serviceName
	otelGRPCEndpoint = ""
	otelGRPCToken = ""
	enableOTELTracer = false

	if strings.EqualFold(os.Getenv(EnableTraceEnvKey), "true") {
		rawTraceConfig := os.Getenv(TraceConfigEnvKey)
		if rawTraceConfig != "" {
			var cfg traceConfig
			if err := json.Unmarshal([]byte(rawTraceConfig), &cfg); err != nil {
				return err
			}
			if cfg.OTLPGrpcExporter.Enable && cfg.OTLPGrpcExporter.Endpoint != "" {
				otelGRPCEndpoint = cfg.OTLPGrpcExporter.Endpoint
				otelGRPCToken = cfg.OTLPGrpcExporter.Token
				enableOTELTracer = true
			}
			return nil
		}
	}

	otelGRPCEndpoint = os.Getenv(OtelGRPCEndpointEnvKey)
	otelGRPCToken = os.Getenv(OtelGRPCTokenEnvKey)
	enableOTELTracer = strings.EqualFold(os.Getenv(OtelEnableSampleEnvKey), "true")
	if serviceName == "" {
		otelServiceName = os.Getenv(OtelServiceNameEnvKey)
	}
	return nil
}

// GetOtelGRPCEndpoint -
func GetOtelGRPCEndpoint() string {
	return otelGRPCEndpoint
}

// GetOtelGRPCToken -
func GetOtelGRPCToken() string {
	return otelGRPCToken
}

// GetOtelServiceName -
func GetOtelServiceName() string {
	return otelServiceName
}

// EnableOTELTracer -
func EnableOTELTracer() bool {
	return enableOTELTracer
}

// EnableCommonTracer -
func EnableCommonTracer() bool {
	return enableOTELTracer && otelGRPCEndpoint != ""
}

// InitCommonTracer init common tracer with service name
func InitCommonTracer(shutdown func(), serviceName string) {
	if err := loadCommonTracerConfig(serviceName); err != nil {
		fmt.Printf("failed to parse %s with error %s\n", TraceConfigEnvKey, err.Error())
		log.GetLogger().Warnf("failed to parse %s with error %s", TraceConfigEnvKey, err.Error())
		return
	}
	var err error
	shutdown, err = InitProvider(context.Background())
	if err != nil {
		fmt.Printf("failed to init %s trace provider with error %s\n", serviceName, err.Error())
		log.GetLogger().Warnf("failed to init %s trace provider with error %s", serviceName, err.Error())
		return
	}
}

// InitProvider init provider for trace http request
func InitProvider(ctx context.Context) (func(), error) {
	if !EnableCommonTracer() {
		fmt.Println("otel tracer env is empty with ", hostName, otelGRPCEndpoint)
		log.GetLogger().Warnf("otel tracer env is empty with %s, %s", hostName, otelGRPCEndpoint)
		return func() {}, nil
	}
	start := time.Now()
	fmt.Println("start to init provider for otel tracer with ", otelGRPCEndpoint)
	log.GetLogger().Infof("start to init provider for otel tracer with %s", otelGRPCEndpoint)
	traceExporter, err := makeTracerExporter(ctx)
	if err != nil {
		return func() {}, err
	}
	tracerProvider, err := makeTraceProvider(ctx, traceExporter)
	if err != nil {
		return func() {}, err
	}

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tracerProvider)
	fmt.Println("succeed to init provider for ", otelGRPCEndpoint, otelServiceName, time.Since(start).String())
	log.GetLogger().Infof("succeed to init provider for %s with %s cost %s",
		otelGRPCEndpoint, otelServiceName, time.Since(start).String())

	return func() {
		cxt, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := traceExporter.Shutdown(cxt); err != nil {
			otel.Handle(err)
		}
	}, nil
}

func makeTracerExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	headers := map[string]string{}
	if otelGRPCToken != "" {
		headers = map[string]string{"Authentication": otelGRPCToken}
	}
	traceGRPCClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(otelGRPCEndpoint),
		otlptracegrpc.WithHeaders(headers),
		otlptracegrpc.WithDialOption(grpc.WithBlock()))

	traceExporter, err := otlptrace.New(ctx, traceGRPCClient)
	if err != nil {
		log.GetLogger().Warnf("failed to create the collector trace exporter with %s", err.Error())
		return nil, err
	}
	return traceExporter, nil
}

type contextAwareIDGenerator struct{}

func (g *contextAwareIDGenerator) NewIDs(ctx context.Context) (oteltrace.TraceID, oteltrace.SpanID) {
	var spanID oteltrace.SpanID
	if _, err := rand.Read(spanID[:]); err != nil {
		return oteltrace.TraceID{}, oteltrace.SpanID{}
	}
	if traceID, ok := RootTraceIDFromContext(ctx); ok {
		return traceID, spanID
	}
	var traceID oteltrace.TraceID
	if _, err := rand.Read(traceID[:]); err != nil {
		return oteltrace.TraceID{}, oteltrace.SpanID{}
	}
	return traceID, spanID
}

func (g *contextAwareIDGenerator) NewSpanID(ctx context.Context, traceID oteltrace.TraceID) oteltrace.SpanID {
	var spanID oteltrace.SpanID
	if _, err := rand.Read(spanID[:]); err != nil {
		return oteltrace.SpanID{}
	}
	return spanID
}

func makeTraceProvider(ctx context.Context, traceExporter *otlptrace.Exporter) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(otelServiceName),
			semconv.HostNameKey.String(hostName),
			semconv.NetHostIPKey.String(hostIP),
		),
	)
	if err != nil {
		log.GetLogger().Warnf("failed to create otel resource with %s", err.Error())
		return nil, err
	}

	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	sample := sdktrace.NeverSample()
	if enableOTELTracer {
		sample = sdktrace.AlwaysSample()
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sample),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithIDGenerator(&contextAwareIDGenerator{}),
	)
	return tracerProvider, nil
}
