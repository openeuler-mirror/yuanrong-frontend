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
	"reflect"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/common/faas_common/tls"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/functiontask"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/schedulerproxy"
	"frontend/pkg/frontend/stream"
	"frontend/pkg/frontend/types"
	"yuanrong.org/kernel/runtime/libruntime/api"
)

func TestProcessResp(t *testing.T) {
	type testCase struct {
		name          string
		ctx           *types.InvokeProcessContext
		resp          *fasthttp.Response
		setupMocks    func() *gomonkey.Patches
		expectedError bool
	}
	responsehandler.Handler = (&FGAdapter{}).MakeResponseHandler()
	cases := []testCase{
		{
			name: "HTTP upload stream with error code",
			ctx: &types.InvokeProcessContext{
				TraceID:            "trace-2",
				IsHTTPUploadStream: true,
				StreamInfo: &types.StreamInvokeInfo{
					RequestStreamErrorCode: 1,
				},
				RespHeader:       make(map[string]string),
				RequestTraceInfo: &types.RequestTraceInfo{},
			},
			resp:          &fasthttp.Response{},
			expectedError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processResp(tc.ctx, tc.resp)
		})
	}

	t.Run("response stop channel closed channel", func(t *testing.T) {
		stopChan := &types.StreamStopChan{C: make(chan struct{})}
		close(stopChan.C)

		ctx := &types.InvokeProcessContext{
			TraceID:            "trace-closed-chan",
			IsHTTPUploadStream: false,
			StreamInfo: &types.StreamInvokeInfo{
				RequestStreamErrorCode: 0,
				ResponseStopChan:       stopChan,
				ResponseStreamName:     "test-stream",
			},
			RespHeader: make(map[string]string),
			RequestTraceInfo: &types.RequestTraceInfo{
				Deadline: time.Now().Add(1 * time.Millisecond),
			},
		}
		resp := &fasthttp.Response{}

		patch := gomonkey.ApplyFunc(stream.CheckIsResponseStream, func(streamName string) bool {
			return true
		})
		defer patch.Reset()

		processResp(ctx, resp)

		assert.Equal(t, 0, ctx.RequestTraceInfo.InnerCode, "Expected InnerCode to remain 0")
	})

	t.Run("response stream timeout", func(t *testing.T) {
		stopChan := &types.StreamStopChan{C: make(chan struct{})}

		ctx := &types.InvokeProcessContext{
			TraceID:            "trace-timeout",
			IsHTTPUploadStream: false,
			StreamInfo: &types.StreamInvokeInfo{
				RequestStreamErrorCode: 0,
				ResponseStopChan:       stopChan,
				ResponseStreamName:     "test-stream",
			},
			RespHeader: make(map[string]string),
			RequestTraceInfo: &types.RequestTraceInfo{
				Deadline: time.Now().Add(1 * time.Millisecond),
			},
		}
		resp := &fasthttp.Response{}

		patch := gomonkey.ApplyFunc(stream.CheckIsResponseStream, func(streamName string) bool {
			return true
		})
		defer patch.Reset()

		var capturedError error
		patch2 := gomonkey.ApplyFunc(setErrorResponse, func(invokeCtx *types.InvokeProcessContext, err error) {
			capturedError = err
		})
		defer patch2.Reset()

		processResp(ctx, resp)

		assert.NotNil(t, capturedError, "Expected error to be set on timeout")
		var snerr snerror.SNError
		ok := errors.As(capturedError, &snerr)
		assert.True(t, ok, "Expected SNError type")
		assert.Equal(t, statuscode.InternalErrorCode, snerr.Code(), "Expected internal error code")
		assert.Contains(t, snerr.Error(), "wait for response stream timeout", "Expected timeout error message")
	})
}

