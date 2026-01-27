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

// Package invocation -
package invocation

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"go.uber.org/zap"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/resspeckey"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/uuid"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/instancemanager"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/schedulerproxy"
	"frontend/pkg/frontend/types"
	"frontend/pkg/frontend/wisecloud"
)

func computeTimeout(originTimeout int64, beginTime time.Time) int64 {
	costTime := time.Now().Sub(beginTime)
	costTimeSecond := int64(math.Trunc(costTime.Seconds()))
	return originTimeout - costTimeSecond
}

type kernelRequestHandler struct {
	ctx           *types.InvokeProcessContext
	funcSpec      *commontype.FuncSpec
	funcKey       string
	resSpecKeyStr string
	resSpecKey    *resspeckey.ResSpecKey
	logger        api.FormatLogger

	startTime              time.Time
	invokeWithOutScheduler bool
	timeout                int64

	currentSchedulerInfo *commontype.InstanceInfo
	unexpectedInstances  []string
}

func newKernelRequestHandler(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec) *kernelRequestHandler {
	if ctx.InvokeTimeout == 0 {
		ctx.InvokeTimeout = funcSpec.FuncMetaData.Timeout
	}
	resSpecKey := convertResSpecKey(ctx, funcSpec)
	return &kernelRequestHandler{
		ctx:           ctx,
		funcSpec:      funcSpec,
		funcKey:       funcSpec.FunctionKey,
		resSpecKey:    &resSpecKey,
		resSpecKeyStr: resSpecKey.String(),
		logger: log.GetLogger().With(zap.Any("traceId", ctx.TraceID), zap.Any("function", funcSpec.FunctionKey),
			zap.Any("timeout", ctx.InvokeTimeout), zap.Any("acquireTimeout", ctx.AcquireTimeout)),
		startTime:              time.Now(),
		invokeWithOutScheduler: ctx.InvokeWithoutScheduler,
		unexpectedInstances:    make([]string, 0),
		timeout:                ctx.InvokeTimeout,
	}
}

func (k *kernelRequestHandler) makeReq(logger api.FormatLogger) (*util.InvokeRequest, error) {
	var err error
	k.currentSchedulerInfo = nil
	if !k.invokeWithOutScheduler {
		k.currentSchedulerInfo, err = schedulerproxy.Proxy.Get(k.funcKey, logger)
		if err != nil {
			logger.Warnf("failed to get scheduler, err: %s", err.Error())
		}
	}

	var instanceId string
	if needDownGrade(k.currentSchedulerInfo) {
		k.invokeWithOutScheduler = true // 这里要处理的情况是，当无可用scheduler时，该请求后续都不走租约机制，直接选择实例调用
		instance := instancemanager.GetGlobalInstanceScheduler().GetRandomInstanceWithoutUnexpectedInstance(
			k.funcKey, k.resSpecKeyStr, k.unexpectedInstances, logger)

		if instance == nil {
			pendingRequest := &wisecloud.PendingRequest{
				CreatedTime:     time.Now(),
				ScheduleTimeout: time.Duration(k.ctx.AcquireTimeout) * time.Second,
				ResultChan:      make(chan *wisecloud.PendingResponse, 1),
			}
			wisecloud.GetQueueManager().AddPendingRequest(k.funcKey, k.resSpecKey, pendingRequest)
			pendingResponse := <-pendingRequest.ResultChan
			if pendingResponse.Error != nil {
				return nil, pendingResponse.Error
			}
			if pendingResponse.Instance == nil {
				return nil, fmt.Errorf("no available instance, no available scheduler")
			}
			instance = pendingResponse.Instance
		}
		instanceId = instance.InstanceID
	}

	req, err := convert(k.ctx, k.currentSchedulerInfo, k.funcSpec, instanceId)
	if err != nil {
		logger.Errorf("failed to convert request, err: %s", err.Error())
		return nil, err
	}
	return req, nil
}

