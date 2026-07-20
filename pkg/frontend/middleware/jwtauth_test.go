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
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		iamAddr            string
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
			name:       "no auth header when auth enabled, should be rejected",
			enableAuth: true,
			authHeader: "",
			// EnableFuncTokenAuth=true means JWTAuthMiddleware must reject non-whitelisted requests without a token.
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:               "invalid JWT format, should return 401",
			enableAuth:         true,
			authHeader:         "invalid.jwt.token",
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:               "invalid role, should return 401",
			enableAuth:         true,
			authHeader:         testJWT("tenant123", "user"),
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:       "IAM validation failed, should return 401",
			enableAuth: true,
			authHeader: testJWT("tenant123", "developer"),
			// Non-empty unreachable IAM address exercises the real IAM validation failure path.
			iamAddr:            "127.0.0.1:1",
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:       "valid JWT and IAM, should pass",
			enableAuth: true,
			authHeader: testJWT("tenant123", "developer"),
			// Empty IAM address intentionally exercises ValidateWithIamServer's configured skip path.
			iamAddr:            "",
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
			origIAMAddr := config.GetConfig().IamConfig.Addr
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth
			config.GetConfig().IamConfig.Addr = tt.iamAddr
			defer func() {
				config.GetConfig().IamConfig.Addr = origIAMAddr
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

func testJWT(sub, role string) string {
	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
			`{"sub":"%s","exp":9876543210,"role":"%s"}`, sub, role))),
		"signature",
	}, ".")
}

func TestJWTAuthMiddlewareStoresClaimsInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	origEnable := config.GetConfig().IamConfig.EnableFuncTokenAuth
	origIAMAddr := config.GetConfig().IamConfig.Addr
	config.GetConfig().IamConfig.EnableFuncTokenAuth = true
	config.GetConfig().IamConfig.Addr = ""
	defer func() {
		config.GetConfig().IamConfig.EnableFuncTokenAuth = origEnable
		config.GetConfig().IamConfig.Addr = origIAMAddr
	}()

	router := gin.New()
	router.POST("/test", JWTAuthMiddleware(), func(ctx *gin.Context) {
		sub, _ := ctx.Get("jwt_sub")
		role, _ := ctx.Get("jwt_role")
		ctx.JSON(http.StatusOK, gin.H{
			"sub":  sub,
			"role": role,
		})
	})

	req, err := http.NewRequest(http.MethodPost, "/test", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set(jwtauth.HeaderXAuth, testJWT("tenant123", "developer"))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"sub":"tenant123"`)
	assert.Contains(t, recorder.Body.String(), `"role":"developer"`)
}

func TestJWTAuthMiddlewareIntegration(t *testing.T) {
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
			name:       "auth enabled without header is rejected",
			enableAuth: true,
			authHeader: "",
			// Direct JWTAuthMiddleware is strict when auth is enabled; whitelist skipping is handled by GlobalJWTAuthMiddleware.
			expectedStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set config
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth

			// Create request
			req, err := http.NewRequest("POST", "/test", nil)
			assert.NoError(t, err)
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
