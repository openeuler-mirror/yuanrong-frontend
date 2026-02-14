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

// Package leaseadaptor -
package leaseadaptor

import (
	"encoding/json"
	"fmt"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	commontypes "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/types"
)

func convertInvokeTag(ctx *types.InvokeProcessContext) map[string]string {
	m := make(map[string]string)
	headerValue := util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderInvokeTag)
	if headerValue == "" {
		return m
	}
	err := json.Unmarshal([]byte(headerValue), &m)
	if err != nil {
		log.GetLogger().Errorf("convert invoke tag failed, traceId: %s, err: %s", ctx.TraceID, err.Error())
		return make(map[string]string)
	}
	return m
}

func getTimeout(funcSpecTimeout int64, ctxTimeout int64) int64 {
	if ctxTimeout != 0 {
		return ctxTimeout
	}
	return funcSpecTimeout
}

func makeAcquireOption(ctx *types.InvokeProcessContext, funcSpec *commontypes.FuncSpec) (
	*commontypes.AcquireOption, snerror.SNError) {
	acquireOption := &commontypes.AcquireOption{
		DesignateInstanceID: "",
		RequestID:           "",
		TraceID:             ctx.TraceID,
		FuncSig:             funcSpec.FuncMetaSignature,
		Timeout:             getTimeout(util.GetAcquireTimeout(funcSpec), ctx.AcquireTimeout),
		TrafficLimited:      ctx.TrafficLimited,
		PoolLabel:           util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderPoolLabel),
		InvokeTag:           convertInvokeTag(ctx),
		InstanceLabel:       util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderInstanceLabel),
	}

	var err error
	acquireOption.ResourceSpecs, err = util.ConvertResourceSpecs(ctx, funcSpec)
	if err != nil {
		return nil, snerror.NewWithError(statuscode.FrontendStatusInternalError, err)
	}
	instanceSession := util.PeekIgnoreCase(ctx.ReqHeader, httpconstant.HeaderInstanceSession)
	if instanceSession != "" {
		session := &commontypes.InstanceSessionConfig{}
		err = json.Unmarshal([]byte(instanceSession), &session)
		if err != nil {
			return nil, snerror.NewWithError(statuscode.FrontendStatusInternalError,
				fmt.Errorf("unmarshal session request header, header: %s, err: %s", instanceSession, err.Error()))
		}
		acquireOption.InstanceSession = session
	}
	return acquireOption, nil
}