func (k *kernelRequestHandler) invoke() error {
	defer resetSchedulerProxy(k.ctx)
	count := 0
	for {
		count++
		k.ctx.RequestID = uuid.New().String()
		k.ctx.InvokeTimeout = computeTimeout(k.ctx.InvokeTimeout, k.startTime)
		if k.ctx.InvokeTimeout <= 0 {
			return fmt.Errorf("do invoke failed, timeout")
		}

		logger := k.logger.With(zap.Any("requestId", k.ctx.RequestID), zap.Any("timeLeft", k.ctx.InvokeTimeout),
			zap.Any("count", count))
		req, err := k.makeReq(logger)
		if err != nil {
			logger.Errorf("make req failed: %s", err.Error())
			httputil.HandleInvokeError(k.ctx, err)
			return err
		}

		if k.invokeWithOutScheduler {
			wisecloud.GetMetricsManager().InvokeStart(k.funcKey, k.resSpecKeyStr, req.InstanceID)
		}

		snError := invokeByClient(k.ctx, *req, logger)
		if k.invokeWithOutScheduler {
			wisecloud.GetMetricsManager().InvokeEnd(k.funcKey, k.resSpecKeyStr, req.InstanceID)
		}
		if snError != nil {
			if snError.Code() == constant.AcquireLeaseTrafficLimitErrorCode && k.currentSchedulerInfo != nil {
				schedulerproxy.Proxy.SetStain(k.funcKey, k.currentSchedulerInfo.InstanceName)
				k.ctx.TrafficLimited = true
				continue
			} else if snError.Code() == statuscode.FrontendStatusInternalError {
				logger.Errorf("failed to invoke name by client, err: %s", snError.Error())
				responsehandler.SetErrorInContext(k.ctx, statuscode.FrontendStatusInternalError, snError.Error())
				return snError
			} else if snError.Code() == statuscode.ErrAllSchedulerUnavailable {
				logger.Warnf("all schedulers are unavailable")
				k.invokeWithOutScheduler = true // 这里要处理的情况是，当无可用scheduler时，该请求后续都不走租约机制，直接选择实例调用
				continue
			} else if k.invokeWithOutScheduler && invokeInstanceNeedRetry(snError.Code()) {
				logger.Warnf("do invokeByInstanceId failed, retry, code: %d, message: %s",
					snError.Code(), snError.Error())
				k.unexpectedInstances = append(k.unexpectedInstances, req.InstanceID)
				continue
			} else {
				httputil.HandleInvokeError(k.ctx, snError)
			}
		}
		return nil
	}
}

func invokeInstanceNeedRetry(code int) bool {
	// 暂时不考虑区分 同实例重试和不同实例重试的错误码
	needRetryCode := map[int]struct{}{
		statuscode.ErrInstanceNotFound:    {}, // 1003
		statuscode.ErrInstanceExitedCode:  {}, // 1007
		statuscode.ErrInstanceCircuitCode: {}, // 1009
		statuscode.ErrInstanceEvicted:     {}, // 1013

		// 参考libruntime写法, runtime\src\libruntime\invokeadaptor\task_submitter.cpp
		statuscode.ErrRequestBetweenRuntimeBusCode:      {}, // 3001
		statuscode.ErrInnerCommunication:                {}, // 3002
		statuscode.ErrRequestBetweenRuntimeFrontendCode: {}, // 3008

		statuscode.ErrSharedMemoryLimited:   {}, // 4202
		statuscode.ErrOperateDiskFailed:     {}, // 4203
		statuscode.ErrInsufficientDiskSpace: {}, // 4204
		statuscode.ErrFinalized:             {}, // 9000
	}
	_, ok := needRetryCode[code]
	return ok
}

func getAcquireReqCPUAndMemory(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec) (int64, int64) {
	cpu := funcSpec.ResourceMetaData.CPU
	memory := funcSpec.ResourceMetaData.Memory
	if ctx == nil || ctx.ReqHeader == nil {
		return cpu, memory
	}
	if cpuString := util.PeekIgnoreCase(ctx.ReqHeader, constant.HeaderCPUSize); cpuString != "" {
		cpuInt, err := strconv.Atoi(cpuString)
		if err != nil || cpuInt <= 0 {
			log.GetLogger().Warnf("invalid value %s from request header", constant.HeaderCPUSize)
		} else {
			cpu = int64(cpuInt)
		}
	}

	if memoryString := util.PeekIgnoreCase(ctx.ReqHeader, constant.HeaderMemorySize); memoryString != "" {
		memoryInt, err := strconv.Atoi(memoryString)
		if err != nil || memoryInt <= 0 {
			log.GetLogger().Warnf("invalid value %s from request header", constant.ResourceMemoryName)
		} else {
			memory = int64(memoryInt)
		}
	}
	return cpu, memory
}

func convertResSpecKey(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec) resspeckey.ResSpecKey {
	invokeLabel := util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderInstanceLabel)
	cpu, memory := getAcquireReqCPUAndMemory(ctx, funcSpec)

	resSpec := resspeckey.ConvertResourceMetaDataToResSpec(funcSpec.ResourceMetaData)
	resSpec.InvokeLabel = invokeLabel
	resSpec.CPU = cpu
	resSpec.Memory = memory

	return resspeckey.ConvertToResSpecKey(resSpec)
}

