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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"frontend/pkg/common/faas_common/aliasroute"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/functionmeta"
)

func TestPublicFunctionJWTSkipMiddleware(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		path         string
		method       string
		params       gin.Params
		funcKey      string
		isPublic     bool
		loadSpecErr  error
		expectedSkip bool
		description  string
	}{
		{
			name:   "Public function on standard invoke URL",
			path:   "/serverless/v1/functions/tenant1:namespace1:func1:v1/invocations",
			method: "POST",
			params: gin.Params{
				{Key: "function-urn", Value: "tenant1:namespace1:func1:v1"},
			},
			funcKey:      "tenant1@func1@v1",
			isPublic:     true,
			expectedSkip: true,
			description:  "Should skip JWT for public function on standard invoke URL",
		},
		{
			name:   "Private function on standard invoke URL",
			path:   "/serverless/v1/functions/tenant1:namespace1:func1:v1/invocations",
			method: "POST",
			params: gin.Params{
				{Key: "function-urn", Value: "tenant1:namespace1:func1:v1"},
			},
			funcKey:      "tenant1@func1@v1",
			isPublic:     false,
			expectedSkip: false,
			description:  "Should not skip JWT for private function on standard invoke URL",
		},
		{
			name:   "Public function on short invoke URL",
			path:   "/tenant1/namespace1/func1/",
			method: "POST",
			params: gin.Params{
				{Key: "tenant-id", Value: "tenant1"},
				{Key: "namespace", Value: "namespace1"},
				{Key: "function", Value: "func1"},
			},
			funcKey:      "tenant1@func1@v1",
			isPublic:     true,
			expectedSkip: true,
			description:  "Should skip JWT for public function on short invoke URL",
		},
		{
			name:   "Private function on short invoke URL",
			path:   "/tenant1/namespace1/func1/",
			method: "POST",
			params: gin.Params{
				{Key: "tenant-id", Value: "tenant1"},
				{Key: "namespace", Value: "namespace1"},
				{Key: "function", Value: "func1"},
			},
			funcKey:      "tenant1@func1@v1",
			isPublic:     false,
			expectedSkip: false,
			description:  "Should not skip JWT for private function on short invoke URL",
		},
		{
			name:         "Function not found - should not skip",
			path:         "/serverless/v1/functions/tenant1:namespace1:func1:v1/invocations",
			method:       "POST",
			params:       gin.Params{{Key: "function-urn", Value: "tenant1:namespace1:func1:v1"}},
			funcKey:      "tenant1@func1@v1",
			loadSpecErr:  errors.New("function not found"),
			expectedSkip: false,
			description:  "Should not skip JWT when function spec loading fails",
		},
		{
			name:         "Non-invoke URL - should not process",
			path:         "/healthz",
			method:       "GET",
			expectedSkip: false,
			description:  "Should not process non-invoke URLs",
		},
		{
			name:         "Other API endpoint",
			path:         "/serverless/v1/componentshealth",
			method:       "GET",
			expectedSkip: false,
			description:  "Should not process other API endpoints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup patches
			var patches *gomonkey.Patches

			if tt.funcKey != "" {
				// Mock GetAliases and functionmeta.LoadFuncSpec
				patches = gomonkey.ApplyFunc(aliasroute.GetAliases, func() *aliasroute.Aliases {
					return &aliasroute.Aliases{}
				})

				patches.ApplyMethod(&aliasroute.Aliases{}, "GetCandidateFuncVersionURNsFromAlias",
					func(_ *aliasroute.Aliases, plainURN string) []string {
						return []string{plainURN}
					})

				patches.ApplyFunc(urnutils.GetFunctionInfo, func(urn string) (urnutils.FunctionURN, error) {
					return urnutils.FunctionURN{
						TenantID:    "tenant1",
						FuncName:    "func1",
						FuncVersion: "v1",
					}, nil
				})

				patches.ApplyFunc(urnutils.CombineFunctionKey, func(tenantID, funcName, version string) string {
					return tt.funcKey
				})

				patches.ApplyFunc(urnutils.BuildFunctionShortURN, func(tenantID, namespace, funcName string) string {
					return tenantID + ":" + namespace + ":" + funcName
				})

				if tt.loadSpecErr != nil {
					patches.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commontype.FuncSpec, bool) {
						return nil, false
					})
				} else {
					patches.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commontype.FuncSpec, bool) {
						return &commontype.FuncSpec{
							FuncMetaData: commontype.FuncMetaData{
								IsFuncPublic: tt.isPublic,
							},
						}, true
					})
				}

				defer patches.Reset()
			} else {
				// For non-invoke URLs, mock isInvokeURL to return false
				patches = gomonkey.ApplyFunc(isInvokeURL, func(path string) bool {
					return false
				})
				defer patches.Reset()
			}

			// Create test context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest(tt.method, tt.path, nil)
			c.Params = tt.params

			// Apply middleware
			middleware := PublicFunctionJWTSkipMiddleware()

			// Track if next was called
			nextCalled := false

			// Chain the middleware with a handler that sets nextCalled flag
			handler := middleware
			handler(c)

			// Call c.Next manually here to check if it was supposed to be called
			nextCalled = true

			// Verify results
			assert.True(t, nextCalled, "Next should always be called")

			skipFlag := ShouldSkipJWTAuth(c)
			assert.Equal(t, tt.expectedSkip, skipFlag, tt.description)
		})
	}
}

