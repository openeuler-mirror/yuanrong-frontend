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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/grpc/pb/core"
	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/sandboxrouter/execendpoint"
)

type handlerTestLibruntime struct {
	*mockUtils.FakeLibruntimeSdkClient
	createRaw func([]byte, api.RawRequestOption) ([]byte, error)
	invokeRaw func([]byte, api.RawRequestOption) ([]byte, error)
	killRaw   func([]byte, api.RawRequestOption) ([]byte, error)
}

func (f *handlerTestLibruntime) CreateInstanceRaw(req []byte, option api.RawRequestOption) ([]byte, error) {
	if f.createRaw != nil {
		return f.createRaw(req, option)
	}
	return f.FakeLibruntimeSdkClient.CreateInstanceRaw(req, option)
}

func (f *handlerTestLibruntime) InvokeByInstanceIdRaw(req []byte, option api.RawRequestOption) ([]byte, error) {
	if f.invokeRaw != nil {
		return f.invokeRaw(req, option)
	}
	return f.FakeLibruntimeSdkClient.InvokeByInstanceIdRaw(req, option)
}

func (f *handlerTestLibruntime) KillRaw(req []byte, option api.RawRequestOption) ([]byte, error) {
	if f.killRaw != nil {
		return f.killRaw(req, option)
	}
	return f.FakeLibruntimeSdkClient.KillRaw(req, option)
}

func Test_CreateHandler(t *testing.T) {
	convey.Convey("test CreateHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		convey.Convey("read body error", func() {
			// Reset patches immediately after the handler call. In nested GoConvey
			// cases, defer runs at the parent scope and can leak patches into siblings.
			patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
				return []byte{}, errors.New("read body error")
			})
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			CreateHandler(ctx)
			patches.Reset()
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
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				createRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					return []byte{}, errors.New("CreateInstanceRaw error")
				},
			})
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
			patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
				return []byte{}, errors.New("read body error")
			})
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set(constant.HeaderRemoteClientId, "test-client-id")
			InvokeHandler(ctx)
			patches.Reset()
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
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				invokeRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					return []byte{}, errors.New("InvokeByInstanceIdRaw error")
				},
			})
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
			patches := gomonkey.ApplyFunc(io.ReadAll, func(r io.Reader) ([]byte, error) {
				return []byte{}, errors.New("read body error")
			})
			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			reqBody := "test body"
			bodyMarshal, _ := json.Marshal(reqBody)
			ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
			ctx.Request.Header.Set("remoteClientId", "test-client-id")
			KillHandler(ctx)
			patches.Reset()
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
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					return []byte{}, errors.New("KillRaw error")
				},
			})
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
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func(req []byte, option api.RawRequestOption) ([]byte, error) {
					captured = req
					resp := &core.KillResponse{Code: 0, Message: ""}
					out, _ := proto.Marshal(resp)
					return out, nil
				},
			})
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
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					resp := &core.KillResponse{Message: "instance not found"}
					resp.Code = 22 // a non-zero error code
					out, _ := proto.Marshal(resp)
					return out, nil
				},
			})
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

func TestKillHandlerTenantAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	convey.Convey("KillHandler tenant authorization", t, func() {
		store := execendpoint.Default()
		store.Delete("owned-by-user1")
		store.Delete("owned-by-user2")
		store.Delete("missing-for-auth")
		defer store.Delete("owned-by-user1")
		defer store.Delete("owned-by-user2")
		defer store.Delete("missing-for-auth")
		store.PutSummary(execendpoint.Summary{InstanceID: "owned-by-user1", TenantID: "user1", StatusCode: 3})
		store.PutSummary(execendpoint.Summary{InstanceID: "owned-by-user2", TenantID: "user2", StatusCode: 3})

		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)

		convey.Convey("developer tenant cannot delete another tenant instance", func() {
			called := false
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					called = true
					return []byte{}, nil
				},
			})

			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Set("jwt_sub", "user1")
			ctx.Set("jwt_role", "developer")
			ctx.Request, _ = http.NewRequest(http.MethodPost, "/frontend/v1/instance/kill",
				bytes.NewBufferString(`{"instanceID":"owned-by-user2","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")

			KillHandler(ctx)

			convey.So(rw.Code, convey.ShouldEqual, http.StatusForbidden)
			convey.So(called, convey.ShouldBeFalse)
		})

		convey.Convey("X-Auth header is enforced even when middleware context is absent", func() {
			called := false
			validatePatches := gomonkey.ApplyFunc(jwtauth.ValidateWithIamServer, func(string, string) error {
				return nil
			})
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					called = true
					return []byte{}, nil
				},
			})

			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Request, _ = http.NewRequest(http.MethodPost, "/frontend/v1/instance/kill",
				bytes.NewBufferString(`{"instanceID":"owned-by-user2","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Request.Header.Set(jwtauth.HeaderXAuth, functionSystemTestJWT("user1", jwtauth.RoleDeveloper))

			KillHandler(ctx)
			validatePatches.Reset()

			convey.So(rw.Code, convey.ShouldEqual, http.StatusForbidden)
			convey.So(called, convey.ShouldBeFalse)
		})

		convey.Convey("developer tenant can delete own instance", func() {
			called := false
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					called = true
					resp := &core.KillResponse{Code: 0, Message: ""}
					out, _ := proto.Marshal(resp)
					return out, nil
				},
			})

			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Set("jwt_sub", "user1")
			ctx.Set("jwt_role", "developer")
			ctx.Request, _ = http.NewRequest(http.MethodPost, "/frontend/v1/instance/kill",
				bytes.NewBufferString(`{"instanceID":"owned-by-user1","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")

			KillHandler(ctx)

			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(called, convey.ShouldBeTrue)
		})

		convey.Convey("system developer tenant can delete any tenant instance", func() {
			called := false
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					called = true
					resp := &core.KillResponse{Code: 0, Message: ""}
					out, _ := proto.Marshal(resp)
					return out, nil
				},
			})

			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Set("jwt_sub", "0")
			ctx.Set("jwt_role", "developer")
			ctx.Request, _ = http.NewRequest(http.MethodPost, "/frontend/v1/instance/kill",
				bytes.NewBufferString(`{"instanceID":"owned-by-user2","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")

			KillHandler(ctx)

			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(called, convey.ShouldBeTrue)
		})

		convey.Convey("authenticated delete of uncached instance is rejected before runtime", func() {
			called := false
			util.SetAPIClientLibruntime(&handlerTestLibruntime{
				FakeLibruntimeSdkClient: mock,
				killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
					called = true
					return []byte{}, nil
				},
			})

			rw := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rw)
			ctx.Set("jwt_sub", "user1")
			ctx.Set("jwt_role", "developer")
			ctx.Request, _ = http.NewRequest(http.MethodPost, "/frontend/v1/instance/kill",
				bytes.NewBufferString(`{"instanceID":"missing-for-auth","signal":1}`))
			ctx.Request.Header.Set("Content-Type", "application/json")

			KillHandler(ctx)

			convey.So(rw.Code, convey.ShouldEqual, http.StatusNotFound)
			convey.So(called, convey.ShouldBeFalse)
		})
	})
}

func functionSystemTestJWT(sub, role string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"sub":%q,"role":%q,"exp":%d}`,
		sub, role, time.Now().Add(time.Hour).Unix(),
	)))
	return header + "." + payload + ".signature"
}