func needDownGrade(schedulerInfo *commontype.InstanceInfo) bool {
	if schedulerInfo == nil || schedulerproxy.Proxy.IsEmpty() {
		return true
	}
	return false
}

func invokeByClient(ctx *types.InvokeProcessContext, request util.InvokeRequest,
	logger api.FormatLogger,
) snerror.SNError {
	logger.Infof("send request %v to grpc", request)

	invokeStart := time.Now()
	var notifyMsg []byte
	var err error
	if request.InstanceID != "" {
		notifyMsg, err = util.NewClient().Invoke(request)
	} else {
		notifyMsg, err = util.NewClient().InvokeByName(request)
	}

	invokeTotalTime := time.Since(invokeStart)
	logger.Debugf("get response %s, err: %v", string(notifyMsg), err)

	if err != nil {
		if rtErr, ok := err.(api.ErrorInfo); ok {
			logger.Errorf("invoke request, errCode: %d, error: %s, totalTime: %v",
				rtErr.Code, rtErr.Error(), invokeTotalTime.Seconds())
			if snErr := checkErrorMsg(rtErr.Error()); snErr != nil {
				return snErr
			}
			return snerror.New(rtErr.Code, rtErr.Error())
		}
		if snError := checkInstanceResp(notifyMsg); snError != nil {
			return snError
		}
		logger.Errorf("invoke GRPC request error: %s, totalTime: %v", err.Error(), invokeTotalTime.Seconds())
		errMsg := fmt.Sprintf("invoke GRPC request error: %s", err.Error())
		httputil.JudgeRetry(err, ctx)
		return snerror.New(statuscode.FrontendStatusInternalError, errMsg)
	}
	respMsg, snErr := responsehandler.SetResponseInContext(ctx, notifyMsg)
	if snErr != nil {
		return snErr
	}
	if ctx.RequestTraceInfo != nil {
		ctx.RequestTraceInfo.FrontendCost = invokeTotalTime
		ctx.RequestTraceInfo.WorkerCost = httputil.GetTimeFromResp(respMsg.UserFuncTime)
	}
	logger.Infof("invoke end, totalTime: %f, executorTime: %f, userTime: %f", invokeTotalTime.Seconds(),
		httputil.GetTimeFromResp(respMsg.ExecutorTime).Seconds(), httputil.GetTimeFromResp(respMsg.UserFuncTime).Seconds())
	return nil
}

// Convert an http request to a POSIX invoke request
func convert(ctx *types.InvokeProcessContext, schedulerInfo *commontype.InstanceInfo, funcSpec *commontype.FuncSpec,
	instanceId string,
) (*util.InvokeRequest, error) {
	resourceSpecs, err := util.ConvertResourceSpecs(ctx, funcSpec)
	if err != nil {
		return nil, err
	}

	req := &util.InvokeRequest{
		Function:        ctx.FuncKey,
		TraceID:         ctx.TraceID,
		RequestID:       ctx.RequestID,
		ReturnObjectIDs: []string{},
		ResourceSpecs:   resourceSpecs,
		PoolLabel:       util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderPoolLabel),
		InvokeTag:       convertInvokeTag(ctx),
		InstanceLabel:   util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderInstanceLabel),
		AcquireTimeout:  getTimeout(util.GetAcquireTimeout(funcSpec), ctx.AcquireTimeout),
		InvokeTimeout:   ctx.InvokeTimeout,
		FuncSig:         funcSpec.FuncMetaSignature,
		TrafficLimited:  ctx.TrafficLimited,
		BusinessType:    funcSpec.FuncMetaData.BusinessType,
		TenantID:        funcSpec.FuncMetaData.TenantID,
		InstanceID:      instanceId,
	}

	if schedulerInfo != nil {
		req.SchedulerID = schedulerInfo.InstanceID
		req.SchedulerFuncKey = schedulerInfo.FunctionName
	}

	instanceSession := util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderInstanceSession)
	if instanceSession != "" {
		session := &commontype.InstanceSessionConfig{}
		err = json.Unmarshal([]byte(instanceSession), &session)
		if err != nil {
			return nil, err
		}
		req.InstanceSession = session
	}
	body, err := httputil.TranslateInvokeMsgToCallReq(ctx)
	if err != nil {
		responsehandler.SetErrorInContext(ctx, statuscode.FrontendStatusInternalError, err.Error())
		return req, err
	}
	dynamicResourceSpecs, err := prepareDynamicResource(ctx)
	if err != nil {
		responsehandler.SetErrorInContext(ctx, statuscode.FrontendStatusInternalError, err.Error())
		return req, err
	}
	if dynamicResourceSpecs[constant.ResourceCPUName] > 0 && dynamicResourceSpecs[constant.ResourceMemoryName] > 0 {
		req.ResourceSpecs = dynamicResourceSpecs
	}

	req.Args = newArgList([]byte(ctx.TraceID), body)
	return req, nil
}

