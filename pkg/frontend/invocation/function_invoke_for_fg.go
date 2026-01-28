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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/common/faas_common/tls"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/functiontask"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/stream"
	"frontend/pkg/frontend/types"
)

const (
	invokePath     = "/invoke"
	defaultBodyMap = 100
	requestTimeout = "timeout"
	retryInterval  = 1000
)

var (
	// ErrServiceNotAvailable -
	ErrServiceNotAvailable = errors.New("worker service is not available")
)

type innerCode int

// String convert innerCode to error type string
func (ic innerCode) String() string {
	if ic < 0 {
		return "invalidCode"
	}
	if ic == 0 {
		return "sysOK"
	}
	if ic < snerror.UserErrorMax {
		return "usrErr"
	}
	return "sysErr"
}

func functionInvokeForFG(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec) error {
	if ctx.IsHTTPUploadStream {
		timeout := getRequestDeadline(funcSpec)
		if err := stream.HTTPStreamInvokeHandler(ctx, timeout); err != nil {
			setErrorResponse(ctx, err)
			return nil
		}
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	readBodyStart := time.Now()
	transferHTTPRequest(ctx, req)
	readBodyTotal := time.Since(readBodyStart)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	prepareRequest(ctx, req, funcSpec)
	defer stream.ReleaseResponse(ctx.StreamInfo.ResponseStreamName)
	err := invokeFunction(ctx, funcSpec, req, resp)
	if err != nil {
		setErrorResponse(ctx, err)
		return nil
	}
	writeRspStart := time.Now()
	processResp(ctx, resp)
	writeRspCost := time.Since(writeRspStart)
	setTraceInfo(ctx, resp)
	fillAfterInvokeHeader(ctx)
	setLogAndPublishMetrics(ctx, readBodyTotal, writeRspCost)
	return nil
}

func processResp(ctx *types.InvokeProcessContext, resp *fasthttp.Response) {
	logger := log.GetLogger().With(zap.Any("traceID", ctx.TraceID))
	if ctx.StreamInfo.ResponseStopChan != nil && stream.CheckIsResponseStream(ctx.StreamInfo.ResponseStreamName) {
		logger.Infof("start wait download stream rsp")
		timeout := ctx.RequestTraceInfo.Deadline.Sub(time.Now())
		writeTimoutTimer := time.NewTimer(timeout)
		select {
		case _, ok := <-ctx.StreamInfo.ResponseStopChan.C:
			// wait until the response stream is written.
			if !ok {
				logger.Infof("received response stream stop signal, return")
			}
		case <-writeTimoutTimer.C:
			logger.Warnf("wait for response stream timeout, timeout: %v", timeout)
			setErrorResponse(ctx, snerror.New(statuscode.InternalErrorCode, "wait for response stream timeout"))
		}
	} else if ctx.IsHTTPUploadStream && ctx.StreamInfo.GetRequestStreamErrorCode() != 0 {
		// 上传流场景优先使用上传协程中的异常码，否则才使用http的影响返回
		logger.Warnf("upload stream async err code %d", ctx.StreamInfo.GetRequestStreamErrorCode())
		setErrorResponse(ctx, snerror.New(int(ctx.StreamInfo.GetRequestStreamErrorCode()), "internal error"))
	} else {
		transferHTTPResponse(ctx, resp)
	}
}

func setErrorResponse(ctx *types.InvokeProcessContext, err error) {
	errorCode := statuscode.FrontendStatusInternalError
	responsehandler.SetErrorInContextWithDefault(ctx, err, errorCode, err.Error())
	log.GetLogger().Infof("%s | %s | %s | %f | | | | innerCode %d | %s | invoke end in frontend",
		ctx.RequestTraceInfo.FuncName, ctx.RequestTraceInfo.TenantID, ctx.TraceID,
		time.Since(ctx.StartTime).Seconds(),
		ctx.RequestTraceInfo.InnerCode, innerCode(ctx.RequestTraceInfo.InnerCode).String())
}

func setTraceInfo(ctx *types.InvokeProcessContext, resp *fasthttp.Response) {
	ctx.RequestTraceInfo.CallNode = string(resp.Header.Peek(constant.HeaderCallNode))
	ctx.RequestTraceInfo.CallInstance = string(resp.Header.Peek(constant.HeaderCallInstance))

	ctx.RequestTraceInfo.TotalCost = time.Since(ctx.StartTime)
	ctx.RequestTraceInfo.FrontendCost = ctx.RequestTraceInfo.TotalCost - ctx.RequestTraceInfo.AllBusCost
	workerCost := string(resp.Header.Peek(constant.HeaderWorkerCost))
	if len(workerCost) != 0 {
		v, err := strconv.ParseInt(workerCost, baseTen, bitSize)
		if err != nil {
			log.GetLogger().Errorf("failed to ParseInt, workerCost %s,traceID:%s", workerCost,
				ctx.TraceID)
		} else {
			ctx.RequestTraceInfo.WorkerCost = time.Duration(v) * time.Nanosecond
		}
	}
	ctx.RequestTraceInfo.BusCost = ctx.RequestTraceInfo.AllBusCost - ctx.RequestTraceInfo.WorkerCost
}

func fillAfterInvokeHeader(ctx *types.InvokeProcessContext) {
	ctx.RespHeader[constant.HeaderCallNode] = ctx.RequestTraceInfo.CallNode
	ctx.RespHeader[constant.HeaderCallInstance] = ctx.RequestTraceInfo.CallInstance
}

func setLogAndPublishMetrics(ctx *types.InvokeProcessContext,
	readBodyTotal time.Duration, writeRspCost time.Duration) {
	// funcName|tenantID|traceID|cost|frontendCost:busCost:workerCost:readBodyCost|contentLength|nodeIP|workerPod|
	// innerCode|ErrMsg
	log.GetLogger().Infof("%s | %s | %s | %f | %f:%f:%f:%f:%f | %d |%s | %s | innerCode %d | %s | "+
		"invoke end in frontend", ctx.RequestTraceInfo.FuncName, ctx.RequestTraceInfo.TenantID, ctx.TraceID,
		ctx.RequestTraceInfo.TotalCost.Seconds(), ctx.RequestTraceInfo.FrontendCost.Seconds(),
		ctx.RequestTraceInfo.BusCost.Seconds(), ctx.RequestTraceInfo.WorkerCost.Seconds(), readBodyTotal.Seconds(),
		writeRspCost.Seconds(), len(ctx.ReqBody), ctx.RequestTraceInfo.CallNode, ctx.RequestTraceInfo.CallInstance,
		ctx.RequestTraceInfo.InnerCode, innerCode(ctx.RequestTraceInfo.InnerCode).String())
}

func invokeFunction(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec,
	req *fasthttp.Request, resp *fasthttp.Response) error {
	var (
		instanceAllocationInfo *commontype.InstanceAllocationInfo
		err                    error
	)
	for {
		instanceAllocationInfo, err = acquireInstance(ctx, funcSpec)
		if err != nil {
			return err
		}
		if !functiontask.GetBusProxies().IsBusProxyHealthy(instanceAllocationInfo.NodeIP, ctx.TraceID) {
			instanceAllocationInfo, err = selectBusForInstance(ctx)
			if err != nil {
				return err
			}
		}
		initProxyRequest(req, instanceAllocationInfo, funcSpec)
		needBreak, needTry, err := invokeBus(ctx, req, resp, funcSpec)
		if err != nil {
			leaseadaptor.GetInstanceManager().ReleaseInstanceAllocation(instanceAllocationInfo,
				true, ctx.TraceID)
			if needTry {
				time.Sleep(retryInterval * time.Millisecond)
				continue
			}
			return err
		}
		if needBreak {
			leaseadaptor.GetInstanceManager().ReleaseInstanceAllocation(instanceAllocationInfo,
				false, ctx.TraceID)
			return nil
		}
		if needTry {
			leaseadaptor.GetInstanceManager().ReleaseInstanceAllocation(instanceAllocationInfo,
				false, ctx.TraceID)
			time.Sleep(retryInterval * time.Millisecond)
		}
	}
}

func selectBusForInstance(ctx *types.InvokeProcessContext) (*commontype.InstanceAllocationInfo, error) {
	nodeIP := functiontask.GetBusProxies().NextWithName(ctx.FuncKey, true)
	if nodeIP == "" {
		log.GetLogger().Errorf("select bus failed not found busProxy,traceID:%s", ctx.TraceID)
		return nil, fmt.Errorf("not found busProxy")
	}
	log.GetLogger().Infof("select bus for function:%s,"+
		"busIP:%s,traceID:%s", ctx.RequestTraceInfo.AnonymizeURN, nodeIP, ctx.TraceID)
	return &commontype.InstanceAllocationInfo{
		NodeIP:   nodeIP,
		NodePort: constant.BusProxyHTTPPort,
	}, nil
}

func prepareRequest(ctx *types.InvokeProcessContext, req *fasthttp.Request, funcSpec *commontype.FuncSpec) {
	insResource := getInsResource(funcSpec, req)
	setHeader(req, ctx.RequestTraceInfo.URN, insResource)
	setRequestDeadline(ctx.RequestTraceInfo, funcSpec)
	prepareStreamResponse(ctx, req)
}

func transferHTTPRequest(ctx *types.InvokeProcessContext, req *fasthttp.Request) {
	for key, value := range ctx.ReqHeader {
		req.Header.Set(key, value)
	}
	streamName := ctx.ReqHeader[constant.HeaderRequestStreamName]
	streamEvent := ctx.ReqHeader[constant.HeaderStreamAPIGEvent]
	if streamName != "" && streamEvent != "" {
		body := []byte(streamEvent)

		req.Header.Set(httpconstant.ContentType, httpconstant.ApplicationJSON)
		req.Header.Set(constant.HeaderContentLength, strconv.Itoa(len(body)))
		req.SetBody(body)
	} else {
		req.SetBody(ctx.ReqBody)
	}
}

func transferHTTPResponse(ctx *types.InvokeProcessContext, resp *fasthttp.Response) {
	ctx.RespHeader[httpconstant.ContentType] = httpconstant.ApplicationJSON
	if ctx.RequestTraceInfo.InnerCode == statuscode.HeavyLoadCode {
		ctx.RespHeader[constant.HeaderInnerCode] = strconv.Itoa(statuscode.FrontendStatusInternalError)
	} else {
		ctx.RespHeader[constant.HeaderInnerCode] = string(resp.Header.Peek(constant.HeaderInnerCode))
	}
	ctx.RespHeader[constant.HeaderLogResult] = string(resp.Header.Peek(constant.HeaderLogResult))
	ctx.RespHeader[constant.HeaderInvokeSummary] = string(resp.Header.Peek(constant.HeaderInvokeSummary))
	ctx.RespHeader[constant.HeaderBillingDuration] = string(resp.Header.Peek(constant.HeaderBillingDuration))
	ctx.StatusCode = resp.StatusCode()
	ctx.RespBody = resp.Body()
}

func getInsResource(funcSpec *commontype.FuncSpec, req *fasthttp.Request) commontype.InstanceResource {
	if funcSpec.FuncMetaData.BusinessType == constant.BusinessTypeCAE {
		return commontype.InstanceResource{
			CPU:    "0",
			Memory: "0",
		}
	}
	insResource := commontype.InstanceResource{
		CPU:    string(req.Header.Peek(constant.HeaderCPUSize)),
		Memory: string(req.Header.Peek(constant.HeaderMemorySize)),
	}
	if insResource.CPU == "" && insResource.Memory == "" {
		insResource.CPU, insResource.Memory = getCPUAndMemory(&funcSpec.ResourceMetaData)
	}
	return insResource
}

func getCPUAndMemory(metaResource *commontype.ResourceMetaData) (string, string) {
	return strconv.Itoa(int(metaResource.CPU)), strconv.Itoa(int(metaResource.Memory))
}

func setHeader(req *fasthttp.Request, urn string, insResource commontype.InstanceResource) {
	req.Header.Set(constant.HeaderInvokeURN, urn)
	if req.Header.Peek(constant.HeaderLogType) == nil {
		req.Header.Set(constant.HeaderLogType, constant.DefaultLogFlag)
	}
	req.Header.Set(constant.HeaderCPUSize, insResource.CPU)
	req.Header.Set(constant.HeaderMemorySize, insResource.Memory)
}

func initProxyRequest(proxyReq *fasthttp.Request, instanceInfo *commontype.InstanceAllocationInfo,
	funcSpec *commontype.FuncSpec) {
	proxyReq.SetRequestURI(invokePath)
	setLubanBody(proxyReq)
	proxyReq.Header.SetMethod("POST")
	proxyReq.SetHost(instanceInfo.NodeIP + ":" + instanceInfo.NodePort)
	proxyReq.URI().SetScheme(tls.GetURLScheme(config.GetConfig().HTTPSConfig.HTTPSEnable))
	proxyReq.Header.ResetConnectionClose()
	// defaultaz-#- will Spliced by bus
	proxyReq.Header.Set(constant.HeaderInstanceID, strings.TrimPrefix(instanceInfo.InstanceID, "defaultaz-#-"))
	proxyReq.Header.Set(constant.HeaderInstanceIP, instanceInfo.InstanceIP)
	if instanceInfo.CPU != 0 && instanceInfo.Memory != 0 &&
		funcSpec.FuncMetaData.BusinessType != constant.BusinessTypeCAE {
		proxyReq.Header.Set(constant.HeaderCPUSize, strconv.Itoa(int(instanceInfo.CPU)))
		proxyReq.Header.Set(constant.HeaderMemorySize, strconv.Itoa(int(instanceInfo.Memory)))
	}
	httputil.AddAuthorizationHeaderForFG(proxyReq)
}

func setLubanBody(req *fasthttp.Request) {
	lubanID := req.Header.Peek(httpconstant.HeaderLuBanGTraceID)
	if len(lubanID) == 0 {
		return
	}
	bodyMap := make(map[string]interface{}, defaultBodyMap)
	if err := json.Unmarshal(req.Body(), &bodyMap); err != nil {
		return
	}
	bodyMap[httpconstant.HeaderLuBanNTraceID] = string(req.Header.Peek(httpconstant.HeaderLuBanNTraceID))
	bodyMap[httpconstant.HeaderLuBanGTraceID] = string(lubanID)
	bodyMap[httpconstant.HeaderLuBanSpanID] = string(req.Header.Peek(httpconstant.HeaderLuBanSpanID))
	bodyMap[httpconstant.HeaderLuBanEvnID] = string(req.Header.Peek(httpconstant.HeaderLuBanEvnID))
	bodyMap[httpconstant.HeaderLuBanEventID] = string(req.Header.Peek(httpconstant.HeaderLuBanEventID))
	bodyMap[httpconstant.HeaderLuBanDomainID] = string(req.Header.Peek(httpconstant.HeaderLuBanDomainID))
	rsp, err := json.Marshal(bodyMap)
	if err != nil {
		return
	}
	req.SetBody(rsp)
}

func setRequestDeadline(requestTraceInfo *types.RequestTraceInfo, funcSpec *commontype.FuncSpec) {
	requestTraceInfo.Deadline = time.Now().Add(getRequestDeadline(funcSpec))
}

func prepareStreamResponse(ctx *types.InvokeProcessContext, req *fasthttp.Request) {
	if stream.RegisterResponse(ctx) {
		req.Header.Set(constant.HeaderResponseStreamName, ctx.StreamInfo.ResponseStreamName)
		req.Header.Set(constant.HeaderFrontendResponseStreamName, stream.GetFrontendResponseStreamName())
	}
}

func getRequestDeadline(funcSpec *commontype.FuncSpec) time.Duration {
	var timeout time.Duration
	if funcSpec.FuncMetaData.Runtime == constant.CustomContainerRuntimeType {
		timeout = time.Duration(config.GetConfig().HTTPConfig.WorkerInstanceReadTimeOut) * time.Second
	} else {
		funcTimeout := funcSpec.FuncMetaData.Timeout
		if funcTimeout > httputil.GetSyncRequestTimeout() {
			funcTimeout = httputil.GetSyncRequestTimeout()
		}
		timeout = time.Duration(config.GetConfig().E2EMaxDelayTime+
			funcSpec.ExtendedMetaData.Initializer.Timeout+funcTimeout) * time.Second
	}
	return timeout
}

func getInnerCode(ctx *types.InvokeProcessContext, resp *fasthttp.Response) int {
	Code := resp.Header.Peek(constant.HeaderInnerCode)
	innerCodeStr, err := strconv.Atoi(string(Code))
	if err != nil {
		log.GetLogger().Warnf("failed to parse inner code <%s>, error %s,traceID:%s",
			string(Code), err.Error(), ctx.TraceID)
		innerCodeStr = 0
	}
	return innerCodeStr
}

// invokeBus return (needBreak, needTry, error)
func invokeBus(ctx *types.InvokeProcessContext, req *fasthttp.Request,
	resp *fasthttp.Response, funcSpec *commontype.FuncSpec) (bool, bool, error) {
	if ctx.RequestTraceInfo.TryCount >= config.GetConfig().InvokeMaxRetryTimes {
		log.GetLogger().Warnf("failed to request bus for %d times,traceID:%s",
			config.GetConfig().InvokeMaxRetryTimes, ctx.TraceID)
		return false, false, ErrServiceNotAvailable
	}
	log.GetLogger().Infof("send request to %s,functionURN: %s,traceID:%s", req.Host(),
		ctx.RequestTraceInfo.AnonymizeURN, ctx.TraceID)
	err := doRequest(ctx, req, resp)
	if err != nil && config.GetConfig().RetryConfig != nil && config.GetConfig().RetryConfig.InstanceExceptionRetry {
		log.GetLogger().Errorf("retry %d, connection of worker instance %s is not healthy: %s,traceID:%s",
			ctx.RequestTraceInfo.TryCount, req.Host(), err.Error(), ctx.TraceID)
		if err.Error() == requestTimeout {
			return false, false, err
		}
		ctx.RequestTraceInfo.TryCount++
		req.Header.Set(constant.HeaderRetryFlag, constant.TrueStr)
		if ctx.RequestTraceInfo.TryCount >= config.GetConfig().InvokeMaxRetryTimes {
			return false, false, err
		}
		return false, true, nil
	}
	ctx.RequestTraceInfo.InnerCode = getInnerCode(ctx, resp)
	if shouldRetry(ctx.RequestTraceInfo.InnerCode, funcSpec.FuncMetaData.BusinessType) {
		log.GetLogger().Warnf("request in proxy %s for function %s failed: %d,retry count: %d,traceID:%s",
			req.Host(), ctx.RequestTraceInfo.AnonymizeURN, ctx.RequestTraceInfo.InnerCode,
			ctx.RequestTraceInfo.TryCount, ctx.TraceID)
		ctx.RequestTraceInfo.TryCount++
		return false, true, nil
	}
	// instance is not health should release abnormal try
	if ctx.RequestTraceInfo.InnerCode == statuscode.SpecificInstanceNotFound {
		log.GetLogger().Errorf("function %s request in proxy %s failed should release abnormal,"+
			"response: %d, cost: %s,traceID:%s", ctx.RequestTraceInfo.AnonymizeURN, req.Host(),
			ctx.RequestTraceInfo.InnerCode, ctx.RequestTraceInfo.LastBusCost, ctx.TraceID)
		return false, true, ErrServiceNotAvailable
	}
	if ctx.RequestTraceInfo.InnerCode != statuscode.InnerResponseSuccessCode {
		log.GetLogger().Errorf("function %s request in proxy %s failed, response: %d, cost: %s,traceID:%s",
			ctx.RequestTraceInfo.AnonymizeURN, req.Host(), ctx.RequestTraceInfo.InnerCode,
			ctx.RequestTraceInfo.LastBusCost, ctx.TraceID)
	}
	return true, false, nil
}

func doRequest(ctx *types.InvokeProcessContext, req *fasthttp.Request,
	resp *fasthttp.Response) error {
	start := time.Now()
	timeout := ctx.RequestTraceInfo.Deadline.Sub(start)
	if timeout <= 0 {
		log.GetLogger().Errorf("request has reached deadline,traceID:%s", ctx.TraceID)
		return errors.New("timeout")
	}
	err := httputil.GetGlobalClient().DoTimeout(req, resp, timeout)
	if err != nil {
		resp.Header.Set(constant.HeaderInnerCode, strconv.Itoa(statuscode.FrontendStatusInternalError))
		resp.SetStatusCode(http.StatusInternalServerError)
		return err
	}
	cost := time.Since(start)
	ctx.RequestTraceInfo.LastBusCost = cost
	ctx.RequestTraceInfo.AllBusCost += cost
	log.GetLogger().Debugf("response from %s with http status code %d,traceID:%s",
		req.Host(), resp.StatusCode(), ctx.TraceID)
	return nil
}

// If retry is needed, false is returned. Otherwise, true is returned.
func shouldRetry(innerCode int, businessType string) bool {
	switch innerCode {
	case statuscode.WorkerExitErrCode, statuscode.UserFuncIsUpdatedCode,
		statuscode.RefreshSilentFunc, statuscode.ClientExitErrCode,
		// bus return
		statuscode.BackpressureCode, statuscode.InstanceExceedConcurrency,
		// scheduler return
		constant.InsAcquireLeaseExistErrorCode:
		return true
	case statuscode.SendReqErrCode:
		if utils.IsCAEFunc(businessType) {
			return true
		}
	default:
		return false
	}
	return false
}

func acquireInstance(ctx *types.InvokeProcessContext,
	funcSpec *commontype.FuncSpec) (*commontype.InstanceAllocationInfo, error) {
	defer resetSchedulerProxy(ctx)

	var instanceAllocationInfo *commontype.InstanceAllocationInfo
	var snError snerror.SNError
	logger := log.GetLogger().With(zap.Any("traceId", ctx.TraceID), zap.Any("function", ctx.FuncKey))
	for {
		instanceAllocationInfo, snError = leaseadaptor.GetInstanceManager().AcquireInstance(ctx, funcSpec, logger)
		if snError != nil {
			logger.Errorf("failed to acquire lease, error:%+v", snError)
			if shouldRetry(snError.Code(), funcSpec.FuncMetaData.BusinessType) &&
				ctx.RequestTraceInfo.TryCount < (config.GetConfig().InvokeMaxRetryTimes-1) {
				logger.Warnf("failed to acquire lease for function %s failed: %d, retry count: %d",
					ctx.RequestTraceInfo.AnonymizeURN, ctx.RequestTraceInfo.InnerCode, ctx.RequestTraceInfo.TryCount)
				ctx.RequestTraceInfo.TryCount++
				time.Sleep(retryInterval * time.Millisecond)
				continue
			}
			if snError.Code() == statuscode.NoInstanceAvailableErrCode &&
				utils.IsCAEFunc(funcSpec.FuncMetaData.BusinessType) {
				instanceAllocationInfo, err := selectBusForInstance(ctx)
				if err != nil {
					return nil, err
				}
				logger.Infof("no instance available success to select bus for cae function:%s, busIP:%s",
					ctx.RequestTraceInfo.AnonymizeURN, instanceAllocationInfo.NodeIP)
				return instanceAllocationInfo, nil
			}
			return nil, snError
		}
		logger.Infof("success to acquire lease:%s, instanceID:%s",
			instanceAllocationInfo.ThreadID, instanceAllocationInfo.InstanceID)
		return instanceAllocationInfo, nil
	}
}
