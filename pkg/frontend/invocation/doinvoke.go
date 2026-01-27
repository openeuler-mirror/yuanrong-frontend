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

package invocation

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/statuscode"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/functionmeta"
	"frontend/pkg/frontend/metrics"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/schedulerproxy"
	"frontend/pkg/frontend/types"
)

const (
	maxInvokeRetries = 5
	retrySleepTime   = 1000 * time.Millisecond
	baseTen          = 10
	bitSize          = 64
)

var metricsOnce sync.Once

func initInvokeMetrics() {
	metricsOnce.Do(func() {
		// Register counter for function invocations with function name and http code
		err := metrics.RegisterCounter("function_invocations_total", "Total number of function invocations", []string{"function_name", "http_code"})
		if err != nil {
			log.GetLogger().Warnf("failed to register function_invocations_total metric: %v", err)
		}
		// Register histogram for function invocation duration
		err = metrics.RegisterHistogram("function_invocation_duration_seconds", "Function invocation duration in seconds", []string{"function_name"}, nil)
		if err != nil {
			log.GetLogger().Warnf("failed to register function_invocation_duration_seconds metric: %v", err)
		}
	})
}

// InvokeHandler the handler of invoke
func InvokeHandler(ctx *types.InvokeProcessContext) error {
	initInvokeMetrics()
	var err error
	traceID := ctx.TraceID
	funcKey := ctx.FuncKey
	invokeStart := time.Now()
	invokeTotalTime := time.Since(invokeStart)

	// Use defer to ensure metrics are reported even if function returns early
	defer func() {
		// Prepare metrics reporting
		functionName := ctx.FuncKey
		if functionName == "" {
			functionName = "unknown"
		}
		// Get http code from context
		httpCode := ctx.StatusCode
		httpCodeStr := strconv.Itoa(httpCode)

		// Report invocation count
		if err := metrics.IncrementCounter("function_invocations_total", functionName, httpCodeStr); err != nil {
			log.GetLogger().Debugf("failed to report function_invocations_total metric: %v", err)
		}

		// Report invocation duration
		if err := metrics.ObserveHistogram("function_invocation_duration_seconds", invokeTotalTime.Seconds(), functionName); err != nil {
			log.GetLogger().Debugf("failed to report function_invocation_duration_seconds metric: %v", err)
		}
	}()

	funcSpec, exist := functionmeta.LoadFuncSpec(funcKey)
	if !exist {
		responsehandler.SetErrorInContext(ctx, statuscode.FuncMetaNotFound, "function metadata not found")
		log.GetLogger().Errorf("function %s doesn't exist in cache", funcKey)
		return errors.New("function doesn't exist")
	}
	sessionId := ctx.ReqHeader[httpconstant.HeaderInstanceSession]
	instanceLabel := ctx.ReqHeader[httpconstant.HeaderInstanceLabel]
	log.GetLogger().Infof("invoking function %s, signature %s, traceID %s, sessionId %s, instanceLabel %s",
		funcSpec.FunctionKey, funcSpec.FuncMetaSignature, traceID, sessionId, instanceLabel)
	err = doInvokeWithRetry(ctx, funcSpec)
	invokeTotalTime = time.Since(invokeStart)
	if err != nil {
		log.GetLogger().Errorf("failed to finish the request, traceID %s, error: %s", traceID, err.Error())
		httputil.HandleInvokeError(ctx, err)
	}
	log.GetLogger().Debugf("invoke function %s success, traceID %s, sessionId %s, instanceLabel %s",
		funcSpec.FunctionKey, traceID, sessionId, instanceLabel)
	return err
}

func doInvokeWithRetry(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec) error {
	execute := func() error {
		ctx.ShouldRetry = false
		return doInvoke(ctx, funcSpec)
	}
	shouldRetry := func() bool {
		log.GetLogger().Warnf("invoke will be retried is %t, traceID: %s", ctx.ShouldRetry, ctx.TraceID)
		return ctx.ShouldRetry
	}
	return util.Retry(execute, shouldRetry, maxInvokeRetries, retrySleepTime)
}

func doInvoke(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec) error {
	if config.GetConfig().FunctionInvokeBackend == constant.BackendTypeFG {
		return functionInvokeForFG(ctx, funcSpec)
	}
	kernelReqHandler := newKernelRequestHandler(ctx, funcSpec)
	return kernelReqHandler.invoke()
}

func resetSchedulerProxy(ctx *types.InvokeProcessContext) {
	if ctx.TrafficLimited {
		schedulerproxy.Proxy.Reset()
		ctx.TrafficLimited = false
	}
}