func getTimeout(funcSpecTimeout int64, ctxTimeout int64) int64 {
	if ctxTimeout != 0 {
		return ctxTimeout
	}
	return funcSpecTimeout
}

func checkErrorMsg(msg string) snerror.SNError {
	if len(msg) != 0 {
		var errInfo struct {
			ErrorCode    int    `json:"code"`
			ErrorMessage string `json:"message"`
		}
		if unMarshalErr := json.Unmarshal([]byte(msg), &errInfo); unMarshalErr != nil {
			log.GetLogger().Debugf("unmarshal notifyMsg error : %s", unMarshalErr.Error())
			return nil
		}
		if errInfo.ErrorCode != 0 && errInfo.ErrorMessage != "" {
			// current faasscheduler has reached instance limit, should retry and chose another faasscheduler
			return snerror.New(errInfo.ErrorCode, errInfo.ErrorMessage)
		}
	}
	return nil
}

func checkInstanceResp(notifyMsg []byte) snerror.SNError {
	if notifyMsg != nil && len(notifyMsg) != 0 {
		var insResponse struct {
			ErrorCode    int    `json:"errorCode"`
			ErrorMessage string `json:"errorMessage"`
		}
		if unMarshalErr := json.Unmarshal(notifyMsg, &insResponse); unMarshalErr != nil {
			log.GetLogger().Errorf("unmarshal notifyMsg error : %s", unMarshalErr.Error())
		}
		if insResponse.ErrorCode != 0 && insResponse.ErrorMessage != "" {
			// current faasscheduler has reached instance limit, should retry and chose another faasscheduler
			return snerror.New(insResponse.ErrorCode, insResponse.ErrorMessage)
		}
	}
	return nil
}

func newArgList(payloads ...[]byte) []*api.Arg {
	var result []*api.Arg
	for _, p := range payloads {
		result = append(result, &api.Arg{Type: api.Value, Data: p})
	}
	return result
}

func convertInvokeTag(ctx *types.InvokeProcessContext) map[string]string {
	m := make(map[string]string)
	headerValue, ok := ctx.ReqHeader[httpconstant.HeaderInvokeTag]
	if !ok || headerValue == "" {
		return m
	}
	err := json.Unmarshal([]byte(headerValue), &m)
	if err != nil {
		log.GetLogger().Errorf("convert invoke tag failed, traceId: %s, err: %s", ctx.TraceID, err.Error())
		return m
	}
	return m
}

func prepareDynamicResource(ctx *types.InvokeProcessContext) (map[string]int64, error) {
	dynamicResourcesRoute := make(map[string]int64)
	cpuBytes := ctx.ReqHeader[httpconstant.HeaderCPUSize]
	memoryBytes := ctx.ReqHeader[httpconstant.HeaderMemorySize]
	customResourcesString := httputil.GetCompatibleHeader(ctx.ReqHeader, constant.HeaderCustomResourceNew,
		constant.HeaderCustomResource)

	logger := log.GetLogger().With(zap.Any("traceId", ctx.TraceID), zap.Any("funcKey", ctx.FuncKey))
	if cpuBytes != "" && memoryBytes != "" {
		cpu, err := strconv.ParseInt(cpuBytes, baseTen, bitSize)
		if err != nil {
			return dynamicResourcesRoute, err
		}
		memory, err := strconv.ParseInt(memoryBytes, baseTen, bitSize)
		if err != nil {
			return dynamicResourcesRoute, err
		}
		dynamicResourcesRoute[constant.ResourceCPUName] = cpu
		dynamicResourcesRoute[constant.ResourceMemoryName] = memory
	}
	if customResourcesString != "" {
		var customResources map[string]int64
		if err := json.Unmarshal([]byte(customResourcesString), &customResources); err != nil {
			logger.Errorf("failed to unmarshal custom resources %s", err.Error())
			return dynamicResourcesRoute, err
		}
		for resourceType, resource := range customResources {
			if resource > constant.MinCustomResourcesSize {
				dynamicResourcesRoute[resourceType] = resource
			} else {
				logger.Warnf("ignore invalid value %f of custom resource %s", resource, resourceType)
			}
		}
	}
	if len(dynamicResourcesRoute) != 0 {
		logger.Infof("dynamicResourcesRoute is %v", dynamicResourcesRoute)
	}
	return dynamicResourcesRoute, nil
}
