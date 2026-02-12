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

package middleware

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/types"
)

// mockResponseHandlerForBodySize implements responsehandler.HandlerInterface for testing
type mockResponseHandlerForBodySize struct{}

func (m *mockResponseHandlerForBodySize) SetResponseFromFrontend(ctx *types.InvokeProcessContext, innerCode int, message interface{}) {
	// Do nothing in test
}

func (m *mockResponseHandlerForBodySize) SetResponseFromInvocation(ctx *types.InvokeProcessContext,
	message []byte) (*types.CallResp, snerror.SNError) {
	return nil, nil
}

func TestBodySizeChecker(t *testing.T) {
	// Set up mock response handler
	originalHandler := responsehandler.Handler
	responsehandler.Handler = &mockResponseHandlerForBodySize{}
	defer func() {
		responsehandler.Handler = originalHandler
	}()

	tests := []struct {
		name                     string
		ctx                      *types.InvokeProcessContext
		maxRequestBodySize       int
		maxStreamRequestBodySize int
		shouldFail               bool
	}{
		{
			name: "not exceeds MaxRequestBodySize",
			ctx: &types.InvokeProcessContext{
				ReqBody: make([]byte, 1*megabytes),
			},
			maxRequestBodySize: 1,
			shouldFail:         false,
		},
		{
			name: "exceeds MaxRequestBodySize",
			ctx: &types.InvokeProcessContext{
				ReqBody: make([]byte, 1*megabytes+1),
			},
			maxRequestBodySize: 1,
			shouldFail:         true,
		},
		{
			name: "not exceeds MaxStreamRequestBodySize",
			ctx: &types.InvokeProcessContext{
				ReqBody:            []byte("test body"),
				ReqHeader:          map[string]string{constant.HeaderContentLength: "1048576"},
				IsHTTPUploadStream: true,
			},
			maxStreamRequestBodySize: 1,
			shouldFail:               false,
		},
		{
			name: "exceeds MaxStreamRequestBodySize",
			ctx: &types.InvokeProcessContext{
				ReqBody:            []byte("test body"),
				ReqHeader:          map[string]string{constant.HeaderContentLength: "1048577"},
				IsHTTPUploadStream: true,
			},
			maxStreamRequestBodySize: 1,
			shouldFail:               true,
		},
		{
			name: "exceeds default 1GB MaxStreamRequestBodySize",
			ctx: &types.InvokeProcessContext{
				ReqBody:            []byte("test body"),
				ReqHeader:          map[string]string{constant.HeaderContentLength: "1073741824"},
				IsHTTPUploadStream: true,
			},
			maxStreamRequestBodySize: 1024,
			shouldFail:               false,
		},
		{
			name: "Content-Length header not found",
			ctx: &types.InvokeProcessContext{
				ReqBody:            []byte("test body"),
				IsHTTPUploadStream: true,
			},
			shouldFail: true,
		},
		{
			name: "Content-Length is invalid",
			ctx: &types.InvokeProcessContext{
				ReqBody:            []byte("test body"),
				ReqHeader:          map[string]string{constant.HeaderContentLength: "-1"},
				IsHTTPUploadStream: true,
			},
			shouldFail: true,
		},
	}

	convey.Convey("Test BodySizeChecker", t, func() {
		for _, tt := range tests {
			convey.Convey(tt.name, func() {
				conf := types.Config{
					HTTPConfig: &types.FrontendHTTP{
						MaxRequestBodySize:       tt.maxRequestBodySize,
						MaxStreamRequestBodySize: tt.maxStreamRequestBodySize,
					}}
				config.SetConfig(conf)
				nextHandler := func(ctx *types.InvokeProcessContext) error { return nil }
				checker := BodySizeChecker(nextHandler)
				err := checker(tt.ctx)
				if tt.shouldFail {
					convey.So(err, convey.ShouldNotBeNil)
					if err == nil {
						t.Errorf("Expected error but got none for test: %s", tt.name)
					}
				} else {
					convey.So(err, convey.ShouldBeNil)
					if err != nil {
						t.Errorf("Did not expect error but got: %v for test: %s", err, tt.name)
					}
				}
			})
		}
	})
}
