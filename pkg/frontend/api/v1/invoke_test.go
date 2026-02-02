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
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"
	"github.com/valyala/fasthttp"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/localauth"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/monitor"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/common/faas_common/tls"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/functionmeta"
	"frontend/pkg/frontend/functiontask"
	"frontend/pkg/frontend/instancemanager"
	"frontend/pkg/frontend/invocation"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/schedulerproxy"
	"frontend/pkg/frontend/stream"
	"frontend/pkg/frontend/tenanttrafficlimit"
	"frontend/pkg/frontend/types"
	"frontend/pkg/frontend/upgradecompatible"
)

func constructFakeInvokeRequest(funcName, reqBody string, rw http.ResponseWriter) *gin.Context {
	ctx, _ := gin.CreateTestContext(rw)
	bodyMarshal, _ := json.Marshal(reqBody)
	ctx.Request, _ = http.NewRequest("POST", "/test", bytes.NewBuffer(bodyMarshal))
	ctx.AddParam("function-urn", funcName)
	return ctx
}

type fakeClient struct{}

func (f *fakeClient) AcquireInstance(functionKey string, req commontype.AcquireOption) (*commontype.InstanceAllocationInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (f *fakeClient) ReleaseInstance(allocation *commontype.InstanceAllocationInfo, abnormal bool) {
	// TODO implement me
	panic("implement me")
}

func (f *fakeClient) Invoke(req util.InvokeRequest) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (f *fakeClient) CreateInstanceRaw(createReq []byte) ([]byte, error) {
	return nil, nil
}

func (f *fakeClient) InvokeInstanceRaw(invokeReq []byte) ([]byte, error) {
	return nil, nil
}

func (f *fakeClient) KillRaw(killReq []byte) ([]byte, error) {
	return nil, nil
}

func (c *fakeClient) CreateInstanceByLibRt(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (instanceID string, err error) {
	InstanceID := ""
	return InstanceID, nil
}

func (c *fakeClient) KillByLibRt(instanceID string, signal int, payload []byte) error {
	return nil
}

// InvokeByName copy from faasinvoker_test.go
func (f *fakeClient) InvokeByName(request util.InvokeRequest) ([]byte, error) {
	req := &types.CallReq{
		Header: map[string]string{},
	}
	json.Unmarshal(request.Args[1].Data, req)

	resp := &types.CallResp{
		InnerCode: strconv.Itoa(statuscode.InnerResponseSuccessCode),
		Body:      req.Body,
	}
	return json.Marshal(resp)
}

func (f *fakeClient) IsHealth() bool {
	return true
}

func (f *fakeClient) IsDsHealth() bool {
	return true
}

type fakeFailedClient struct{}

func (c *fakeFailedClient) AcquireInstance(functionKey string, req commontype.AcquireOption) (*commontype.InstanceAllocationInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (c *fakeFailedClient) ReleaseInstance(allocation *commontype.InstanceAllocationInfo, abnormal bool) {
	// TODO implement me
	panic("implement me")
}

func (c *fakeFailedClient) Invoke(req util.InvokeRequest) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (c *fakeFailedClient) IsLibruntime() bool {
	return false
}

func (c *fakeFailedClient) CreateInstanceRaw(createReq []byte) ([]byte, error) {
	return nil, nil
}

func (c *fakeFailedClient) InvokeInstanceRaw(invokeReq []byte) ([]byte, error) {
	return nil, nil
}

func (c *fakeFailedClient) KillRaw(killReq []byte) ([]byte, error) {
	return nil, nil
}

func (f *fakeFailedClient) IsHealth() bool {
	return false
}

func (f *fakeFailedClient) IsDsHealth() bool {
	return true
}

// Invoke -
func (c *fakeFailedClient) InvokeByName(request util.InvokeRequest) ([]byte, error) {
	req := &types.CallReq{
		Header: map[string]string{},
	}
	json.Unmarshal(request.Args[1].Data, req)

	resp := &types.CallResp{
		InnerCode: strconv.Itoa(statuscode.InternalErrorCode),
		Body:      json.RawMessage("\"runtime initialization timed out after 3s\""),
	}
	res, _ := json.Marshal(resp)
	return res, errors.New("runtime initialization timed out after 3s")
}

func (c *fakeFailedClient) CreateInstance(req *functionsystem.CreateRequest) (*functionsystem.CreateResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (c *fakeFailedClient) InvokeInstance(req *functionsystem.InvokeRequest) (*functionsystem.NotifyRequest, error) {
	// TODO implement me
	panic("implement me")
}

func (c *fakeFailedClient) Kill(req *functionsystem.KillRequest) (*functionsystem.KillResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (c *fakeFailedClient) CreateInstanceByLibRt(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (instanceID string, err error) {
	InstanceID := ""
	return InstanceID, nil
}

func (c *fakeFailedClient) KillByLibRt(instanceID string, signal int, payload []byte) error {
	return nil
}

func fakeCaaSInvokeHandler(ctx *types.InvokeProcessContext) error {
	return nil
}

func Test_InvokeHandler(t *testing.T) {
	patches := []*gomonkey.Patches{
		gomonkey.ApplyFunc(util.NewClient, func() util.Client {
			return &fakeClient{}
		}),
		gomonkey.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commontype.FuncSpec, bool) {
			return &commontype.FuncSpec{FunctionKey: funcKey, FuncMetaData: commontype.FuncMetaData{Timeout: 10}}, true
		}),
		// new mock
		gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
			return &types.Config{
				HTTPConfig: &types.FrontendHTTP{MaxRequestBodySize: 1},
				MemoryEvaluatorConfig: &types.MemoryEvaluatorConfig{
					RequestMemoryEvaluator: 2,
				},
				DefaultTenantLimitQuota: 1800,
			}
		}),
		gomonkey.ApplyFunc(upgradecompatible.GetAccessFaaSSchedulerType, func() string {
			return "libruntime"
		}),
		gomonkey.ApplyMethod(reflect.TypeOf(instancemanager.GetFaaSSchedulerInstanceManager()), "IsExist", func(_ *instancemanager.FaaSSchedulerInstanceManager) bool {
			return true
		}),
		gomonkey.ApplyMethodFunc(leaseadaptor.GetInstanceManager(), "AcquireInstance", func(ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec,
			logger api.FormatLogger,
		) (*commontype.InstanceAllocationInfo, snerror.SNError) {
			return &commontype.InstanceAllocationInfo{}, nil
		}),
	}
	defer func() {
		for _, patch := range patches {
			time.Sleep(100 * time.Millisecond)
			patch.Reset()
		}
	}()
	fgAdapter := &invocation.FGAdapter{}
	responsehandler.Handler = fgAdapter.MakeResponseHandler()
	middleware.Invoker = fgAdapter.MakeInvoker()
	urnutils.SetSeparator(urnutils.TenantProductSplitStr)
	stopCh := make(chan struct{})
	_ = monitor.InitMemMonitor(stopCh)
	funcNameDemo := "functions/sn:cn:yrk:xxxxxxxxxxx:function:0@base@testpythonbase001:latest"
	reqBody := "test body"
	schedulerInfo := &schedulerproxy.SchedulerNodeInfo{
		InstanceInfo: &commontype.InstanceInfo{
			InstanceName: "instance1",
			InstanceID:   "instance1",
			Address:      "127.0.0.1",
		},
		UpdateTime: time.Now(),
	}
	schedulerproxy.Proxy.Add(schedulerInfo, log.GetLogger())

	convey.Convey("stream not enable", t, func() {
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest("", reqBody, rw)
		defer gomonkey.ApplyFunc(stream.IsHTTPUploadStream, func(r interface{}) bool {
			return true
		}).Reset()
		InvokeHandler(ctx)
		t.Logf("test stream not enable, rsp: %s", rw.Body.String())
		convey.So(rw.Body.String(), convey.ShouldContainSubstring, "internal system error")
		convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
	})

	testFgStreamException(t, funcNameDemo, reqBody)

	convey.Convey("big body", t, func() {
		rw := httptest.NewRecorder()
		reqBigBody := strings.Repeat("a", 6*1024*1024)
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBigBody, rw)
		InvokeHandler(ctx)
		t.Logf("req body len: %d\n", rw.Body.Len())
		convey.So(rw.Body.String(), convey.ShouldContainSubstring, "the size of request body is beyond")
		convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
	})

	convey.Convey("failed to set processCtx req", t, func() {
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest("", reqBody, rw)
		ctx.Params = make(gin.Params, 0, 0)
		InvokeHandler(ctx)
		convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
	})

	convey.Convey("tenant traffic limit", t, func() {
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		defer gomonkey.ApplyFunc(tenanttrafficlimit.Limit, func(tenantID string) error {
			return errors.New("traffic limit")
		}).Reset()
		InvokeHandler(ctx)
		convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
	})
	convey.Convey("invoke success", t, func() {
		defer gomonkey.ApplyFunc(util.NewClient, func() util.Client {
			return &fakeClient{}
		}).Reset()
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		InvokeHandler(ctx)
		t.Logf("body %s\n", rw.Body.String())
		convey.So(rw.Body.String(), convey.ShouldEqual, "\"test body\"")
		convey.So(rw.Code, convey.ShouldEqual, 200)
	})
	convey.Convey("invoke failed", t, func() {
		defer gomonkey.ApplyFunc(util.NewClient, func() util.Client {
			return &fakeFailedClient{}
		}).Reset()
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		InvokeHandler(ctx)
		t.Logf("body %s\n", rw.Body.String())
		convey.So(rw.Body.String(), convey.ShouldContainSubstring, "runtime initialization timed out after 3s")
		convey.So(rw.Code, convey.ShouldEqual, 500)
	})
	convey.Convey("invoke for fg success", t, func() {
		resp := &commontype.InstanceResponse{
			InstanceAllocationInfo: commontype.InstanceAllocationInfo{
				FuncKey:    "xxxxxxxxxxx/0@base@testpythonbase001/latest",
				ThreadID:   "lease1-1",
				InstanceID: "lease1", LeaseInterval: 100000,
			},
			ErrorCode:     constant.InsReqSuccessCode,
			ErrorMessage:  "",
			SchedulerTime: 0,
		}
		body, _ := json.Marshal(resp)
		c := &fasthttp.Client{}
		defer gomonkey.ApplyMethod(reflect.TypeOf(c),
			"DoTimeout", func(c *fasthttp.Client, req *fasthttp.Request,
				resp *fasthttp.Response, timeout time.Duration,
			) error {
				resp.Header.Set(constant.HeaderInnerCode, "0")
				resp.Header.Set(constant.HeaderWorkerCost, "20")
				resp.Header.Set(constant.HeaderCallNode, "node1")
				resp.Header.Set(constant.HeaderCallInstance, "instance1")
				resp.SetBody(body)
				resp.SetStatusCode(200)
				return nil
			}).Reset()
		defer gomonkey.ApplyMethod(reflect.TypeOf(functiontask.GetBusProxies()), "IsBusProxyHealthy",
			func(_ *functiontask.BusProxies, _ string, _ string) bool {
				return true
			}).Reset()
		defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
			return &types.Config{
				FunctionInvokeBackend: constant.BackendTypeKernel,
				MemoryEvaluatorConfig: &types.MemoryEvaluatorConfig{
					RequestMemoryEvaluator: 2,
				},
				DefaultTenantLimitQuota: 1800,
				HTTPConfig: &types.FrontendHTTP{
					WorkerInstanceReadTimeOut: 60,
					MaxRequestBodySize:        1,
				},
				HTTPSConfig:     &tls.InternalHTTPSConfig{},
				E2EMaxDelayTime: 60,
				LocalAuth: &localauth.AuthConfig{
					AKey:     "ak",
					SKey:     "sk",
					Duration: 5,
				},
				InvokeMaxRetryTimes: 3,
				RetryConfig:         &types.RetryConfig{},
			}
		}).Reset()
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		InvokeHandler(ctx)
		convey.So(rw.Code, convey.ShouldEqual, 200)
		convey.So(rw.Header().Get(constant.HeaderCallNode), convey.ShouldEqual, "node1")
		convey.So(rw.Header().Get(constant.HeaderCallInstance), convey.ShouldEqual, "instance1")
		time.Sleep(150 * time.Millisecond)
	})
	convey.Convey("invoke for fg failed", t, func() {
		resp := &commontype.InstanceResponse{
			InstanceAllocationInfo: commontype.InstanceAllocationInfo{
				FuncKey:    "xxxxxxxxxxx/0@base@testpythonbase001/latest",
				ThreadID:   "lease1-1",
				InstanceID: "lease1", LeaseInterval: 100000,
			},
			ErrorCode:     constant.InsReqSuccessCode,
			ErrorMessage:  "",
			SchedulerTime: 0,
		}
		body, _ := json.Marshal(resp)
		defer gomonkey.ApplyMethod(reflect.TypeOf(functiontask.GetBusProxies()), "IsBusProxyHealthy",
			func(_ *functiontask.BusProxies, _ string, _ string) bool {
				return true
			}).Reset()
		c := &fasthttp.Client{}
		defer gomonkey.ApplyMethod(reflect.TypeOf(c),
			"DoTimeout", func(c *fasthttp.Client, req *fasthttp.Request,
				resp *fasthttp.Response, timeout time.Duration,
			) error {
				resp.Header.Set(constant.HeaderInnerCode, "200500")
				resp.Header.Set(constant.HeaderWorkerCost, "20")
				resp.Header.Set(constant.HeaderCallNode, "node1")
				resp.Header.Set(constant.HeaderCallInstance, "instance1")
				resp.SetBody(body)
				resp.SetStatusCode(200)
				return nil
			}).Reset()
		defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
			return &types.Config{
				FunctionInvokeBackend: constant.BackendTypeKernel,
				MemoryEvaluatorConfig: &types.MemoryEvaluatorConfig{
					RequestMemoryEvaluator: 2,
				},
				DefaultTenantLimitQuota: 1800,
				HTTPConfig: &types.FrontendHTTP{
					WorkerInstanceReadTimeOut: 60,
					MaxRequestBodySize:        1,
				},
				HTTPSConfig:     &tls.InternalHTTPSConfig{},
				E2EMaxDelayTime: 60,
				LocalAuth: &localauth.AuthConfig{
					AKey:     "ak",
					SKey:     "sk",
					Duration: 5,
				},
				InvokeMaxRetryTimes: 2,
				RetryConfig: &types.RetryConfig{
					InstanceExceptionRetry: true,
				},
			}
		}).Reset()
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		InvokeHandler(ctx)
		convey.So(rw.Code, convey.ShouldEqual, 200)
		convey.So(rw.Header().Get(constant.HeaderInnerCode), convey.ShouldEqual, "200500")
		time.Sleep(150 * time.Millisecond)
	})
	convey.Convey("grace exit", t, func() {
		middleware.GraceExit()
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		InvokeHandler(ctx)
		t.Logf("body: %s\n", rw.Body.String())
		convey.So(rw.Body.String(), convey.ShouldEqual, "frontend exiting")
		convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
	})
}

