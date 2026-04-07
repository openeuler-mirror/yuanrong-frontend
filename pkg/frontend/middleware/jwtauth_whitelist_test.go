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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

func TestIsInAuthWhitelist(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		method   string
		expected bool
	}{
		{
			name:     "root path GET - should be in whitelist",
			path:     "/",
			method:   "GET",
			expected: true,
		},
		{
			name:     "root path POST - should not be in whitelist",
			path:     "/",
			method:   "POST",
			expected: false,
		},
		{
			name:     "healthz GET - should be in whitelist",
			path:     "/healthz",
			method:   "GET",
			expected: true,
		},
		{
			name:     "healthz POST - should not be in whitelist",
			path:     "/healthz",
			method:   "POST",
			expected: false,
		},
		{
			name:     "componentshealth GET - should be in whitelist",
			path:     "/serverless/v1/componentshealth",
			method:   "GET",
			expected: true,
		},
		{
			name:     "lease PUT - should be in whitelist",
			path:     "/client/v1/lease",
			method:   "PUT",
			expected: true,
		},
		{
			name:     "lease DELETE - should be in whitelist",
			path:     "/client/v1/lease",
			method:   "DELETE",
			expected: true,
		},
		{
			name:     "lease POST - should not be in whitelist",
			path:     "/client/v1/lease",
			method:   "POST",
			expected: false,
		},
		{
			name:     "terminal ws - should be in whitelist (auth handled in handler)",
			path:     "/terminal/ws",
			method:   "GET",
			expected: true,
		},
		{
			name:     "terminal static - should be in whitelist (public resources)",
			path:     "/terminal/static/style.css",
			method:   "GET",
			expected: true,
		},
		{
			name:     "terminal static JS - should be in whitelist",
			path:     "/terminal/static/xterm.js",
			method:   "GET",
			expected: true,
		},
		{
			name:     "auth login page - should be in whitelist",
			path:     "/auth/login-page",
			method:   "GET",
			expected: true,
		},
		{
			name:     "auth callback - should be in whitelist",
			path:     "/auth/callback",
			method:   "GET",
			expected: true,
		},
		{
			name:     "auth token exchange - should be in whitelist",
			path:     "/auth/token/exchange",
			method:   "POST",
			expected: true,
		},
		{
			name:     "invoke endpoint - should not be in whitelist",
			path:     "/serverless/v1/posix/instance/invoke",
			method:   "POST",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInAuthWhitelist(tt.path, tt.method)
			assert.Equal(t, tt.expected, result, "Expected %v for path %s %s", tt.expected, tt.method, tt.path)
		})
	}
}

