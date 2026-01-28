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

// Package v1 -
package v1

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"frontend/pkg/common/faas_common/aliasroute"
	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/common"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/config"
	frontendlog "frontend/pkg/frontend/log"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/stream"
	"frontend/pkg/frontend/types"
)

// InvokeHandler -
// Invocation godoc
// @Summary      Invoke FaaS
// @Description  通过HTTP请求调用FaaS函数
// @Accept       json
// @Produce      json
// @Router       /serverless/v1/functions/{function-urn}/invocations [POST]
// @Param        X-Instance-Cpu	header string false "指定函数实例使用的CPU大小"
// @Param        X-Instance-Memory header string false "指定函数实例使用的内存大小"
// @Param        X-Instance-Custom-Resource header string false "指定函数实例使用的自定义资源大小"
// @Param        X-Invoke-Alias header string false "指定函数的别名进行调用"
// @Param        X-Stream-Apig-Event header string false "流式调用时通过此 Header 指定调用事件"
// @Param        X-Log-Type header string false "指定函数调用是否需要日志回显，Tail标识需要回显"
// @Param        X-Pool-Label header string false "指定函数实例池化启动时使用的资源池"
// @Param        function-urn path string true "用户函数的URN"
// @Param        invoke-event body string true "用户函数处理事件"
// @Success      200  {string}  string "调用成功返回，格式由用户函数决定"
// @Failure      500  {object}  types.InvokeErrorResponse "调用报错返回，包含错误码和错误信息"
// @Header       200,500  {string}  X-Inner-Code "调用结果内部返回码"
// @Header       200  {string}  X-Billing-Duration "本次调用计费信息"
// @Header       200  {string}  X-Invoke-Summary "本次调用摘要信息"
// @Header       200  {string}  X-Log-Result "调用过程中产生日志"
func InvokeHandler(ctx *gin.Context) {
	traceID := httputil.InitTraceID(ctx)
	logger := log.GetLogger().With(zap.Any("traceId", traceID))
	logger.Infof("invoking handler receives one request")

	processCtx, err := buildProcessContext(ctx, traceID)
	if err != nil {
		logger.Errorf("failed to set processCtx req, error: %s", err.Error())
		writeHTTPResponse(ctx, processCtx)
		return
	}
	defer writeInterfaceLog(processCtx)
	logger = logger.With(zap.Any("funcKey", processCtx.FuncKey))
	if err := middleware.Invoker.Handle(processCtx); err != nil {
		logger.Errorf("invoke failed,error: %s", err.Error())
	}
	writeHTTPResponse(ctx, processCtx)
	sessionId := processCtx.ReqHeader[httpconstant.HeaderInstanceSession]
	instanceLabel := processCtx.ReqHeader[httpconstant.HeaderInstanceLabel]
	logger.Infof("invoke function success, totalTime %f, sessionId %s, instanceLabel %s",
		time.Since(processCtx.StartTime).Seconds(), sessionId, instanceLabel)
}

func writeInterfaceLog(invokeCtx *types.InvokeProcessContext) {
	if invokeCtx.RequestTraceInfo == nil {
		log.GetLogger().Errorf("write invoke interface log failed, traceIno is nil")
		return
	}

	totalTime := time.Since(invokeCtx.StartTime)
	tenantId := invokeCtx.RequestTraceInfo.TenantID
	funcName := invokeCtx.RequestTraceInfo.FuncName
	version := invokeCtx.RequestTraceInfo.Version

	splits := strings.Split(invokeCtx.FuncKey, urnutils.FunctionKeySep) // {tenantid}@{funtionName}@{version}

	if len(splits) == 3 && config.GetConfig().BusinessType != constant.BusinessTypeFG { // magicnumber
		tenantId = splits[0] // tenantIdIndex
		funcName = splits[1] // funcNameIndex
		version = splits[2]  // versionIndex
	}

	message := "OK"
	if invokeCtx.StatusCode != http.StatusOK {
		message = string(invokeCtx.RespBody)
	}
	if len(message) > 100 { // 仅保留前100个字符
		message = message[:100] // 仅保留前100个字符
	}
	// tenantId | funcName | version | traceId | instanceLabel | sessionId | statusCode | code | totalCost |
	logContent := fmt.Sprintf("invocation |%s | %s | %s | %s | %s | %s | %d | %s | %.2f | %s",
		tenantId, funcName, version,
		invokeCtx.TraceID,
		invokeCtx.ReqHeader[httpconstant.HeaderInstanceLabel],
		invokeCtx.ReqHeader[httpconstant.HeaderInstanceSession],
		invokeCtx.StatusCode,
		invokeCtx.RespHeader[httpconstant.HeaderInnerCode],
		totalTime.Seconds()*1000, // 秒转换成毫秒
		message)

	frontendlog.Write(logContent)
}

