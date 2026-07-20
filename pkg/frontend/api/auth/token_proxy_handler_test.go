/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
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

package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/types"
)

type fakeTokenProxyIAMClient struct {
	requireCalls int

	requiredTenantID string
	requiredRole     string
	requiredTTL      *int64
	requiredTraceID  string

	requireResponse *TokenProxyResponse
	requireErr      error
}

func (f *fakeTokenProxyIAMClient) totalCalls() int {
	return f.requireCalls
}

func (f *fakeTokenProxyIAMClient) RequireToken(_ context.Context, tenantID, role string, ttl *int64,
	traceID string) (*TokenProxyResponse, error) {
	f.requireCalls++
	f.requiredTenantID = tenantID
	f.requiredRole = role
	if ttl != nil {
		ttlValue := *ttl
		f.requiredTTL = &ttlValue
	}
	f.requiredTraceID = traceID
	if f.requireErr != nil {
		return nil, f.requireErr
	}
	return f.requireResponse, nil
}

func newTokenProxyHandlerTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(""))
	return ctx, recorder
}

func tokenProxyTestJWT(t *testing.T, sub, role string, exp int64) string {
	t.Helper()
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	payload := map[string]interface{}{
		"sub":  sub,
		"role": role,
		"exp":  exp,
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal jwt header: %v", err)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal jwt payload: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payloadBytes) + ".signature"
}

func configureTokenProxyAuth(t *testing.T, iamAddr string, validator func(string, string) error) {
	t.Helper()
	prevConfig := *config.GetConfig()
	prevValidator := tokenProxyTokenValidator
	config.SetConfig(types.Config{
		IamConfig: types.IamConfig{
			Addr: iamAddr,
		},
	})
	tokenProxyTokenValidator = validator
	t.Cleanup(func() {
		config.SetConfig(prevConfig)
		tokenProxyTokenValidator = prevValidator
	})
}

func operatorJWT(t *testing.T) string {
	configureTokenProxyAuth(t, "iam.example", func(string, string) error {
		return nil
	})
	return tokenProxyTestJWT(t, systemTenantID, developerRole, time.Now().Add(time.Hour).Unix())
}

func TestRequireTokenProxyForwardsDeveloperTokenRequest(t *testing.T) {
	ttl := int64(3600)
	client := &fakeTokenProxyIAMClient{
		requireResponse: &TokenProxyResponse{
			Headers: http.Header{
				iamHeaderAuth:            []string{"developer-token"},
				iamHeaderExpiredTimeSpan: []string{"1782988800"},
			},
			Body: []byte("OK"),
		},
	}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
	ctx.Request.Header.Set(iamHeaderAuth, operatorJWT(t))
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-001")
	ctx.Request.Header.Set(iamHeaderRole, developerRole)
	ctx.Request.Header.Set(iamHeaderTTL, "3600")
	ctx.Request.Header.Set(constant.HeaderTraceID, "trace-001")

	handler.RequireTokenProxyHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if client.requireCalls != 1 {
		t.Fatalf("expected one require call, got %d", client.requireCalls)
	}
	if client.requiredTenantID != "dev-001" || client.requiredRole != developerRole {
		t.Fatalf("unexpected forwarded target: tenant=%s role=%s", client.requiredTenantID, client.requiredRole)
	}
	if client.requiredTTL == nil || *client.requiredTTL != ttl {
		t.Fatalf("unexpected ttl: %#v", client.requiredTTL)
	}
	if client.requiredTraceID != "trace-001" {
		t.Fatalf("unexpected trace id: %s", client.requiredTraceID)
	}
	if recorder.Header().Get(iamHeaderAuth) != "developer-token" {
		t.Fatalf("developer token header was not proxied")
	}
	if recorder.Body.String() != "OK" {
		t.Fatalf("unexpected body: %q", recorder.Body.String())
	}
}

func TestRequireTokenProxyRejectsNonSystemCaller(t *testing.T) {
	configureTokenProxyAuth(t, "iam.example", func(string, string) error {
		return nil
	})
	client := &fakeTokenProxyIAMClient{}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
	ctx.Request.Header.Set(iamHeaderAuth, tokenProxyTestJWT(t, "dev-001", developerRole,
		time.Now().Add(time.Hour).Unix()))
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-002")
	ctx.Request.Header.Set(iamHeaderRole, developerRole)

	handler.RequireTokenProxyHandler(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if client.totalCalls() != 0 {
		t.Fatalf("iam client should not be called")
	}
}

func TestRequireTokenProxyDelegatesCallerValidationEachRequest(t *testing.T) {
	validateCalls := 0
	configureTokenProxyAuth(t, "iam.example", func(string, string) error {
		validateCalls++
		return nil
	})
	token := tokenProxyTestJWT(t, systemTenantID, developerRole, time.Now().Add(time.Hour).Unix())
	client := &fakeTokenProxyIAMClient{
		requireResponse: &TokenProxyResponse{Body: []byte("OK")},
	}
	handler := &Handler{tokenProxyClient: client}

	for i := 0; i < 2; i++ {
		ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
		ctx.Request.Header.Set(iamHeaderAuth, token)
		ctx.Request.Header.Set(iamHeaderTenantID, "dev-001")
		ctx.Request.Header.Set(iamHeaderRole, developerRole)

		handler.RequireTokenProxyHandler(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d expected status %d, got %d: %s", i, http.StatusOK, recorder.Code,
				recorder.Body.String())
		}
	}
	const expectedRequestCount = 2
	if validateCalls != expectedRequestCount {
		t.Fatalf("validated caller token %d times, want %d", validateCalls, expectedRequestCount)
	}
	if client.requireCalls != expectedRequestCount {
		t.Fatalf("expected both proxied requests to reach iam-server, got %d", client.requireCalls)
	}
}

func TestRequireTokenProxyAcceptsNonExpiringSystemCallerToken(t *testing.T) {
	configureTokenProxyAuth(t, "iam.example", func(string, string) error {
		return nil
	})
	client := &fakeTokenProxyIAMClient{
		requireResponse: &TokenProxyResponse{Body: []byte("OK")},
	}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
	ctx.Request.Header.Set(iamHeaderAuth, tokenProxyTestJWT(t, systemTenantID, developerRole, -1))
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-ttl-minus-one")
	ctx.Request.Header.Set(iamHeaderRole, developerRole)
	ctx.Request.Header.Set(iamHeaderTTL, "-1")

	handler.RequireTokenProxyHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if client.requireCalls != 1 {
		t.Fatalf("expected one require call, got %d", client.requireCalls)
	}
	if client.requiredTTL == nil || *client.requiredTTL != -1 {
		t.Fatalf("unexpected ttl: %#v", client.requiredTTL)
	}
}

func TestRequireTokenProxyRejectsNonDeveloperTargetRole(t *testing.T) {
	client := &fakeTokenProxyIAMClient{}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
	ctx.Request.Header.Set(iamHeaderAuth, operatorJWT(t))
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-001")
	ctx.Request.Header.Set(iamHeaderRole, "user")

	handler.RequireTokenProxyHandler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if client.totalCalls() != 0 {
		t.Fatalf("iam client should not be called")
	}
}

func TestAbandonTokenProxyDeclaresUnsupported(t *testing.T) {
	client := &fakeTokenProxyIAMClient{}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/abandon")
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-001")

	handler.AbandonTokenProxyHandler(ctx)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotImplemented, recorder.Code, recorder.Body.String())
	}
	if client.totalCalls() != 0 {
		t.Fatalf("iam client should not be called")
	}
}