func TestExtractFunctionKey(t *testing.T) {
	functionURN := "sn:cn:yrk:12345678901234561234567890123456:function:0@yrservice@test-faas-python-runtime-001"
	ctx := &gin.Context{}
	ctx.AddParam("function-urn", functionURN)
	type args struct {
		ctx *gin.Context
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "TestExtractFunctionKey",
			args: args{
				ctx: ctx,
			},
			want:    "12345678901234561234567890123456/0@yrservice@test-faas-python-runtime-001/",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcUrn, _, err := extractFunctionURN(ctx, make(map[string]string))
			funcKey := urnutils.CombineFunctionKey(funcUrn.TenantID, funcUrn.FuncName, funcUrn.FuncVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractFunctionKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if funcKey != tt.want {
				t.Errorf("ExtractFunctionKey() got = %v, want %v", funcKey, tt.want)
			}
		})
	}
}

func testFgStreamException(t *testing.T, funcNameDemo string, reqBody string) {
	convey.Convey("invoke for fg stream upload exception", t, func() {
		defer gomonkey.ApplyFunc(config.GetConfig, mockFgStreamReqConfig()).Reset()
		defer gomonkey.ApplyFunc(stream.HTTPStreamInvokeHandler,
			func(ctx interface{}, timeout interface{}) error {
				return errors.New("mocked error")
			}).Reset()
		defer gomonkey.ApplyFunc(stream.IsHTTPUploadStream, func(r interface{}) bool {
			return true
		}).Reset()
		rw := httptest.NewRecorder()
		ctx := constructFakeInvokeRequest(funcNameDemo, reqBody, rw)
		ctx.Request.Header.Set(constant.HeaderContentType, httpconstant.MultipartFormContentType)
		ctx.Request.Header.Set(constant.HeaderContentLength, "1")
		InvokeHandler(ctx)
		t.Logf("body %s\n", rw.Body.String())
		convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
		convey.So(rw.Body.String(), convey.ShouldContainSubstring, "mocked error")
	})
}