func TestMatchRule(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		method   string
		rule     AuthWhitelistRule
		expected bool
	}{
		{
			name:   "exact match with correct method",
			path:   "/healthz",
			method: "GET",
			rule: AuthWhitelistRule{
				Path:      "/healthz",
				Methods:   []string{"GET"},
				MatchType: "exact",
			},
			expected: true,
		},
		{
			name:   "exact match with wrong method",
			path:   "/healthz",
			method: "POST",
			rule: AuthWhitelistRule{
				Path:      "/healthz",
				Methods:   []string{"GET"},
				MatchType: "exact",
			},
			expected: false,
		},
		{
			name:   "exact match with empty methods (all methods)",
			path:   "/healthz",
			method: "POST",
			rule: AuthWhitelistRule{
				Path:      "/healthz",
				Methods:   []string{},
				MatchType: "exact",
			},
			expected: true,
		},
		{
			name:   "prefix match - matched",
			path:   "/terminal/static/style.css",
			method: "GET",
			rule: AuthWhitelistRule{
				Path:      "/terminal",
				Methods:   []string{},
				MatchType: "prefix",
			},
			expected: true,
		},
		{
			name:   "prefix match - not matched",
			path:   "/api/terminal",
			method: "GET",
			rule: AuthWhitelistRule{
				Path:      "/terminal",
				Methods:   []string{},
				MatchType: "prefix",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchRule(tt.path, tt.method, tt.rule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCustomAuthWhitelist(t *testing.T) {
	// Save original whitelist
	originalWhitelist := customAuthWhitelist
	defer func() {
		customAuthWhitelist = originalWhitelist
	}()

	// Add custom rule
	customRule := AuthWhitelistRule{
		Path:      "/custom/api",
		Methods:   []string{"POST"},
		MatchType: "exact",
		SkipAuth:  true,
	}
	SetAuthWhitelist([]AuthWhitelistRule{customRule})

	// Test custom rule is applied
	result := isInAuthWhitelist("/custom/api", "POST")
	assert.True(t, result, "Custom whitelist rule should be applied")

	// Test original rules still work
	result = isInAuthWhitelist("/healthz", "GET")
	assert.True(t, result, "Default whitelist rules should still work")

	// Add another rule
	AddAuthWhitelistRule(AuthWhitelistRule{
		Path:      "/another/api",
		Methods:   []string{"GET"},
		MatchType: "prefix",
		SkipAuth:  true,
	})

	result = isInAuthWhitelist("/another/api/test", "GET")
	assert.True(t, result, "Added whitelist rule should be applied")
}

func TestGlobalJWTAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		path               string
		method             string
		enableAuth         bool
		authHeader         string
		mockParseJWT       func(string) (*jwtauth.ParsedJWT, error)
		mockValidateIAM    func(string, string) error
		expectedStatusCode int
		shouldCallNext     bool
	}{
		{
			name:               "whitelisted path should skip auth",
			path:               "/healthz",
			method:             "GET",
			enableAuth:         true,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:               "non-whitelisted path with auth disabled",
			path:               "/serverless/v1/posix/instance/invoke",
			method:             "POST",
			enableAuth:         false,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:               "non-whitelisted path without auth header",
			path:               "/serverless/v1/posix/instance/invoke",
			method:             "POST",
			enableAuth:         true,
			authHeader:         "",
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:               "terminal path now requires auth",
			path:               "/terminal/ws",
			method:             "GET",
			enableAuth:         true,
			authHeader:         "",
			expectedStatusCode: http.StatusOK, // No auth header but optional auth
			shouldCallNext:     true,
		},
		{
			name:       "invoke URL with RoleUser should be allowed",
			path:       "/serverless/v1/functions/urn-tenant1-ns1-func1-v1/invocations",
			method:     "POST",
			enableAuth: true,
			authHeader: "valid-jwt-user",
			mockParseJWT: func(token string) (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Role: jwtauth.RoleUser,
						Sub:  "tenant1",
					},
				}, nil
			},
			mockValidateIAM: func(token, traceID string) error {
				return nil
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:       "invoke URL with RoleDeveloper should be allowed",
			path:       "/serverless/v1/functions/urn-tenant1-ns1-func1-v1/invocations",
			method:     "POST",
			enableAuth: true,
			authHeader: "valid-jwt-developer",
			mockParseJWT: func(token string) (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Role: jwtauth.RoleDeveloper,
						Sub:  "tenant1",
					},
				}, nil
			},
			mockValidateIAM: func(token, traceID string) error {
				return nil
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:       "short invoke URL with RoleUser should be allowed",
			path:       "/tenant1/namespace1/function1/",
			method:     "POST",
			enableAuth: true,
			authHeader: "valid-jwt-user",
			mockParseJWT: func(token string) (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Role: jwtauth.RoleUser,
						Sub:  "tenant1",
					},
				}, nil
			},
			mockValidateIAM: func(token, traceID string) error {
				return nil
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
		{
			name:       "non-invoke URL with RoleUser should be rejected",
			path:       "/serverless/v1/posix/instance/create",
			method:     "POST",
			enableAuth: true,
			authHeader: "valid-jwt-user",
			mockParseJWT: func(token string) (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Role: jwtauth.RoleUser,
						Sub:  "tenant1",
					},
				}, nil
			},
			expectedStatusCode: http.StatusUnauthorized,
			shouldCallNext:     false,
		},
		{
			name:       "non-invoke URL with RoleDeveloper should be allowed",
			path:       "/serverless/v1/posix/instance/create",
			method:     "POST",
			enableAuth: true,
			authHeader: "valid-jwt-developer",
			mockParseJWT: func(token string) (*jwtauth.ParsedJWT, error) {
				return &jwtauth.ParsedJWT{
					Payload: &jwtauth.JWTPayload{
						Role: jwtauth.RoleDeveloper,
						Sub:  "tenant1",
					},
				}, nil
			},
			mockValidateIAM: func(token, traceID string) error {
				return nil
			},
			expectedStatusCode: http.StatusOK,
			shouldCallNext:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			config.GetConfig().IamConfig.EnableFuncTokenAuth = tt.enableAuth

			// Setup patches
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

			// Create test router
			router := gin.New()
			nextCalled := false
			// Apply both middleware in the correct order
			router.Use(InvokePreprocessMiddleware())
			router.Use(GlobalJWTAuthMiddleware())
			router.Handle(tt.method, tt.path, func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			// Create request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set(jwtauth.HeaderXAuth, tt.authHeader)
			}
			w := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(w, req)

			// Verify
			assert.Equal(t, tt.expectedStatusCode, w.Code)
			if tt.shouldCallNext {
				assert.True(t, nextCalled, "Next handler should be called")
			}
		})
	}
}