func buildProcessContext(ctx *gin.Context, traceID string) (processCtx *types.InvokeProcessContext, err error) {
	processCtx = types.CreateInvokeProcessContext()
	processCtx.TraceID = traceID
	processCtx.RequestID = traceID
	processCtx.ResponseWriter = &GinWriter{
		Context: ctx,
	}

	var (
		funcUrn  urnutils.FunctionURN
		plainURN string
	)
	defer func() {
		if err != nil {
			processCtx.StatusCode = http.StatusBadRequest
			responsehandler.SetErrorInContextWithDefault(processCtx, err, statuscode.FrontendStatusBadRequest,
				err.Error())
		}
	}()
	err = handleRequestBodyAndStream(ctx, processCtx, traceID)
	if err != nil {
		return
	}
	processCtx.ReqHeader = readHeaders(ctx.Request.Header)
	processCtx.ReqPath = ctx.Request.URL.Path
	processCtx.ReqMethod = ctx.Request.Method
	processCtx.ReqQuery = ctx.Request.URL.RawQuery
	funcUrn, plainURN, err = extractFunctionURN(ctx, processCtx.ReqHeader)
	if err != nil {
		return
	}
	processCtx.FuncKey = urnutils.CombineFunctionKey(funcUrn.TenantID, funcUrn.FuncName, funcUrn.FuncVersion)
	if config.GetConfig().BusinessType == constant.BusinessTypeFG {
		if err = processContextForFG(ctx, processCtx, plainURN, funcUrn); err != nil {
			return
		}
	}
	return
}

// GinWriter 实现 ResponseWriter 接口
type GinWriter struct {
	Context *gin.Context
}

// SSEWrite SSE协议响应回传
func (gw *GinWriter) SSEWrite(data []byte) (int, error) {
	// 设置SSE响应头
	if gw == nil || gw.Context == nil {
		return 0, fmt.Errorf("context has nil")
	}
	gw.Context.Header("Content-Type", constant.HeaderAcceptEventStream)
	gw.Context.Header("Cache-Control", "no-cache")
	gw.Context.Header("Connection", "keep-alive")
	gw.Context.Header("Transfer-Encoding", "chunked")
	gw.Context.Header("X-Accel-Buffering", "no")

	// 尝试写入数据，最多重试3次
	sseEvent := fmt.Sprintf("data: %s\n\n", string(data))
	maxRetries := 3
	retryCount := 0
	writer := gw.Context.Writer
	for {
		if writer == nil {
			break
		}
		_, err := writer.Write([]byte(sseEvent))
		if err == nil {
			break
		}
		retryCount++
		if retryCount >= maxRetries {
			log.GetLogger().Errorf("SSE write response failed after %d retries, err: %v", maxRetries, err)
			gw.Context.Abort()
			return 0, err
		}
		time.Sleep(time.Second)
	}

	// 立即发送数据到客户端
	if writer != nil {
		writer.Flush()
	}
	return 0, nil
}

// ClientDisconnectChan -
func (gw *GinWriter) ClientDisconnectChan() <-chan struct{} {
	return gw.Context.Request.Context().Done()
}