func mockFgStreamReqConfig() func() *types.Config {
	return func() *types.Config {
		return &types.Config{
			FunctionInvokeBackend: constant.BackendTypeKernel,
			MemoryEvaluatorConfig: &types.MemoryEvaluatorConfig{
				RequestMemoryEvaluator: 2,
			},
			DefaultTenantLimitQuota: 1800,
			HTTPConfig: &types.FrontendHTTP{
				WorkerInstanceReadTimeOut: 60,
				MaxRequestBodySize:        1,
				MaxStreamRequestBodySize:  1,
			},
			HTTPSConfig:     &tls.InternalHTTPSConfig{},
			E2EMaxDelayTime: 60,
			LocalAuth: &localauth.AuthConfig{
				AKey:     "ak",
				SKey:     "sk",
				Duration: 5,
			},
			InvokeMaxRetryTimes: 3,
			RetryConfig:         &types.RetryConfig{},
			StreamEnable:        true,
		}
	}
}

func TestGinWriter(t *testing.T) {
	convey.Convey("Test GinWriter", t, func() {
		// Create a new gin.Engine with test mode
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		gw := &GinWriter{
			Context: ctx,
		}

		convey.Convey("When calling SSEWrite with data", func() {
			data := []byte("test data")
			_, err := gw.SSEWrite(data)
			convey.So(err, convey.ShouldEqual, nil)
			convey.So(ctx.Writer.Header().Get("Content-Type"), convey.ShouldEqual, "text/event-stream")
			convey.So(ctx.Writer.Header().Get("Cache-Control"), convey.ShouldEqual, "no-cache")
			convey.So(ctx.Writer.Header().Get("Connection"), convey.ShouldEqual, "keep-alive")
			convey.So(ctx.Writer.Header().Get("Transfer-Encoding"), convey.ShouldEqual, "chunked")
			convey.So(ctx.Writer.Header().Get("X-Accel-Buffering"), convey.ShouldEqual, "no")
			convey.So(rw.Body.String(), convey.ShouldContainSubstring, "data: test data\n\n")
		})
	})
}

