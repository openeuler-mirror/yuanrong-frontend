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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
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
		mockParseJWT       func() (*jwtauth.ParsedJWT, error)
		mockValidateIAM    func(string, string) error
		requestBody        map[string]interface{}
		expectedStatusCode int
	}{
		{
			name:         "invoke without JWT when auth disabled",
			enableAuth:   false,
			authHeader:   "",
			requestBody:  map[string]string{"test": "data"},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:         "invoke without JWT when auth enabled (optional auth)",
			enableAuth:   true,
			authHeader:   "",
			requestBody:  map[string]string{"test": "data"},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:       "invoke with invalid JWT",
			enableAuth: true,
			authHeader: "invalid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return nil, errors.New("invalid JWT")
			},
			requestBody:        map[string]string{"test": "data"},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:       "invoke with wrong role",
			enableAuth: true,
			authHeader: "valid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Sub:  "tenant123",
						Role: "user",
					},
				}, nil
			},
			requestBody:        map[string]string{"test": "data"},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:       "invoke with valid JWT but IAM validation failed",
			enableAuth: true,
			authHeader: "valid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Sub:  "tenant123",
						Role: "developer",
					},
				}, nil
			},
			mockValidateIAM: func(authHeader, traceID string) error {
				return errors.New("IAM validation failed")
			},
			requestBody:        map[string]string{"test": "data"},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:       "invoke with valid JWT and IAM",
			enableAuth: true,
			authHeader: "valid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Sub:  "tenant123",
						Role: "developer",
					},
				}, nil
			},
			mockValidateIAM: func(authHeader, traceID string) error {
				return nil
			},
			requestBody:        map[string]string{"test": "data"},
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth

			// Setup mocks
			patches := make([]*gomonkey.Patches, 0)
			if tt.mockParseJWT != nil {
				patch := gomonkey.ApplyFunc(jwtauth.ParseJWT, tt.mockParseJWT)
				patches = append(patches, patch)
			}
			if tt.mockValidateIAM != nil {
				patch := gomonkey.ApplyFunc(jwtauth.ValidateWithIamServer, tt.mockValidateIAM)
				patches = append(patches, patch)
			}
			defer func() {
				for _, p := range patches {
					p.Reset()
				}
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
			name:               "invoke without auth when enabled should succeed (optional)",
			endpoint:           "/invoke",
			authHeader:         "",
			enableAuth:         true,
			expectedStatusCode: http.StatusOK,
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
