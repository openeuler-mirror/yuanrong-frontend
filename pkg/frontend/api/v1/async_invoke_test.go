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

package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/frontend/asyncinvocation"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/types"
)

type mockHandlerChain struct {
	handleFunc func(ctx *types.InvokeProcessContext) error
}

func (m *mockHandlerChain) Use(mws ...middleware.Middleware) {}
func (m *mockHandlerChain) Handle(ctx *types.InvokeProcessContext) error {
	return m.handleFunc(ctx)
}

func TestAsyncInvokeHandler_Returns202(t *testing.T) {
	convey.Convey("AsyncInvokeHandler should return 202 with requestId", t, func() {
		gin.SetMode(gin.TestMode)

		patches := gomonkey.ApplyFunc(buildProcessContext, func(ctx *gin.Context, traceID string) (*types.InvokeProcessContext, error) {
			return types.CreateInvokeProcessContext(), nil
		})
		defer patches.Reset()

		origInvoker := middleware.Invoker
		middleware.Invoker = &mockHandlerChain{
			handleFunc: func(ctx *types.InvokeProcessContext) error {
				ctx.StatusCode = http.StatusOK
				ctx.RespBody = []byte("ok")
				return nil
			},
		}
		defer func() { middleware.Invoker = origInvoker }()

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("POST", "/test", nil)

		AsyncInvokeHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusAccepted)

		var resp map[string]string
		err := json.Unmarshal(rw.Body.Bytes(), &resp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(resp["requestId"], convey.ShouldNotBeEmpty)

		// Wait for background goroutine to complete
		time.Sleep(200 * time.Millisecond)

		// Verify result was stored
		result, ok := asyncinvocation.GetAsyncResultStore().Load(resp["requestId"])
		convey.So(ok, convey.ShouldBeTrue)
		convey.So(result.Status, convey.ShouldEqual, asyncinvocation.StatusCompleted)
		convey.So(result.StatusCode, convey.ShouldEqual, http.StatusOK)

		// Cleanup
		asyncinvocation.GetAsyncResultStore().Delete(resp["requestId"])
	})
}

func TestAsyncInvokeHandler_BuildContextError(t *testing.T) {
	convey.Convey("AsyncInvokeHandler should handle buildProcessContext error", t, func() {
		gin.SetMode(gin.TestMode)

		errCtx := types.CreateInvokeProcessContext()
		errCtx.StatusCode = http.StatusBadRequest
		errCtx.RespBody = []byte(`{"code":400,"message":"bad request"}`)
		patches := gomonkey.ApplyFunc(buildProcessContext, func(ctx *gin.Context, traceID string) (*types.InvokeProcessContext, error) {
			return errCtx, fmt.Errorf("missing function-urn")
		})
		defer patches.Reset()

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("POST", "/test", nil)

		AsyncInvokeHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
	})
}

func TestGetAsyncResultHandler_NotFound(t *testing.T) {
	convey.Convey("GetAsyncResultHandler should return 404 for unknown requestId", t, func() {
		gin.SetMode(gin.TestMode)

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("GET", "/test", nil)
		ctx.AddParam("request-id", "nonexistent-id")

		GetAsyncResultHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusNotFound)
	})
}

func TestGetAsyncResultHandler_Pending(t *testing.T) {
	convey.Convey("GetAsyncResultHandler should return pending status", t, func() {
		gin.SetMode(gin.TestMode)
		store := asyncinvocation.GetAsyncResultStore()
		store.Store("pending-req", &asyncinvocation.AsyncResult{
			RequestID: "pending-req",
			Status:    asyncinvocation.StatusPending,
			CreatedAt: time.Now(),
		})
		defer store.Delete("pending-req")

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("GET", "/test", nil)
		ctx.AddParam("request-id", "pending-req")

		GetAsyncResultHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)

		var resp map[string]string
		err := json.Unmarshal(rw.Body.Bytes(), &resp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(resp["requestId"], convey.ShouldEqual, "pending-req")
		convey.So(resp["status"], convey.ShouldEqual, "pending")
	})
}

func TestGetAsyncResultHandler_Running(t *testing.T) {
	convey.Convey("GetAsyncResultHandler should return running status", t, func() {
		gin.SetMode(gin.TestMode)
		store := asyncinvocation.GetAsyncResultStore()
		store.Store("running-req", &asyncinvocation.AsyncResult{
			RequestID: "running-req",
			Status:    asyncinvocation.StatusRunning,
			CreatedAt: time.Now(),
		})
		defer store.Delete("running-req")

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("GET", "/test", nil)
		ctx.AddParam("request-id", "running-req")

		GetAsyncResultHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)

		var resp map[string]string
		err := json.Unmarshal(rw.Body.Bytes(), &resp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(resp["status"], convey.ShouldEqual, "running")
	})
}

func TestGetAsyncResultHandler_Completed(t *testing.T) {
	convey.Convey("GetAsyncResultHandler should return full result when completed", t, func() {
		gin.SetMode(gin.TestMode)
		store := asyncinvocation.GetAsyncResultStore()
		now := time.Now()
		store.Store("completed-req", &asyncinvocation.AsyncResult{
			RequestID:   "completed-req",
			Status:      asyncinvocation.StatusCompleted,
			StatusCode:  200,
			RespBody:    []byte("result data"),
			RespHeaders: map[string]string{"X-Custom": "value"},
			CreatedAt:   now,
			CompletedAt: &now,
		})
		defer store.Delete("completed-req")

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("GET", "/test", nil)
		ctx.AddParam("request-id", "completed-req")

		GetAsyncResultHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)

		var resp asyncinvocation.AsyncResult
		err := json.Unmarshal(rw.Body.Bytes(), &resp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(resp.RequestID, convey.ShouldEqual, "completed-req")
		convey.So(resp.Status, convey.ShouldEqual, asyncinvocation.StatusCompleted)
		convey.So(resp.StatusCode, convey.ShouldEqual, 200)
	})
}

func TestGetAsyncResultHandler_Failed(t *testing.T) {
	convey.Convey("GetAsyncResultHandler should return full result when failed", t, func() {
		gin.SetMode(gin.TestMode)
		store := asyncinvocation.GetAsyncResultStore()
		now := time.Now()
		store.Store("failed-req", &asyncinvocation.AsyncResult{
			RequestID:   "failed-req",
			Status:      asyncinvocation.StatusFailed,
			StatusCode:  500,
			Error:       "invocation error",
			CreatedAt:   now,
			CompletedAt: &now,
		})
		defer store.Delete("failed-req")

		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		ctx.Request, _ = http.NewRequest("GET", "/test", nil)
		ctx.AddParam("request-id", "failed-req")

		GetAsyncResultHandler(ctx)

		convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)

		var resp asyncinvocation.AsyncResult
		err := json.Unmarshal(rw.Body.Bytes(), &resp)
		convey.So(err, convey.ShouldBeNil)
		convey.So(resp.Status, convey.ShouldEqual, asyncinvocation.StatusFailed)
		convey.So(resp.Error, convey.ShouldEqual, "invocation error")
	})
}
