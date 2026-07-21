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
	"frontend/pkg/frontend/sandboxrouter/execendpoint"
)

type handlerTestLibruntime struct {
	*mockUtils.FakeLibruntimeSdkClient
	createRaw func([]byte, api.RawRequestOption) ([]byte, error)
	invokeRaw func([]byte, api.RawRequestOption) ([]byte, error)
	killRaw   func([]byte, api.RawRequestOption) ([]byte, error)
}

func newHandlerRequest(t testing.TB, method, target, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
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

func TestKillHandler(t *testing.T) {
	convey.Convey("test KillHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		addKillReadErrorCase(t)
		addKillBasicCases(t, mock)
		addKillJSONSuccessCase(t, mock)
		addKillJSONNotFoundCase(t, mock)
		addKillInvalidJSONCase(t)
	})
}

func newKillHandlerContext(t testing.TB, body, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newHandlerRequest(t, http.MethodPost, "/test", body)
	ctx.Request.Header.Set(constant.HeaderRemoteClientId, "test-client-id")
	if contentType != "" {
		ctx.Request.Header.Set("Content-Type", contentType)
	}
	return ctx, recorder
}

func addKillReadErrorCase(t testing.TB) {
	convey.Convey("read body error", func() {
		patches := gomonkey.ApplyFunc(io.ReadAll, func(io.Reader) ([]byte, error) {
			return nil, errors.New("read body error")
		})
		defer patches.Reset()
		ctx, recorder := newKillHandlerContext(t, `"test body"`, "")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusInternalServerError)
	})
}

func addKillBasicCases(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("KillHandler success", func() {
		ctx, recorder := newKillHandlerContext(t, `"test body"`, "")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusOK)
	})
	convey.Convey("KillHandler failed", func() {
		util.SetAPIClientLibruntime(&handlerTestLibruntime{
			FakeLibruntimeSdkClient: mock,
			killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
				return nil, errors.New("kill raw error")
			},
		})
		ctx, recorder := newKillHandlerContext(t, `"test body"`, "")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusBadRequest)
	})
}

func addKillJSONSuccessCase(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("KillHandler json body transcoded to protobuf", func() {
		var captured []byte
		util.SetAPIClientLibruntime(&handlerTestLibruntime{
			FakeLibruntimeSdkClient: mock,
			killRaw: func(req []byte, _ api.RawRequestOption) ([]byte, error) {
				captured = req
				return proto.Marshal(&core.KillResponse{})
			},
		})
		ctx, recorder := newKillHandlerContext(
			t, `{"instanceID":"inst-json-1","signal":1}`, "application/json")
		KillHandler(ctx)
		decoded := &core.KillRequest{}
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusOK)
		convey.So(proto.Unmarshal(captured, decoded), convey.ShouldBeNil)
		convey.So(decoded.GetInstanceID(), convey.ShouldEqual, "inst-json-1")
		convey.So(decoded.GetSignal(), convey.ShouldEqual, 1)
		convey.So(recorder.Header().Get("Content-Type"), convey.ShouldEqual, "application/json")
		convey.So(recorder.Body.String(), convey.ShouldContainSubstring, `"code":0`)
	})
}

func addKillJSONNotFoundCase(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("KillHandler json not-found code surfaced to client", func() {
		util.SetAPIClientLibruntime(&handlerTestLibruntime{
			FakeLibruntimeSdkClient: mock,
			killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
				return proto.Marshal(&core.KillResponse{Code: 22, Message: "instance not found"})
			},
		})
		ctx, recorder := newKillHandlerContext(t, `{"instanceID":"missing","signal":1}`, "application/json")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusOK)
		convey.So(recorder.Body.String(), convey.ShouldContainSubstring, `"code":22`)
		convey.So(recorder.Body.String(), convey.ShouldContainSubstring, "instance not found")
	})
}

func addKillInvalidJSONCase(t testing.TB) {
	convey.Convey("KillHandler invalid json body returns bad request", func() {
		ctx, recorder := newKillHandlerContext(t, `{not-json`, "application/json")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusBadRequest)
	})
}

func TestKillHandlerTenantAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	convey.Convey("KillHandler tenant authorization", t, func() {
		prepareKillAuthorizationStore()
		defer clearKillAuthorizationStore()
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		addCrossTenantKillCase(t, mock)
		addHeaderKillAuthorizationCase(t, mock)
		addAllowedTenantKillCase(t, mock, "user1", "owned-by-user1")
		addAllowedTenantKillCase(t, mock, "0", "owned-by-user2")
		addKillAllInstancesCase(t, mock)
		addMissingInstanceKillCase(t, mock)
	})
}

func prepareKillAuthorizationStore() {
	store := execendpoint.Default()
	clearKillAuthorizationStore()
	store.PutSummary(execendpoint.Summary{InstanceID: "owned-by-user1", TenantID: "user1", StatusCode: 3})
	store.PutSummary(execendpoint.Summary{InstanceID: "owned-by-user2", TenantID: "user2", StatusCode: 3})
}

