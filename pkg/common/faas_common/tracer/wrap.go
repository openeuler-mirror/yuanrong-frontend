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

// Package tracer for gin and fast http
package tracer

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"frontend/pkg/common/faas_common/constant"
)

// WrapGinHandler wrap gin handler
func WrapGinHandler(originHandlerFunc func(c *gin.Context)) func(c *gin.Context) {
	return func(c *gin.Context) {
		if EnableCommonTracer() {
			path := c.Request.URL.Path
			tracerName := otelServiceName
			method := c.Request.Method
			tr := otel.Tracer(tracerName)
			traceParent := c.Request.Header.Get(constant.HeaderTraceParent)
			propagattor := otel.GetTextMapPropagator()
			pCtx, forceRoot := BuildInboundContext(
				c.Request.Context(),
				traceParent,
				c.Request.Header.Get(constant.HeaderTraceID),
				c.Request.Header.Get(constant.HeaderRequestID),
			)
			options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindServer)}
			if forceRoot {
				options = append(options, trace.WithNewRoot())
			}

			childCtx, span := tr.Start(pCtx, path, options...)
			span.SetAttributes(attribute.Key("http.target").String(path))
			span.SetAttributes(attribute.Key("http.method").String(method))
			span.SetAttributes(attribute.Key("http.requestID").String(c.Request.Header.Get(constant.HeaderRequestID)))
			span.SetAttributes(attribute.Key("http.traceID").String(c.Request.Header.Get(constant.HeaderTraceID)))
			defer span.End()
			// set child ctx to request header by carrier
			c.Request = c.Request.WithContext(childCtx)
			childCarrier := propagation.HeaderCarrier{}
			propagattor.Inject(childCtx, childCarrier)
			c.Request.Header.Set(constant.HeaderTraceParent, childCarrier.Get(constant.HeaderTraceParent))
		}
		// call origin handler function
		originHandlerFunc(c)
	}
}

// WrapFastHTTPHandler wrap fast http handler
func WrapFastHTTPHandler(originHandlerFunc func(ctx *fasthttp.RequestCtx)) func(ctx *fasthttp.RequestCtx) {
	return func(ctx *fasthttp.RequestCtx) {
		if EnableCommonTracer() {
			path := ctx.Request.URI().String()
			tracerName := otelServiceName
			method := string(ctx.Method())
			tr := otel.Tracer(tracerName)
			traceParent := string(ctx.Request.Header.Peek(constant.HeaderTraceParent))
			propagattor := otel.GetTextMapPropagator()
			pCtx, forceRoot := BuildInboundContext(
				context.Background(),
				traceParent,
				string(ctx.Request.Header.Peek(constant.HeaderTraceID)),
				string(ctx.Request.Header.Peek(constant.HeaderRequestID)),
			)
			options := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindServer)}
			if forceRoot {
				options = append(options, trace.WithNewRoot())
			}

			childCtx, span := tr.Start(pCtx, path, options...)
			span.SetAttributes(attribute.Key("http.target").String(path))
			span.SetAttributes(attribute.Key("http.method").String(method))
			requestID := string(ctx.Request.Header.Peek(constant.HeaderRequestID))
			traceID := string(ctx.Request.Header.Peek(constant.HeaderTraceID))
			span.SetAttributes(attribute.Key("http.requestID").String(requestID))
			span.SetAttributes(attribute.Key("http.traceID").String(traceID))
			defer span.End()
			// set child ctx to request header by carrier
			childCarrier := propagation.HeaderCarrier{}
			propagattor.Inject(childCtx, childCarrier)
			ctx.Request.Header.Set(constant.HeaderTraceParent, childCarrier.Get(constant.HeaderTraceParent))
		}
		// call origin handler function
		originHandlerFunc(ctx)
	}
}
