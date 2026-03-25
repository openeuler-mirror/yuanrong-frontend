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

package v1

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/datasystemclient"
	"frontend/pkg/common/faas_common/snerror"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/invocation"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/types"
)

const testFunctionURN = "sn:cn:yrk:12345678901234561234567890123456:function:0@yrservice@test-faas-python-runtime-001"

func TestInterruptSessionHandler(t *testing.T) {
	convey.Convey("interrupt request should query session and invoke resolved instance", t, func() {
		gin.SetMode(gin.TestMode)
		oldResponseHandler := responsehandler.Handler
		defer func() { responsehandler.Handler = oldResponseHandler }()

		responsehandler.Handler = (&invocation.FGAdapter{}).MakeResponseHandler()
		var capturedSessionID string
		var capturedFuncKey string
		var capturedInstanceID string
		var capturedPath string
		var capturedBody string
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(leaseadaptor.QuerySession,
			func(funcKey, sessionID, traceID string) (*commontype.InstanceAllocationInfo, snerror.SNError) {
				capturedFuncKey = funcKey
				capturedSessionID = sessionID
				return &commontype.InstanceAllocationInfo{InstanceID: "instance-1"}, nil
			})
		patches.ApplyFunc(invocation.InvokeResolvedInstance,
			func(processCtx *types.InvokeProcessContext, instanceID string) error {
				capturedInstanceID = instanceID
				capturedPath = processCtx.ReqPath
				capturedBody = string(processCtx.ReqBody)
				processCtx.StatusCode = http.StatusOK
				processCtx.RespBody = []byte("ok")
				return nil
			})

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		body := []byte(`{"message":"interrupt"}`)
		ctx.Request, _ = http.NewRequest(http.MethodPost,
			"/serverless/v1/functions/"+testFunctionURN+"/sessions/session-1/interrupt",
			bytes.NewBuffer(body))
		ctx.AddParam("function-urn", testFunctionURN)
		ctx.AddParam(sessionIDParam, "session-1")

		processCtx, err := buildProcessContext(ctx, "trace-1")
		convey.So(err, convey.ShouldBeNil)
		err = setInstanceSessionHeader(processCtx.ReqHeader, "session-1")
		convey.So(err, convey.ShouldBeNil)
		err = handleInterruptRequest(processCtx)
		convey.So(err, convey.ShouldBeNil)

		convey.So(capturedPath, convey.ShouldEqual,
			"/serverless/v1/functions/"+testFunctionURN+"/sessions/session-1/interrupt")
		convey.So(capturedBody, convey.ShouldEqual, `{"message":"interrupt"}`)
		convey.So(capturedFuncKey, convey.ShouldEqual, "12345678901234561234567890123456/0@yrservice@test-faas-python-runtime-001/")
		convey.So(capturedSessionID, convey.ShouldEqual, "session-1")
		convey.So(capturedInstanceID, convey.ShouldEqual, "instance-1")
		convey.So(processCtx.StatusCode, convey.ShouldEqual, http.StatusOK)
		convey.So(string(processCtx.RespBody), convey.ShouldEqual, "ok")
		_ = rw
	})
}

func TestDeleteSessionHandler(t *testing.T) {
	convey.Convey("delete session success", t, func() {
		var capturedKey string
		var capturedTenantID string
		patch := gomonkey.ApplyFunc(datasystemclient.KVDelWithRetry,
			func(key string, option *datasystemclient.Option, traceID string) error {
				capturedKey = key
				capturedTenantID = option.TenantID
				return nil
			})
		defer patch.Reset()

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest(http.MethodDelete,
			"/serverless/v1/functions/"+testFunctionURN+"/sessions/session-1", nil)
		ctx.AddParam("function-urn", testFunctionURN)
		ctx.AddParam(sessionIDParam, "session-1")

		DeleteSessionHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		convey.So(capturedTenantID, convey.ShouldEqual, "12345678901234561234567890123456")
		convey.So(capturedKey, convey.ShouldEqual,
			"12345678901234561234567890123456/0@yrservice@test-faas-python-runtime-001/-session-1")
	})

	convey.Convey("delete session failed", t, func() {
		patch := gomonkey.ApplyFunc(datasystemclient.KVDelWithRetry,
			func(key string, option *datasystemclient.Option, traceID string) error {
				return errors.New("delete failed")
			})
		defer patch.Reset()

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest(http.MethodDelete,
			"/serverless/v1/functions/"+testFunctionURN+"/sessions/session-1", nil)
		ctx.AddParam("function-urn", testFunctionURN)
		ctx.AddParam(sessionIDParam, "session-1")

		DeleteSessionHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
		convey.So(rw.Body.String(), convey.ShouldContainSubstring, "delete failed")
	})
}
