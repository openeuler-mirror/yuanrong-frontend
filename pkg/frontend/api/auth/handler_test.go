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

package auth

import (
	"encoding/base64"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/types"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	return c, w
}

func setIAMServerConfig(t *testing.T, addr string) {
	t.Helper()
	prev := *config.GetConfig()
	config.SetConfig(types.Config{
		IamConfig: types.IamConfig{
			Addr: addr,
		},
	})
	t.Cleanup(func() {
		config.SetConfig(prev)
	})
}

func TestLoginHandlerRedirectsToExternalAuthURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iam-server/v1/auth/url" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "login" {
			t.Fatalf("unexpected auth type: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"http://casdoor.example/login"}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/login", nil)
	c.Request.Host = "localhost:3000"

	handler.LoginHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "http://casdoor.example/login" {
		t.Fatalf("unexpected redirect location: %s", got)
	}

	var stateCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == StateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected oauth state cookie to be set")
	}
}

func TestRegisterHandlerRedirectsToRegisterURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("type"); got != "register" {
			t.Fatalf("unexpected auth type: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"http://casdoor.example/signup"}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/register", nil)
	c.Request.Host = "localhost:3000"

	handler.RegisterHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "http://casdoor.example/signup" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestLoginPageHandlerRedirectsToOIDCEntrypoint(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/login-page", nil)
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")

	handler.LoginPageHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/frontend/auth/login?next=%2Ffrontend%2Fterminal" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestRegisterPageHandlerRedirectsToRegisterEntrypoint(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/register-page", nil)
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")

	handler.RegisterPageHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/frontend/auth/register?next=%2Ffrontend%2Fterminal" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestTokenExchangePageHandlerRendersDirectEndpoint(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/token-exchange-page", nil)
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")

	handler.TokenExchangePageHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !strings.Contains(w.Body.String(), "/frontend/auth/token/direct") {
		t.Fatalf("expected direct token endpoint in page body")
	}
}

func TestTokenPageHandlerRendersCurrentToken(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/token-page", nil)
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")
	c.Request.AddCookie(&http.Cookie{Name: CookieIamToken, Value: "iam-token"})

	handler.TokenPageHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "iam-token") {
		t.Fatalf("expected token to be rendered")
	}
	if !strings.Contains(body, "/frontend/terminal?token=iam-token") {
		t.Fatalf("expected terminal link to include token, got: %s", body)
	}
}

func TestTokenPageHandlerPromptsLoginAndReturnsToTokenPage(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/token-page", nil)
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")

	handler.TokenPageHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/frontend/auth/login?next=%2Ffrontend%2Fauth%2Ftoken-page" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestDirectTokenHandlerInvalidRequest(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodPost, "/auth/token/direct", []byte(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DirectTokenHandler(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestDirectTokenHandlerPropagatesIAMError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unsupported"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodPost, "/auth/token/direct", []byte(`{"username":"developer","password":"dev123"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DirectTokenHandler(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(resp["error"], "iam-server returned status 401") {
		t.Fatalf("unexpected error response: %s", resp["error"])
	}
}

func TestTokenExchangeHandlerSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iam-server/v1/token/exchange" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"iam-token","tenant_id":"developer","expires_in":3600}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodPost, "/auth/token/exchange", []byte(`{"id_token":"id-token"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.TokenExchangeHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var resp TokenExchangeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AccessToken != "iam-token" {
		t.Fatalf("unexpected access token: %s", resp.AccessToken)
	}
}

func TestCallbackHandlerInvalidState(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/callback?state=invalid&code=test-code", nil)
	c.Request.AddCookie(&http.Cookie{Name: StateCookieName, Value: "different-state"})

	handler.CallbackHandler(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCallbackHandlerExchangesCodeAndSetsCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iam-server/v1/token/code-exchange" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"iam-token","tenant_id":"developer","role":"developer","expires_in":3600}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/callback?state=test-state&code=test-code", nil)
	c.Request.Host = "localhost:3000"
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")
	c.Request.AddCookie(&http.Cookie{Name: StateCookieName, Value: "test-state"})
	c.Request.AddCookie(&http.Cookie{Name: NextCookieName, Value: "/frontend/functions"})

	handler.CallbackHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/frontend/functions?token=iam-token" {
		t.Fatalf("unexpected redirect location: %s", got)
	}

	var iamCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == CookieIamToken {
			iamCookie = cookie
			break
		}
	}
	if iamCookie == nil || iamCookie.Value != "iam-token" {
		t.Fatalf("expected iam token cookie to be set")
	}
}

func TestCallbackHandlerAcceptsSignedStateWithoutCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"iam-token","tenant_id":"developer","role":"developer","expires_in":3600}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	state := generateState()
	c, w := newTestContext(http.MethodGet, "/auth/callback?state="+url.QueryEscape(state)+"&code=test-code", nil)
	c.Request.Host = "localhost:3000"

	handler.CallbackHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/terminal?token=iam-token" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestLogoutHandlerClearsCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("type"); got != "logout" {
			t.Fatalf("unexpected auth type: %s", got)
		}
		if got := r.URL.Query().Get("redirect_uri"); got != "http://localhost:3000/auth/login-page" {
			t.Fatalf("unexpected redirect_uri: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"http://casdoor.example/logout"}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodPost, "/auth/logout", nil)
	c.Request.Host = "localhost:3000"

	handler.LogoutHandler(c)

	if w.Code != http.StatusOK && w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected logout to succeed gracefully, got status %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "http://casdoor.example/logout" {
		t.Fatalf("unexpected logout redirect location: %s", got)
	}

	var iamCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == CookieIamToken {
			iamCookie = cookie
			break
		}
	}
	if iamCookie == nil || iamCookie.Value != "" || iamCookie.MaxAge != -1 {
		t.Fatalf("expected iam cookie to be cleared")
	}
}

func TestCallbackHandlerDefaultsToTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"iam-token","tenant_id":"developer","role":"developer","expires_in":3600}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/callback?state=test-state&code=test-code", nil)
	c.Request.Host = "localhost:3000"
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")
	c.Request.AddCookie(&http.Cookie{Name: StateCookieName, Value: "test-state"})

	handler.CallbackHandler(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/frontend/terminal?token=iam-token" {
		t.Fatalf("unexpected redirect location: %s", got)
	}
}

func TestUserHandlerNotAuthenticated(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/user", nil)

	handler.UserHandler(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerAcceptsPermanentToken(t *testing.T) {
	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/user", nil)

	token := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"tenant-permanent","exp":-1,"role":"developer"}`)),
		"signature",
	}, ".")
	c.Request.AddCookie(&http.Cookie{Name: CookieIamToken, Value: token})

	handler.UserHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"tenant_id":"tenant-permanent"`) {
		t.Fatalf("expected permanent token tenant in response, got: %s", w.Body.String())
	}
}

func TestGenerateState(t *testing.T) {
	state1 := generateState()
	state2 := generateState()

	if state1 == state2 {
		t.Fatal("generated states should be unique")
	}
	if !isValidOAuthState(state1, "", http.ErrNoCookie) {
		t.Fatalf("expected generated state to be self-validating")
	}
}

func TestGetRedirectURI(t *testing.T) {
	c, _ := newTestContext(http.MethodGet, "/auth/login", nil)
	c.Request.Host = "localhost:3000"

	if got := getRedirectURI(c); got != "http://localhost:3000/auth/callback" {
		t.Fatalf("unexpected redirect URI: %s", got)
	}
}

func TestGetRedirectURIWithPrefix(t *testing.T) {
	c, _ := newTestContext(http.MethodGet, "/auth/login", nil)
	c.Request.Host = "localhost:3000"
	c.Request.Header.Set("X-Forwarded-Prefix", "/frontend")

	if got := getRedirectURI(c); got != "http://localhost:3000/frontend/auth/callback" {
		t.Fatalf("unexpected redirect URI: %s", got)
	}
}

func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		tls      bool
		expected string
	}{
		{name: "http", host: "localhost:3000", expected: "http://localhost:3000"},
		{name: "https", host: "localhost:443", tls: true, expected: "https://localhost:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestContext(http.MethodGet, "/", nil)
			c.Request.Host = tt.host
			if tt.tls {
				c.Request.TLS = &tls.ConnectionState{}
			}

			if got := getBaseURL(c); got != tt.expected {
				t.Fatalf("unexpected base URL: %s", got)
			}
		})
	}
}

func TestGetBaseURLWithForwardedHeaders(t *testing.T) {
	c, _ := newTestContext(http.MethodGet, "/", nil)
	c.Request.Host = "internal:8080"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "example.com")

	if got := getBaseURL(c); got != "https://example.com" {
		t.Fatalf("unexpected base URL: %s", got)
	}
}

func TestLoginHandlerReturnsErrorWhenIAMMissing(t *testing.T) {
	setIAMServerConfig(t, "")

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/login", nil)

	handler.LoginHandler(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestGetAuthURLFromIAMEncodesRedirectURI(t *testing.T) {
	var gotRedirect string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRedirect = r.URL.Query().Get("redirect_uri")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"http://casdoor.example/login"}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	authURL, err := handler.getAuthURLFromIAM("login", "http://localhost:3000/auth/callback?x=1", "state")
	if err != nil {
		t.Fatalf("getAuthURLFromIAM failed: %v", err)
	}
	if authURL != "http://casdoor.example/login" {
		t.Fatalf("unexpected auth URL: %s", authURL)
	}
	if gotRedirect != "http://localhost:3000/auth/callback?x=1" {
		t.Fatalf("unexpected redirect URI query: %s", gotRedirect)
	}
}

func TestGetAuthURLFromIAMRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	_, err := handler.getAuthURLFromIAM("login", "http://localhost:3000/auth/callback", "state")
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoginRedirectContainsStateQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"http://casdoor.example/login?foo=bar"}`))
	}))
	defer server.Close()

	setIAMServerConfig(t, strings.TrimPrefix(server.URL, "http://"))

	handler := NewHandler("unused")
	c, w := newTestContext(http.MethodGet, "/auth/login", nil)
	c.Request.Host = "localhost:3000"

	handler.LoginHandler(c)

	location := w.Header().Get("Location")
	parsedURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("failed to parse redirect URL: %v", err)
	}
	if parsedURL.Host != "casdoor.example" {
		t.Fatalf("unexpected redirect host: %s", parsedURL.Host)
	}
}