func handleRequestBodyAndStream(ctx *gin.Context, processCtx *types.InvokeProcessContext, traceID string) error {
	stream.BuildStreamContext(ctx, processCtx)
	if stream.IsHTTPUploadStream(ctx.Request) {
		if !config.GetConfig().StreamEnable {
			log.GetLogger().With(zap.String("traceID", traceID)).Warnf("not enable to support http stream")
			return snerror.New(statuscode.HTTPStreamNOTEnableError, statuscode.InternalErrorMessage)
		}
		processCtx.IsHTTPUploadStream = true
	} else {
		var err error
		processCtx.ReqBody, err = ioutil.ReadAll(ctx.Request.Body)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeHTTPResponse(ctx *gin.Context, processCtx *types.InvokeProcessContext) {
	// It has to be in this order. 1. set header 2.writeHeader 3.write
	writeHeadersToResponse(processCtx.RespHeader, ctx.Writer.Header())
	ctx.Writer.WriteHeader(processCtx.StatusCode)
	sseHeader, ok := processCtx.ReqHeader["Accept"]
	if ok && sseHeader == constant.HeaderAcceptEventStream {
		_, err := processCtx.ResponseWriter.SSEWrite(processCtx.RespBody)
		if err != nil {
			log.GetLogger().Errorf("failed to write response body error %s", err.Error())
		}
		_, err = processCtx.ResponseWriter.SSEWrite([]byte("[DONE]"))
		if err != nil {
			log.GetLogger().Errorf("failed to write DONE error %s", err.Error())
		}
	} else {
		_, err := ctx.Writer.Write(processCtx.RespBody)
		if err != nil {
			log.GetLogger().Errorf("failed to write response body error %s", err.Error())
		}
	}
}

func readHeaders(header http.Header) map[string]string {
	headerMap := make(map[string]string)
	for key := range header {
		headerMap[key] = header.Get(key)
	}
	return headerMap
}

func writeHeadersToResponse(headerMap map[string]string, header http.Header) {
	for key, value := range headerMap {
		header.Set(key, value)
	}
}

func extractFunctionURN(c *gin.Context, reqHeaders map[string]string) (urnutils.FunctionURN, string, error) {
	plainURN := c.Param(common.FunctionUrnParam)
	params := make(map[string]string)
	for k, v := range reqHeaders {
		params[strings.ToLower(k)] = v
	}
	functionURN := aliasroute.GetAliases().GetFuncVersionURNWithParams(plainURN, params)
	functionInfo, err := urnutils.GetFunctionInfo(functionURN)
	if err != nil {
		return urnutils.FunctionURN{}, "", err
	}
	return functionInfo, plainURN, nil
}

func processContextForFG(c *gin.Context, processCtx *types.InvokeProcessContext,
	plainURN string, functionInfo urnutils.FunctionURN) error {
	anonymizeURN := urnutils.AnonymizeTenantURN(plainURN)

	log.GetLogger().Debugf("request URN is coming: %s, alias: %s traceID: %s",
		anonymizeURN, c.Request.Header.Get(constant.HeaderInvokeAlias), processCtx.TraceID)

	if err := functionInfo.Valid(); err != nil {
		return fmt.Errorf("invalid function name,err is %s", err)
	}
	if functionInfo.BusinessID == "" || functionInfo.TenantID == "" || functionInfo.FuncName == "" {
		return fmt.Errorf("wrong function name %s", plainURN)
	}

	urn, version := getURNWithVersion(functionInfo.FuncVersion, plainURN)
	processCtx.RequestTraceInfo = &types.RequestTraceInfo{
		URN:          urn,
		AnonymizeURN: anonymizeURN,
		BusinessID:   functionInfo.BusinessID,
		TenantID:     functionInfo.TenantID,
		FuncName:     functionInfo.FuncName,
		Version:      version,
	}
	return nil
}

func getURNWithVersion(version string, plainURN string) (string, string) {
	var newURN string
	if version == "" {
		version = "latest"
		newURN = plainURN + urnutils.URNSep + version
	} else {
		newURN = plainURN
	}
	return newURN, version
}
