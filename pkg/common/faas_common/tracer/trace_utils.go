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

package tracer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"frontend/pkg/common/faas_common/constant"
)

type rootTraceIDContextKey struct{}

// ExtractOtelTraceID extracts the 32-hex OTel trace ID from a custom trace ID.
// Supported formats:
//   - "job-{jobid}-trace-{32hex}"
//   - standard UUID, which is normalized by removing '-'
//
// Returns "" if format is invalid.
func ExtractOtelTraceID(customTraceID string) string {
	if traceID := extractUUIDTraceID(customTraceID); traceID != "" {
		return traceID
	}
	parts := strings.SplitN(customTraceID, "-trace-", 2)
	if len(parts) != 2 {
		return ""
	}
	hexPart := strings.ToLower(parts[1])
	if len(hexPart) != 32 || !isHexString(hexPart) {
		return ""
	}
	return hexPart
}

func extractUUIDTraceID(customTraceID string) string {
	if len(customTraceID) != 36 ||
		customTraceID[8] != '-' ||
		customTraceID[13] != '-' ||
		customTraceID[18] != '-' ||
		customTraceID[23] != '-' {
		return ""
	}
	normalized := strings.ToLower(strings.ReplaceAll(customTraceID, "-", ""))
	if !isHexString(normalized) {
		return ""
	}
	return normalized
}

func isHexString(value string) bool {
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// BuildParentContext builds an OTel context with a remote parent span from a custom trace ID.
// Uses a random SpanID since we only have the trace ID, not the upstream span ID.
// Returns the original context if the trace ID is invalid.
func BuildParentContext(ctx context.Context, customTraceID string) context.Context {
	otelHex := ExtractOtelTraceID(customTraceID)
	if otelHex == "" {
		return ctx
	}
	traceID, err := trace.TraceIDFromHex(otelHex)
	if err != nil {
		return ctx
	}
	var spanIDBytes [8]byte
	if _, err := rand.Read(spanIDBytes[:]); err != nil {
		return ctx
	}
	spanID, err := trace.SpanIDFromHex(hex.EncodeToString(spanIDBytes[:]))
	if err != nil {
		return ctx
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

func ContextWithRootTraceID(ctx context.Context, customTraceID string) context.Context {
	otelHex := ExtractOtelTraceID(customTraceID)
	if otelHex == "" {
		return ctx
	}
	traceID, err := trace.TraceIDFromHex(otelHex)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, rootTraceIDContextKey{}, traceID)
}

func RootTraceIDFromContext(ctx context.Context) (trace.TraceID, bool) {
	traceID, ok := ctx.Value(rootTraceIDContextKey{}).(trace.TraceID)
	if !ok || !traceID.IsValid() {
		return trace.TraceID{}, false
	}
	return traceID, true
}

// BuildInboundContext prefers a locally-derived root trace from X-Trace-Id/Request-Id.
// It only falls back to incoming traceparent when local identifiers cannot form a valid trace.
func BuildInboundContext(ctx context.Context, traceParent string, traceID string, requestID string) (context.Context, bool) {
	if fallbackID := ValidateTraceID(traceID); fallbackID != "" {
		if rootCtx := ContextWithRootTraceID(ctx, fallbackID); RootTraceIDIsValid(rootCtx) {
			return rootCtx, true
		}
	}
	if fallbackID := ValidateTraceID(requestID); fallbackID != "" {
		if rootCtx := ContextWithRootTraceID(ctx, fallbackID); RootTraceIDIsValid(rootCtx) {
			return rootCtx, true
		}
	}
	if traceParent == "" {
		return ctx, false
	}
	carrier := propagation.HeaderCarrier{}
	carrier.Set(constant.HeaderTraceParent, traceParent)
	return otel.GetTextMapPropagator().Extract(ctx, carrier), false
}

func RootTraceIDIsValid(ctx context.Context) bool {
	_, ok := RootTraceIDFromContext(ctx)
	return ok
}

// ValidateTraceID returns the raw trace ID if non-empty, otherwise "".
func ValidateTraceID(raw string) string {
	if raw == "" {
		return ""
	}
	return raw
}
