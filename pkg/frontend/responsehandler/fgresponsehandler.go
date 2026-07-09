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

package responsehandler

import (
	"strconv"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/types"
)

// FGResponseHandler -
type FGResponseHandler struct{}

// SetResponseFromInvocation -
func (f *FGResponseHandler) SetResponseFromInvocation(ctx *types.InvokeProcessContext,
	message []byte) (*types.CallResp, snerror.SNError) {
	if len(ctx.RespHeader) > 0 {
		log.GetLogger().Errorf("response has been written, traceID: %s", ctx.TraceID)
		return &types.CallResp{}, snerror.New(statuscode.FrontendStatusInternalError,
			"response has been written")
	}
	respMsg, err := util.UnmarshalCallResp(message)
	if err != nil {
		log.GetLogger().Errorf("failed to translate response data, traceID: %s, err: %s",
			ctx.TraceID, err.Error())
		return &types.CallResp{}, snerror.New(statuscode.FrontendStatusInternalError, err.Error())
	}

	innerCode, err := strconv.Atoi(respMsg.InnerCode)
	if err != nil {
		log.GetLogger().Errorf("failed to get the innerCode, traceID: %s, err: %s",
			ctx.TraceID, err.Error())
		return respMsg, snerror.New(statuscode.FrontendStatusInternalError, err.Error())
	}

	body, err := generateRespBodyToUser(innerCode, respMsg.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to generate the returned body, traceID: %s, err: %s",
			ctx.TraceID, err.Error())
		return respMsg, snerror.New(statuscode.FrontendStatusInternalError, err.Error())
	}
	setHTTPHeader(ctx, respMsg)
	ctx.StatusCode = statuscode.Code(innerCode)
	ctx.RespBody = body
	return respMsg, nil
}

// SetResponseFromFrontend -
func (f *FGResponseHandler) SetResponseFromFrontend(ctx *types.InvokeProcessContext,
	innerCode int, message interface{}) {
	if ctx.RespHeader == nil {
		ctx.RespHeader = make(map[string]string)
	}
	if len(ctx.RespHeader) > 0 {
		return
	}
	ctx.StatusCode = statuscode.Code(innerCode)
	ctx.RespHeader[constant.HeaderInnerCode] = strconv.Itoa(innerCode)
	buildResponse(ctx, innerCode, message)
}

func setHTTPHeader(ctx *types.InvokeProcessContext, respMsg *types.CallResp) {
	ctx.RespHeader[constant.HeaderContentType] = httpconstant.ApplicationJSON
	ctx.RespHeader[constant.HeaderInnerCode] = respMsg.InnerCode
	ctx.RespHeader[constant.HeaderLogResult] = respMsg.LogResult
	ctx.RespHeader[constant.HeaderInvokeSummary] = respMsg.InvokeSummary
	ctx.RespHeader[constant.HeaderBillingDuration] = respMsg.BillingDuration
	if ctx.NeedReadRespHeader {
		for k, v := range respMsg.Headers {
			ctx.RespHeader[k] = v
		}
	}
}
