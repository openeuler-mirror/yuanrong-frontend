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

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/middleware"
)

func TestInvokeHandlerWithJWTMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock SDK client
	mock := &mockUtils.FakeLibruntimeSdkClient{}
	util.SetAPIClientLibruntime(mock)

	tests := []struct {
		name               string
		enableAuth         bool
		authHeader         string
		iamAddr            string
		requestBody        map[string]interface{}
		expectedStatusCode int
	}{
		{
			name:               "invoke without JWT when auth disabled",
			enableAuth:         false,
			authHeader:         "",
			requestBody:        map[string]interface{}{"test": "data"},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:        "invoke without JWT when auth enabled should be rejected",
			enableAuth:  true,
			authHeader:  "",
			requestBody: map[string]interface{}{"test": "data"},
			// EnableFuncTokenAuth=true makes JWT mandatory on non-whitelisted routes.
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "invoke with invalid JWT",
			enableAuth:         true,
			authHeader:         "invalid.jwt.token",
			requestBody:        map[string]interface{}{"test": "data"},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "invoke with wrong role",
			enableAuth:         true,
			authHeader:         jwtIntegrationTestToken("tenant123", "user"),
			requestBody:        map[string]interface{}{"test": "data"},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:       "invoke with valid JWT but IAM validation failed",
			enableAuth: true,
			authHeader: jwtIntegrationTestToken("tenant123", "developer"),
			// Non-empty unreachable IAM address exercises the real IAM validation failure path.
			iamAddr:            "127.0.0.1:1",
			requestBody:        map[string]interface{}{"test": "data"},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:       "invoke with valid JWT and IAM",
			enableAuth: true,
			authHeader: jwtIntegrationTestToken("tenant123", "developer"),
			// Empty IAM address intentionally exercises ValidateWithIamServer's configured skip path.
			iamAddr:            "",
			requestBody:        map[string]interface{}{"test": "data"},
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			origIAMAddr := config.GetConfig().IamConfig.Addr
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth
			config.GetConfig().IamConfig.Addr = tt.iamAddr
			defer func() {
				config.GetConfig().IamConfig.Addr = origIAMAddr
			}()

			// Create router with middleware (simulating InitRoute behavior)
			router := gin.New()
			router.POST("/test/invoke", middleware.JWTAuthMiddleware(), func(c *gin.Context) {
				// Simulate InvokeHandler behavior
				c.JSON(http.StatusOK, gin.H{"result": "success"})
			})

			// Create request
			bodyBytes, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/test/invoke", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("remoteClientId", "test-client")
			if tt.authHeader != "" {
				req.Header.Set(jwtauth.HeaderXAuth, tt.authHeader)
			}

			// Execute
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify
			assert.Equal(t, tt.expectedStatusCode, w.Code, "status code mismatch for test: "+tt.name)
		})
	}
}

func jwtIntegrationTestToken(sub, role string) string {
	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
			`{"sub":"%s","exp":9876543210,"role":"%s"}`, sub, role))),
		"signature",
	}, ".")
}

func TestMultipleEndpointsWithJWTMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup router
	router := gin.New()

	// Endpoints without JWT middleware (like urlPreCreate)
	router.POST("/create", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "created"})
	})

	// Endpoints with JWT middleware (like urlPreInvoke)
	router.POST("/invoke", middleware.JWTAuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "invoked"})
	})

	tests := []struct {
		name               string
		endpoint           string
		authHeader         string
		enableAuth         bool
		expectedStatusCode int
	}{
		{
			name:               "create without auth should succeed",
			endpoint:           "/create",
			authHeader:         "",
			enableAuth:         true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "invoke without auth when disabled should succeed",
			endpoint:           "/invoke",
			authHeader:         "",
			enableAuth:         false,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:       "invoke without auth when enabled should be rejected",
			endpoint:   "/invoke",
			authHeader: "",
			enableAuth: true,
			// The route has JWT middleware and is not whitelisted; enabled auth requires a token.
			expectedStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth

			// Create request
			req, _ := http.NewRequest("POST", tt.endpoint, nil)
			if tt.authHeader != "" {
				req.Header.Set(jwtauth.HeaderXAuth, tt.authHeader)
			}

			// Execute
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify
			assert.Equal(t, tt.expectedStatusCode, w.Code)
		})
	}
}