func clearKillAuthorizationStore() {
	store := execendpoint.Default()
	store.Delete("owned-by-user1")
	store.Delete("owned-by-user2")
	store.Delete("missing-for-auth")
}

func newAuthorizedKillContext(t testing.TB, sub, instanceID string) (*gin.Context, *httptest.ResponseRecorder) {
	ctx, recorder := newKillHandlerContext(
		t, fmt.Sprintf(`{"instanceID":%q,"signal":1}`, instanceID), "application/json")
	ctx.Set("jwt_sub", sub)
	ctx.Set("jwt_role", jwtauth.RoleDeveloper)
	return ctx, recorder
}

func addCrossTenantKillCase(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("developer tenant cannot delete another tenant instance", func() {
		called := false
		util.SetAPIClientLibruntime(killTrackingClient(mock, &called, nil))
		ctx, recorder := newAuthorizedKillContext(t, "user1", "owned-by-user2")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusForbidden)
		convey.So(called, convey.ShouldBeFalse)
	})
}

func addHeaderKillAuthorizationCase(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("X-Auth header is enforced even when middleware context is absent", func() {
		called := false
		patches := gomonkey.ApplyFunc(jwtauth.ValidateWithIamServer, func(string, string) error { return nil })
		defer patches.Reset()
		util.SetAPIClientLibruntime(killTrackingClient(mock, &called, nil))
		ctx, recorder := newKillHandlerContext(
			t, `{"instanceID":"owned-by-user2","signal":1}`, "application/json")
		ctx.Request.Header.Set(jwtauth.HeaderXAuth, functionSystemTestJWT("user1", jwtauth.RoleDeveloper))
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusForbidden)
		convey.So(called, convey.ShouldBeFalse)
	})
}

func addAllowedTenantKillCase(
	t testing.TB,
	mock *mockUtils.FakeLibruntimeSdkClient,
	sub string,
	instanceID string,
) {
	convey.Convey("authorized tenant can delete instance "+instanceID, func() {
		called := false
		util.SetAPIClientLibruntime(killTrackingClient(mock, &called, &core.KillResponse{}))
		ctx, recorder := newAuthorizedKillContext(t, sub, instanceID)
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusOK)
		convey.So(called, convey.ShouldBeTrue)
	})
}

func addMissingInstanceKillCase(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("authenticated delete of uncached instance is rejected before runtime", func() {
		called := false
		util.SetAPIClientLibruntime(killTrackingClient(mock, &called, nil))
		ctx, recorder := newAuthorizedKillContext(t, "user1", "missing-for-auth")
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusNotFound)
		convey.So(called, convey.ShouldBeFalse)
	})
}

func addKillAllInstancesCase(t testing.TB, mock *mockUtils.FakeLibruntimeSdkClient) {
	convey.Convey("authenticated driver can finalize a job not stored in the instance cache", func() {
		called := false
		util.SetAPIClientLibruntime(killTrackingClient(mock, &called, &core.KillResponse{}))
		ctx, recorder := newKillHandlerContext(
			t, `{"instanceID":"job-finalize","signal":2}`, "application/json")
		ctx.Request.Header.Set(constant.HeaderRemoteClientId, "job-finalize")
		ctx.Set("jwt_sub", "user1")
		ctx.Set("jwt_role", jwtauth.RoleDeveloper)
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusOK)
		convey.So(called, convey.ShouldBeTrue)
	})
	convey.Convey("kill-all job must match the remote client", func() {
		called := false
		util.SetAPIClientLibruntime(killTrackingClient(mock, &called, nil))
		ctx, recorder := newKillHandlerContext(
			t, `{"instanceID":"job-other","signal":2}`, "application/json")
		ctx.Set("jwt_sub", "user1")
		ctx.Set("jwt_role", jwtauth.RoleDeveloper)
		KillHandler(ctx)
		convey.So(recorder.Code, convey.ShouldEqual, http.StatusForbidden)
		convey.So(called, convey.ShouldBeFalse)
	})
}

func killTrackingClient(
	mock *mockUtils.FakeLibruntimeSdkClient,
	called *bool,
	response *core.KillResponse,
) *handlerTestLibruntime {
	return &handlerTestLibruntime{
		FakeLibruntimeSdkClient: mock,
		killRaw: func([]byte, api.RawRequestOption) ([]byte, error) {
			*called = true
			if response == nil {
				return nil, nil
			}
			return proto.Marshal(response)
		},
	}
}

func functionSystemTestJWT(sub, role string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"sub":%q,"role":%q,"exp":%d}`,
		sub, role, time.Now().Add(time.Hour).Unix(),
	)))
	return header + "." + payload + ".signature"
}
