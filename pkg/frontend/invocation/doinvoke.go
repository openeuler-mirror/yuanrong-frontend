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
	"fmt"
	"time"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/statuscode"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/functionmeta"
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

// InvokeHandler the handler of invoke
func InvokeHandler(ctx *types.InvokeProcessContext) error {
	var err error
	traceID := ctx.TraceID
	funcKey := ctx.FuncKey
	funcSpec, exist := functionmeta.LoadFuncSpec(funcKey)
	if !exist {
		responsehandler.SetErrorInContext(ctx, statuscode.FuncMetaNotFound, "function metadata not found")
		log.GetLogger().Errorf("function %s doesn't exist in cache", funcKey)
		return errors.New("function doesn't exist")
	}

	// JWT authentication check: validate X-Auth header before invoking
	// Skip authentication if function is public
	if !funcSpec.FuncMetaData.IsFuncPublic && config.GetConfig().EnableFuncTokenAuth {
		authHeader := ctx.ReqHeader[jwtauth.HeaderXAuth]
		if authHeader != "" {
			// Parse JWT to get role
			parsedJWT, err := jwtauth.ParseJWT(authHeader)
			if err != nil {
				log.GetLogger().Errorf("JWT parsing failed for function %s, traceID %s: %v", funcKey, traceID, err)
				responsehandler.SetErrorInContext(ctx, statuscode.FrontendStatusUnAuthorized, fmt.Sprintf("authentication failed: %v", err))
				return fmt.Errorf("JWT parsing failed: %v", err)
			}
			// Check role and validate tenant ID accordingly
			role := parsedJWT.Payload.Role
			expectedTenantID := funcSpec.FuncMetaData.TenantID

			if role == jwtauth.RoleDeveloper {
				// For developer role, validate tenant ID
				if err := parsedJWT.ValidateTenantID(expectedTenantID); err != nil {
					log.GetLogger().Errorf("JWT tenant ID validation failed for function %s, traceID %s: %v", funcKey, traceID, err)
					responsehandler.SetErrorInContext(ctx, statuscode.FrontendStatusUnAuthorized, fmt.Sprintf("authentication failed: %v", err))
					return fmt.Errorf("JWT tenant ID validation failed: %v", err)
				}
				log.GetLogger().Infof("JWT authentication passed for function %s, role: developer, traceID %s", funcKey, traceID)
			} else if role == jwtauth.RoleUser {
				// For user role, skip tenant ID validation
				log.GetLogger().Infof("JWT authentication passed for function %s, role: user, skipping tenant ID validation, traceID %s", funcKey, traceID)
			} else {
				// only check roles of user or develop. other role just skip
			}
			// After role-based validation passes, send request to IAM server
			if err := jwtauth.ValidateWithIamServer(authHeader, traceID); err != nil {
				log.GetLogger().Errorf("IAM server validation failed for function %s, traceID %s: %v", funcKey, traceID, err)
				responsehandler.SetErrorInContext(ctx, statuscode.FrontendStatusUnAuthorized, fmt.Sprintf("IAM server validation failed: %v", err))
				return fmt.Errorf("IAM server validation failed: %v", err)
			}
			log.GetLogger().Infof("IAM server validation passed for function %s, traceID %s", funcKey, traceID)
		} else {
			log.GetLogger().Errorf("token auth is needed while token is empty for function %s, traceID %s", funcKey, traceID)
			responsehandler.SetErrorInContext(ctx, statuscode.FrontendStatusUnAuthorized, fmt.Sprintf("token is empty for function: %s", funcKey))
			return fmt.Errorf("token is empty for function: %s", funcKey)
		}
	} else {
		log.GetLogger().Infof("Skipping JWT authentication for public function %s, traceID %s", funcKey, traceID)
	}

	sessionId := ctx.ReqHeader[httpconstant.HeaderInstanceSession]
	instanceLabel := ctx.ReqHeader[httpconstant.HeaderInstanceLabel]
	log.GetLogger().Infof("invoking function %s, signature %s, traceID %s, sessionId %s, instanceLabel %s",
		funcSpec.FunctionKey, funcSpec.FuncMetaSignature, traceID, sessionId, instanceLabel)
	err = doInvokeWithRetry(ctx, funcSpec)
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
