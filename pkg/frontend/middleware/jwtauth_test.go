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
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

func TestJWTAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		enableAuth         bool
		authHeader         string
		mockParseJWT       func() (*jwtauth.ParsedJWT, error)
		mockValidateIAM    func(string, string) error
		expectedStatusCode int
		shouldCallNext     bool
	}{
		{
			name:               "auth disabled, should pass",
			enableAuth:         false,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:               "no auth header, should pass",
			enableAuth:         true,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:       "invalid JWT format, should return 401",
			enableAuth: true,
			authHeader: "invalid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return nil, errors.New("invalid JWT format")
			},
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:       "invalid role, should return 401",
			enableAuth: true,
			authHeader: "valid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Header: &jwtauth.JWTHeader{
						Alg: "HMAC-SHA256",
						Typ: "JWT",
					},
					Payload: &jwtauth.JWTPayload{
						Sub:  "tenant123",
						Role: "user",
					},
				}, nil
			},
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:       "IAM validation failed, should return 401",
			enableAuth: true,
			authHeader: "valid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Header: &jwtauth.JWTHeader{
						Alg: "HMAC-SHA256",
						Typ: "JWT",
					},
					Payload: &jwtauth.JWTPayload{
						Sub:  "tenant123",
						Role: "developer",
					},
				}, nil
			},
			mockValidateIAM: func(authHeader, traceID string) error {
				return errors.New("IAM validation failed")
			},
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:       "valid JWT and IAM, should pass",
			enableAuth: true,
			authHeader: "valid.jwt.token",
			mockParseJWT: func() (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Header: &jwtauth.JWTHeader{
						Alg: "HMAC-SHA256",
						Typ: "JWT",
					},
					Payload: &jwtauth.JWTPayload{
						Sub:  "tenant123",
						Role: "developer",
					},
				}, nil
			},
			mockValidateIAM: func(authHeader, traceID string) error {
				return nil
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			w := httptest.NewRecorder()
			c, router := gin.CreateTestContext(w)
			
			// Set config
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth

			// Mock functions
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

			// Track if next was called
			nextCalled := false
			router.POST("/test", JWTAuthMiddleware(), func(ctx *gin.Context) {
				nextCalled = true
				ctx.Status(http.StatusOK)
			})

			// Set auth header and create request
			c.Request, _ = http.NewRequest("POST", "/test", nil)
			if tt.authHeader != "" {
				c.Request.Header.Set(jwtauth.HeaderXAuth, tt.authHeader)
			}

			// Execute
			router.ServeHTTP(w, c.Request)

			// Verify
			assert.Equal(t, tt.expectedStatusCode, w.Code, "status code mismatch")
			
			if tt.shouldCallNext {
				assert.True(t, nextCalled || w.Code == http.StatusOK, "handler should be called")
			} else {
				assert.False(t, nextCalled && w.Code == http.StatusOK, "handler should not be called")
			}
		})
	}
}

func TestJWTAuthMiddleware_Integration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup router with middleware
	router := gin.New()
	router.POST("/test", JWTAuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	tests := []struct {
		name               string
		enableAuth         bool
		authHeader         string
		expectedStatusCode int
	}{
		{
			name:               "no middleware protection when auth disabled",
			enableAuth:         false,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "optional auth when no header provided",
			enableAuth:         true,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set config
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth

			// Create request
			req, _ := http.NewRequest("POST", "/test", nil)
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