type mockResponseWriter struct {
	clientDisconnectChan <-chan struct{}
	sseWriteFunc         func([]byte) (int, error)
}

func (m *mockResponseWriter) ClientDisconnectChan() <-chan struct{} {
	return m.clientDisconnectChan
}

func (m *mockResponseWriter) SSEWrite(data []byte) (int, error) {
	return m.sseWriteFunc(data)
}

func TestWriteHTTPResponse(t *testing.T) {
	convey.Convey("Test WriteHTTPResponse", t, func() {
		// 初始化测试用的上下文和过程上下文
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rw)
		processCtx := &types.InvokeProcessContext{
			StatusCode: http.StatusOK,
			RespHeader: map[string]string{},
			RespBody:   []byte("test response"),
			ReqHeader:  map[string]string{},
		}
		defer gomonkey.ApplyFunc(writeHeadersToResponse, func(headerMap map[string]string, header http.Header) {
			return
		}).Reset()
		convey.Convey("When writing a normal HTTP response", func() {
			processCtx.ResponseWriter = &GinWriter{Context: ctx}
			writeHTTPResponse(ctx, processCtx)
			convey.So(ctx.Writer.Status(), convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "test response")
		})

		convey.Convey("When writing an SSE response", func() {
			processCtx.ReqHeader["Accept"] = "text/event-stream"
			processCtx.ResponseWriter = &mockResponseWriter{
				clientDisconnectChan: make(chan struct{}),
				sseWriteFunc: func(data []byte) (int, error) {
					return len(data), nil
				},
			}
			writeHTTPResponse(ctx, processCtx)
			convey.So(ctx.Writer.Status(), convey.ShouldEqual, http.StatusOK)
		})

		convey.Convey("When writing an SSE response with an error", func() {
			processCtx.ReqHeader["Accept"] = "text/event-stream"
			processCtx.ResponseWriter = &mockResponseWriter{
				clientDisconnectChan: make(chan struct{}),
				sseWriteFunc: func(data []byte) (int, error) {
					return 0, errors.New("SSE write error")
				},
			}
			writeHTTPResponse(ctx, processCtx)
			convey.So(ctx.Writer.Status(), convey.ShouldEqual, http.StatusOK)
		})
	})
}
