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

package frontend

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/grpc/pb/core"
	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/common/util"
)

func Test_CreateHandler(t *testing.T) {
	convey.Convey("test CreateHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		convey.Convey("read body error", func() {
			defer gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
				return []byte{}, errors.New("read body error")
			}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			CreateHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
		})
		convey.Convey("CreateHandler success", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			CreateHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("CreateHandler failed", func() {
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}),
				"CreateInstanceRaw",
				func(_ *mockUtils.FakeLibruntimeSdkClient, createReqRaw []byte,
					option api.RawRequestOption) (createRespRaw []byte, err error) {
					return []byte{}, errors.New("CreateInstanceRaw error")
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			CreateHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
	})
}

func Test_InvokeHandler(t *testing.T) {
	convey.Convey("test InvokeHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		convey.Convey("read body error", func() {
			defer gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
				return []byte{}, errors.New("read body error")
			}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set(constant.HeaderRemoteClientId, "test-client-id")
			InvokeHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
		})
		convey.Convey("InvokeHandler success", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set(constant.HeaderRemoteClientId, "test-client-id")
			InvokeHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("InvokeHandler failed", func() {
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}),
				"InvokeByInstanceIdRaw",
				func(_ *mockUtils.FakeLibruntimeSdkClient, invokeReqRaw []byte,
					option api.RawRequestOption) (resultRaw []byte, err error) {
					return []byte{}, errors.New("InvokeByInstanceIdRaw error")
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set(constant.HeaderRemoteClientId, "test-client-id")
			InvokeHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
	})
}

func Test_KillHandler(t *testing.T) {
	convey.Convey("test KillHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		convey.Convey("read body error", func() {
			defer gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
				return []byte{}, errors.New("read body error")
			}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			KillHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
		})
		convey.Convey("KillHandler success", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			KillHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
		})
		convey.Convey("KillHandler failed", func() {
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}),
				"KillRaw",
				func(_ *mockUtils.FakeLibruntimeSdkClient, killReqRaw []byte,
					option api.RawRequestOption) (killRespRaw []byte, err error) {
					return []byte{}, errors.New("KillRaw error")
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			KillHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
		convey.Convey("KillHandler json body transcoded to protobuf", func() {
			var captured []byte
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}),
				"KillRaw",
				func(_ *mockUtils.FakeLibruntimeSdkClient, killReqRaw []byte,
					option api.RawRequestOption) (killRespRaw []byte, err error) {
					captured = killReqRaw
					resp := &core.KillResponse{Code: 0, Message: ""}
					out, _ := proto.Marshal(resp)
					return out, nil
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Request, _ = http.NewRequest("POST", "/test",
				bytes.NewBufferString(`{"instanceID":"inst-json-1","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			KillHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			// the runtime must still receive a protobuf KillRequest
			decoded := &core.KillRequest{}
			convey.So(proto.Unmarshal(captured, decoded), convey.ShouldBeNil)
			convey.So(decoded.GetInstanceID(), convey.ShouldEqual, "inst-json-1")
			convey.So(decoded.GetSignal(), convey.ShouldEqual, 1)
			// the client gets a JSON response with a numeric code
			convey.So(rw.Header().Get("Content-Type"), convey.ShouldEqual, "application/json")
			convey.So(rw.Body.String(), convey.ShouldContainSubstring, "\"code\":0")
		})
		convey.Convey("KillHandler json not-found code surfaced to client", func() {
			defer gomonkey.ApplyMethod(reflect.TypeOf(&mockUtils.FakeLibruntimeSdkClient{}),
				"KillRaw",
				func(_ *mockUtils.FakeLibruntimeSdkClient, killReqRaw []byte,
					option api.RawRequestOption) (killRespRaw []byte, err error) {
					resp := &core.KillResponse{Message: "instance not found"}
					resp.Code = 22 // a non-zero error code
					out, _ := proto.Marshal(resp)
					return out, nil
				}).Reset()
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Request, _ = http.NewRequest("POST", "/test",
				bytes.NewBufferString(`{"instanceID":"missing","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			KillHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldContainSubstring, "\"code\":22")
			convey.So(rw.Body.String(), convey.ShouldContainSubstring, "instance not found")
		})
		convey.Convey("KillHandler invalid json body returns bad request", func() {
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Request, _ = http.NewRequest("POST", "/test",
				bytes.NewBufferString(`{not-json`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			KillHandler(ctx)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
		})
	})
}