func TestAcquireInstance(t *testing.T) {
	var patches *gomonkey.Patches

	type testCase struct {
		name           string
		setupMocks     func()
		expectedResult *commontype.InstanceAllocationInfo
		expectedError  error
	}

	schedulerInfo := &schedulerproxy.SchedulerNodeInfo{
		InstanceInfo: &commontype.InstanceInfo{InstanceName: "instance1"},
		UpdateTime:   time.Now(),
	}
	schedulerproxy.Proxy.Add(schedulerInfo, log.GetLogger())

	cases := []testCase{
		{
			name: "No Instance Available",
			setupMocks: func() {
				patches = gomonkey.ApplyMethod(reflect.TypeOf(leaseadaptor.GetInstanceManager()),
					"AcquireInstance",
					func(im *leaseadaptor.Manager, ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec, logger api.FormatLogger) (*commontype.InstanceAllocationInfo, snerror.SNError) {
						return nil, snerror.New(statuscode.NoInstanceAvailableErrCode, "no instance available")
					})
				patches.ApplyFunc(time.Sleep, func(d time.Duration) {})
				gomonkey.ApplyMethod(reflect.TypeOf(functiontask.GetBusProxies()), "NextWithName",
					func(_ *functiontask.BusProxies,
						FuncKey string, move bool) string {
						return "192.168.1.1"
					})
			},
			expectedResult: &commontype.InstanceAllocationInfo{NodeIP: "192.168.1.1", NodePort: "22423"},
			expectedError:  nil,
		},
		{
			name: "Should retry error",
			setupMocks: func() {
				patches = gomonkey.ApplyMethod(reflect.TypeOf(leaseadaptor.GetInstanceManager()),
					"AcquireInstance",
					func(im *leaseadaptor.Manager, ctx *types.InvokeProcessContext, funcSpec *commontype.FuncSpec, logger api.FormatLogger) (*commontype.InstanceAllocationInfo, snerror.SNError) {
						return nil, snerror.New(statuscode.SendReqErrCode, "send req error")
					})
				patches.ApplyFunc(time.Sleep, func(d time.Duration) {})
				patches.ApplyFunc(selectBusForInstance,
					func(ctx *types.InvokeProcessContext) (*commontype.InstanceAllocationInfo, error) {
						return &commontype.InstanceAllocationInfo{NodeIP: "192.168.1.1"}, nil
					})
			},
			expectedResult: nil,
			expectedError:  snerror.New(statuscode.SendReqErrCode, "send req error"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()
			defer patches.Reset()

			ctx := &types.InvokeProcessContext{
				FuncKey: "test-func",
				TraceID: "test-trace",
				RequestTraceInfo: &types.RequestTraceInfo{
					TryCount: -2,
				},
			}
			funcSpec := &commontype.FuncSpec{
				FuncMetaData: commontype.FuncMetaData{
					BusinessType: constant.BusinessTypeCAE,
				},
			}

			result, err := acquireInstance(ctx, funcSpec)

			assert.Equal(t, tc.expectedResult, result)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

func TestInvokeBus(t *testing.T) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	convey.Convey("test invoke bus error", t, func() {
		convey.Convey("retry max time", func() {
			defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
				return &types.Config{
					InvokeMaxRetryTimes: 1,
				}
			}).Reset()
			ctx := &types.InvokeProcessContext{
				RequestTraceInfo: &types.RequestTraceInfo{
					TryCount: 2,
				},
			}
			_, _, err := invokeBus(ctx, req, resp, &commontype.FuncSpec{})
			convey.So(err, convey.ShouldEqual, ErrServiceNotAvailable)
		})
		convey.Convey("retry error timeout", func() {
			defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
				return &types.Config{
					InvokeMaxRetryTimes: 5,
					RetryConfig: &types.RetryConfig{
						InstanceExceptionRetry: true,
					},
					HTTPSConfig: &tls.InternalHTTPSConfig{},
					HTTPConfig:  &types.FrontendHTTP{},
				}
			}).Reset()
			c := &fasthttp.Client{}
			defer gomonkey.ApplyMethod(reflect.TypeOf(c),
				"DoTimeout", func(c *fasthttp.Client, req *fasthttp.Request,
					resp *fasthttp.Response, timeout time.Duration) error {
					return errors.New(requestTimeout)
				}).Reset()
			ctx := &types.InvokeProcessContext{
				RequestTraceInfo: &types.RequestTraceInfo{
					Deadline: time.Now().Add(5 * time.Second),
				},
			}
			_, _, err := invokeBus(ctx, req, resp, &commontype.FuncSpec{})
			convey.So(err.Error(), convey.ShouldEqual, requestTimeout)
		})
		convey.Convey("retry count", func() {
			defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
				return &types.Config{
					InvokeMaxRetryTimes: 1,
					RetryConfig: &types.RetryConfig{
						InstanceExceptionRetry: true,
					},
					HTTPSConfig: &tls.InternalHTTPSConfig{},
					HTTPConfig:  &types.FrontendHTTP{},
				}
			}).Reset()
			c := &fasthttp.Client{}
			defer gomonkey.ApplyMethod(reflect.TypeOf(c),
				"DoTimeout", func(c *fasthttp.Client, req *fasthttp.Request,
					resp *fasthttp.Response, timeout time.Duration) error {
					return errors.New("request failed")
				}).Reset()
			ctx := &types.InvokeProcessContext{
				RequestTraceInfo: &types.RequestTraceInfo{
					Deadline: time.Now().Add(5 * time.Second),
				},
			}
			_, _, err := invokeBus(ctx, req, resp, &commontype.FuncSpec{})
			convey.So(err.Error(), convey.ShouldEqual, "request failed")
		})
		convey.Convey("should retry", func() {
			defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
				return &types.Config{
					InvokeMaxRetryTimes: 1,
					RetryConfig: &types.RetryConfig{
						InstanceExceptionRetry: true,
					},
					HTTPSConfig: &tls.InternalHTTPSConfig{},
					HTTPConfig:  &types.FrontendHTTP{},
				}
			}).Reset()
			c := &fasthttp.Client{}
			defer gomonkey.ApplyMethod(reflect.TypeOf(c),
				"DoTimeout", func(c *fasthttp.Client, req *fasthttp.Request,
					resp *fasthttp.Response, timeout time.Duration) error {
					resp.Header.Set(constant.HeaderInnerCode, "150461")
					return nil
				}).Reset()
			ctx := &types.InvokeProcessContext{
				RequestTraceInfo: &types.RequestTraceInfo{
					Deadline: time.Now().Add(5 * time.Second),
				},
			}
			_, needTry, err := invokeBus(ctx, req, resp,
				&commontype.FuncSpec{FuncMetaData: commontype.FuncMetaData{}})
			convey.So(needTry, convey.ShouldBeTrue)
			convey.So(err, convey.ShouldBeNil)
		})
		convey.Convey("should retry and delete", func() {
			defer gomonkey.ApplyFunc(config.GetConfig, func() *types.Config {
				return &types.Config{
					InvokeMaxRetryTimes: 1,
					RetryConfig: &types.RetryConfig{
						InstanceExceptionRetry: true,
					},
					HTTPSConfig: &tls.InternalHTTPSConfig{},
					HTTPConfig:  &types.FrontendHTTP{},
				}
			}).Reset()
			c := &fasthttp.Client{}
			defer gomonkey.ApplyMethod(reflect.TypeOf(c),
				"DoTimeout", func(c *fasthttp.Client, req *fasthttp.Request,
					resp *fasthttp.Response, timeout time.Duration) error {
					resp.Header.Set(constant.HeaderInnerCode, "150460")
					return nil
				}).Reset()
			ctx := &types.InvokeProcessContext{
				RequestTraceInfo: &types.RequestTraceInfo{
					Deadline: time.Now().Add(5 * time.Second),
				},
			}
			_, needTry, err := invokeBus(ctx, req, resp,
				&commontype.FuncSpec{FuncMetaData: commontype.FuncMetaData{}})
			convey.So(needTry, convey.ShouldBeTrue)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestSetLubanBody(t *testing.T) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.Header.Set(httpconstant.HeaderLuBanGTraceID, "123")
	req.Header.Set(httpconstant.HeaderLuBanNTraceID, "123")
	req.Header.Set(httpconstant.HeaderLuBanSpanID, "123")
	req.Header.Set(httpconstant.HeaderLuBanEvnID, "123")
	req.Header.Set(httpconstant.HeaderLuBanEventID, "123")
	req.Header.Set(httpconstant.HeaderLuBanDomainID, "123")
	rawData := map[string]string{"aaa": "bbb"}
	data, _ := json.Marshal(rawData)
	req.SetBody(data)
	setLubanBody(req)
	bodyMap := make(map[string]interface{}, defaultBodyMap)
	_ = json.Unmarshal(req.Body(), &bodyMap)
	assert.Equal(t, "123", bodyMap[httpconstant.HeaderLuBanGTraceID])
}

func TestPrepareStreamResponse(t *testing.T) {
	type testCase struct {
		name                       string
		ctx                        *types.InvokeProcessContext
		req                        *fasthttp.Request
		expectedStreamName         string
		expectedFrontendStreamName string
	}

	cases := []testCase{
		{
			name: "RegisterResponse returns true",
			ctx: &types.InvokeProcessContext{
				StreamInfo: &types.StreamInvokeInfo{
					ResponseStreamName: "responseStreamName",
				},
			},
			req: &fasthttp.Request{
				Header: fasthttp.RequestHeader{},
			},
			expectedStreamName:         "responseStreamName",
			expectedFrontendStreamName: stream.GetFrontendResponseStreamName(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patch := gomonkey.ApplyFunc(stream.RegisterResponse, func(ctx interface{}) bool {
				return true
			})
			defer patch.Reset()

			prepareStreamResponse(tc.ctx, tc.req)
			assert.Equal(t, tc.expectedStreamName, string(tc.req.Header.Peek(constant.HeaderResponseStreamName)))
			assert.Equal(t, tc.expectedFrontendStreamName,
				string(tc.req.Header.Peek(constant.HeaderFrontendResponseStreamName)))
		})
	}
}