func TestTokenProxyRejectsReservedTargetTenant(t *testing.T) {
	client := &fakeTokenProxyIAMClient{}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/abandon")
	ctx.Request.Header.Set(iamHeaderAuth, operatorJWT(t))
	ctx.Request.Header.Set(iamHeaderTenantID, systemTenantID)

	handler.AbandonTokenProxyHandler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
	if client.totalCalls() != 0 {
		t.Fatalf("iam client should not be called")
	}
}

func TestTokenProxyMapsIAMError(t *testing.T) {
	client := &fakeTokenProxyIAMClient{
		requireErr: &TokenProxyIAMError{StatusCode: http.StatusNotFound, Message: "not found"},
	}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
	ctx.Request.Header.Set(iamHeaderAuth, operatorJWT(t))
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-001")
	ctx.Request.Header.Set(iamHeaderRole, developerRole)

	handler.RequireTokenProxyHandler(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestTokenProxyMapsTransportErrorToBadGateway(t *testing.T) {
	client := &fakeTokenProxyIAMClient{
		requireErr: errors.New("connection refused"),
	}
	handler := &Handler{tokenProxyClient: client}
	ctx, recorder := newTokenProxyHandlerTestContext(http.MethodGet, "/auth/token/require")
	ctx.Request.Header.Set(iamHeaderAuth, operatorJWT(t))
	ctx.Request.Header.Set(iamHeaderTenantID, "dev-001")
	ctx.Request.Header.Set(iamHeaderRole, developerRole)

	handler.RequireTokenProxyHandler(ctx)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, recorder.Code)
	}
}