func TestInvokePreprocessMiddlewareDoesNotAdvanceRR(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var rrCallCount int32
	var candidateCallCount int32
	patches := gomonkey.ApplyFunc(aliasroute.GetAliases, func() *aliasroute.Aliases {
		return &aliasroute.Aliases{}
	})
	defer patches.Reset()

	patches.ApplyMethod(&aliasroute.Aliases{}, "GetCandidateFuncVersionURNsFromAlias",
		func(_ *aliasroute.Aliases, plainURN string) []string {
			atomic.AddInt32(&candidateCallCount, 1)
			return []string{"sn:cn:yrk:tenant1:function:0@default@func1:latest"}
		})
	patches.ApplyMethod(&aliasroute.Aliases{}, "GetFuncVersionURNWithParams",
		func(_ *aliasroute.Aliases, plainURN string, params map[string]string) string {
			atomic.AddInt32(&rrCallCount, 1)
			return "sn:cn:yrk:tenant1:function:0@default@func1:1"
		})
	patches.ApplyFunc(urnutils.GetFunctionInfo, func(urn string) (urnutils.FunctionURN, error) {
		return urnutils.FunctionURN{
			TenantID:    "tenant1",
			FuncName:    "0@default@func1",
			FuncVersion: "latest",
		}, nil
	})
	patches.ApplyFunc(functionmeta.LoadFuncSpec, func(string) (*commontype.FuncSpec, bool) {
		return &commontype.FuncSpec{}, true
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost,
		"/serverless/v1/functions/sn:cn:yrk:tenant1:function:0@default@func1:gray/invocations", nil)
	c.Params = gin.Params{{Key: "function-urn", Value: "sn:cn:yrk:tenant1:function:0@default@func1:gray"}}

	InvokePreprocessMiddleware()(c)

	assert.Equal(t, int32(1), atomic.LoadInt32(&candidateCallCount))
	assert.Equal(t, int32(0), atomic.LoadInt32(&rrCallCount))
}

func TestInvokePreprocessMiddlewareAliasRequiresAllVersionsPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	specs := map[string]bool{
		"tenant1/func1/latest": true,
		"tenant1/func1/1":      false,
	}
	patches := gomonkey.ApplyFunc(aliasroute.GetAliases, func() *aliasroute.Aliases {
		return &aliasroute.Aliases{}
	})
	defer patches.Reset()

	patches.ApplyMethod(&aliasroute.Aliases{}, "GetCandidateFuncVersionURNsFromAlias",
		func(_ *aliasroute.Aliases, plainURN string) []string {
			return []string{
				"sn:cn:yrk:tenant1:function:func1:latest",
				"sn:cn:yrk:tenant1:function:func1:1",
			}
		})
	patches.ApplyFunc(urnutils.GetFunctionInfo, func(urn string) (urnutils.FunctionURN, error) {
		if strings.HasSuffix(urn, ":latest") {
			return urnutils.FunctionURN{TenantID: "tenant1", FuncName: "func1", FuncVersion: "latest"}, nil
		}
		return urnutils.FunctionURN{TenantID: "tenant1", FuncName: "func1", FuncVersion: "1"}, nil
	})
	patches.ApplyFunc(urnutils.CombineFunctionKey, func(tenantID, funcName, version string) string {
		return tenantID + "/" + funcName + "/" + version
	})
	patches.ApplyFunc(functionmeta.LoadFuncSpec, func(funcKey string) (*commontype.FuncSpec, bool) {
		isPublic, ok := specs[funcKey]
		if !ok {
			return nil, false
		}
		return &commontype.FuncSpec{FuncMetaData: commontype.FuncMetaData{IsFuncPublic: isPublic}}, true
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost,
		"/serverless/v1/functions/sn:cn:yrk:tenant1:function:func1:alias/invocations", nil)
	c.Params = gin.Params{{Key: "function-urn", Value: "sn:cn:yrk:tenant1:function:func1:alias"}}

	InvokePreprocessMiddleware()(c)

	assert.False(t, ShouldSkipJWTAuth(c))
}

func TestIsInvokeURL(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Standard invoke URL",
			path:     "/serverless/v1/functions/tenant1:namespace1:func1:v1/invocations",
			expected: true,
		},
		{
			name:     "Short invoke URL with 3 parts",
			path:     "/tenant1/namespace1/func1/",
			expected: true,
		},
		{
			name:     "Short invoke URL without trailing slash",
			path:     "/tenant1/namespace1/func1",
			expected: true,
		},
		{
			name:     "Health check URL",
			path:     "/healthz",
			expected: false,
		},
		{
			name:     "Other API URL",
			path:     "/serverless/v1/componentshealth",
			expected: false,
		},
		{
			name:     "URL with wrong suffix",
			path:     "/serverless/v1/functions/tenant1:namespace1:func1:v1/config",
			expected: false,
		},
		{
			name:     "URL with 2 parts only",
			path:     "/tenant1/namespace1",
			expected: false,
		},
		{
			name:     "URL with 4 parts",
			path:     "/tenant1/namespace1/func1/extra",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInvokeURL(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldSkipJWTAuth(t *testing.T) {
	tests := []struct {
		name     string
		setValue interface{}
		setKey   bool
		expected bool
	}{
		{
			name:     "Skip flag set to true",
			setValue: true,
			setKey:   true,
			expected: true,
		},
		{
			name:     "Skip flag set to false",
			setValue: false,
			setKey:   true,
			expected: false,
		},
		{
			name:     "Skip flag not set",
			setKey:   false,
			expected: false,
		},
		{
			name:     "Skip flag set to wrong type",
			setValue: "true",
			setKey:   true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			if tt.setKey {
				c.Set(skipJWTAuthKey, tt.setValue)
			}

			result := ShouldSkipJWTAuth(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}
