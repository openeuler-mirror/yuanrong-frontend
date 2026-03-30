/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestExtractOtelTraceID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		expects string
	}{
		{
			name:    "legacy trace id",
			input:   "job-123-trace-123e4567e89b12d3a456426614174000",
			expects: "123e4567e89b12d3a456426614174000",
		},
		{
			name:    "uuid trace id",
			input:   "123E4567-E89B-12D3-A456-426614174000",
			expects: "123e4567e89b12d3a456426614174000",
		},
		{
			name:    "invalid trace id",
			input:   "trace-123",
			expects: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expects, ExtractOtelTraceID(tt.input))
		})
	}
}

func TestBuildParentContextWithUUIDTraceID(t *testing.T) {
	ctx := BuildParentContext(context.Background(), "123e4567-e89b-12d3-a456-426614174000")
	spanContext := trace.SpanContextFromContext(ctx)

	assert.True(t, spanContext.IsValid())
	assert.True(t, spanContext.IsRemote())
	assert.Equal(t, "123e4567e89b12d3a456426614174000", spanContext.TraceID().String())
}

func TestContextWithRootTraceID(t *testing.T) {
	ctx := ContextWithRootTraceID(context.Background(), "123e4567-e89b-12d3-a456-426614174000")
	traceID, ok := RootTraceIDFromContext(ctx)

	assert.True(t, ok)
	assert.Equal(t, "123e4567e89b12d3a456426614174000", traceID.String())
}
