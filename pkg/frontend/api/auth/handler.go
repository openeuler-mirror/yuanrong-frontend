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

// Package auth provides HTTP handlers for Casdoor authentication endpoints
package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

const (
	// CookieIamToken is the name of the cookie storing the IAM token
	CookieIamToken = "iam_token"
	// StateCookieName is the name of the cookie storing the OAuth2 state
	StateCookieName = "oauth_state"
	// NextCookieName stores the post-auth redirect target
	NextCookieName = "oauth_next"
	stateTTL       = 5 * time.Minute
)

var stateSigningKey = mustGenerateStateSigningKey()

// Handler handles authentication HTTP requests
type Handler struct {
	iamServerAddr string
}

// NewHandler creates a new auth Handler
func NewHandler(iamServerAddr string) *Handler {
	return &Handler{
		iamServerAddr: iamServerAddr,
	}
}

// LoginHandler redirects to Casdoor login page
func (h *Handler) LoginHandler(c *gin.Context) {
	state := generateState()
	redirectURI := getRedirectURI(c)
	secure := isSecureRequest(c)
	nextPath := getRequestedPostAuthPath(c)

	c.SetCookie(StateCookieName, state, 300, "/", "", secure, true)
	c.SetCookie(NextCookieName, nextPath, 300, "/", "", secure, true)

	authURL, err := h.getAuthURLFromIAM("login", redirectURI, state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get auth URL: " + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// RegisterHandler redirects to Casdoor registration page
func (h *Handler) RegisterHandler(c *gin.Context) {
	state := generateState()
	redirectURI := getRedirectURI(c)
	secure := isSecureRequest(c)
	nextPath := getRequestedPostAuthPath(c)

	c.SetCookie(StateCookieName, state, 300, "/", "", secure, true)
	c.SetCookie(NextCookieName, nextPath, 300, "/", "", secure, true)

	registerURL, err := h.getAuthURLFromIAM("register", redirectURI, state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get register URL: " + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, registerURL)
}

// CallbackHandler handles OAuth2 callback from Casdoor
func (h *Handler) CallbackHandler(c *gin.Context) {
	state := c.Query("state")
	storedState, err := c.Cookie(StateCookieName)
	if !isValidOAuthState(state, storedState, err) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid state parameter",
		})
		return
	}

	c.SetCookie(StateCookieName, "", -1, "/", "", isSecureRequest(c), true)
	nextPath, _ := c.Cookie(NextCookieName)
	c.SetCookie(NextCookieName, "", -1, "/", "", isSecureRequest(c), true)

	code := c.Query("code")
	if code == "" {
		errorDesc := c.Query("error_description")
		if errorDesc == "" {
			errorDesc = c.Query("error")
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Authorization failed: " + errorDesc,
		})
		return
	}

	redirectURI := getRedirectURI(c)
	iamToken, err := h.exchangeCodeForIamToken(c.Request.Context(), code, redirectURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to exchange authorization code: " + err.Error(),
		})
		return
	}

	c.SetCookie(CookieIamToken, iamToken, 3600, "/", "", isSecureRequest(c), true)

	c.Redirect(http.StatusTemporaryRedirect, appendTokenToRedirect(resolvePostAuthRedirect(c, nextPath), iamToken))
}

// LogoutHandler clears the session and redirects to Casdoor logout
func (h *Handler) LogoutHandler(c *gin.Context) {
	c.SetCookie(CookieIamToken, "", -1, "/", "", isSecureRequest(c), true)

	logoutURL, err := h.getAuthURLFromIAM("logout", getLogoutRedirectURI(c), "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "Logged out, but failed to redirect to Casdoor logout",
		})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, logoutURL)
}

// TokenExchangeRequest represents the request body for token exchange
type TokenExchangeRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

// DirectTokenRequest represents the request body for direct IAM token acquisition
type DirectTokenRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// TokenExchangeResponse represents the response for token exchange
type TokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// TokenExchangeHandler exchanges a Casdoor ID token for an IAM JWT token
func (h *Handler) TokenExchangeHandler(c *gin.Context) {
	var req TokenExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	iamToken, err := h.exchangeForIamToken(c.Request.Context(), req.IDToken, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to exchange token: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, TokenExchangeResponse{
		AccessToken: iamToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	})
}

// DirectTokenHandler accepts username/password and returns IAM token directly
func (h *Handler) DirectTokenHandler(c *gin.Context) {
	var req DirectTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	iamToken, err := h.loginWithPassword(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Login failed: " + err.Error(),
		})
		return
	}

	c.SetCookie(CookieIamToken, iamToken, 3600, "/", "", isSecureRequest(c), true)

	c.JSON(http.StatusOK, TokenExchangeResponse{
		AccessToken: iamToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	})
}

// UserInfo represents user information
type UserInfo struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// UserHandler returns the current user information
func (h *Handler) UserHandler(c *gin.Context) {
	iamToken, err := c.Cookie(CookieIamToken)
	if err != nil || iamToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Not authenticated",
		})
		return
	}

	parsed, err := jwtauth.ParseJWT(iamToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token: " + err.Error(),
		})
		return
	}

	if parsed.Payload.IsExpired(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Token expired",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"tenant_id":     parsed.Payload.Sub,
		"role":          parsed.Payload.Role,
	})
}

// exchangeForIamToken exchanges a Casdoor ID token for an IAM JWT token
func (h *Handler) exchangeForIamToken(ctx context.Context, idToken string, expiresIn int) (string, error) {
	cfg := config.GetConfig()
	iamAddr := cfg.IamConfig.Addr
	if iamAddr == "" {
		return "", fmt.Errorf("iam-server address not configured")
	}

	url := fmt.Sprintf("http://%s/iam-server/v1/token/exchange", iamAddr)

	reqBody := map[string]interface{}{
		"id_token": idToken,
	}
	if expiresIn > 0 {
		reqBody["expires_in"] = expiresIn
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call iam-server: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iam-server returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var respBody struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
		TenantID  string `json:"tenant_id"`
	}

	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	log.GetLogger().Infof("exchanged id token for iam token, tenant=%s", respBody.TenantID)
	return respBody.Token, nil
}

// getAuthURLFromIAM calls iam-server to get the Casdoor auth URL
func (h *Handler) getAuthURLFromIAM(authType, redirectURI, state string) (string, error) {
	cfg := config.GetConfig()
	iamAddr := cfg.IamConfig.Addr
	if iamAddr == "" {
		return "", fmt.Errorf("iam-server address not configured")
	}

	url := fmt.Sprintf("http://%s/iam-server/v1/auth/url?type=%s&redirect_uri=%s&state=%s",
		iamAddr, authType, url.QueryEscape(redirectURI), url.QueryEscape(state))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to call iam-server: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iam-server returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var respBody struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return respBody.URL, nil
}

// exchangeCodeForIamToken exchanges authorization code for IAM token via iam-server
func (h *Handler) exchangeCodeForIamToken(ctx context.Context, code, redirectURI string) (string, error) {
	cfg := config.GetConfig()
	iamAddr := cfg.IamConfig.Addr
	if iamAddr == "" {
		return "", fmt.Errorf("iam-server address not configured")
	}

	url := fmt.Sprintf("http://%s/iam-server/v1/token/code-exchange", iamAddr)

	reqBody := map[string]interface{}{
		"code":         code,
		"redirect_uri": redirectURI,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call iam-server: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iam-server returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var respBody struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
		TenantID  string `json:"tenant_id"`
		Role      string `json:"role"`
		CpuQuota  int64  `json:"cpu_quota"`
		MemQuota  int64  `json:"mem_quota"`
	}

	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	log.GetLogger().Infof("exchanged code for iam token, tenant=%s, role=%s", respBody.TenantID, respBody.Role)
	return respBody.Token, nil
}

// loginWithPassword authenticates username/password via iam-server
func (h *Handler) loginWithPassword(ctx context.Context, username, password string) (string, error) {
	cfg := config.GetConfig()
	iamAddr := cfg.IamConfig.Addr
	if iamAddr == "" {
		return "", fmt.Errorf("iam-server address not configured")
	}

	url := fmt.Sprintf("http://%s/iam-server/v1/token/login", iamAddr)

	reqBody := map[string]interface{}{
		"username": username,
		"password": password,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call iam-server: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("iam-server returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var respBody struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
		TenantID  string `json:"tenant_id"`
		Role      string `json:"role"`
		CpuQuota  int64  `json:"cpu_quota"`
		MemQuota  int64  `json:"mem_quota"`
	}

	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	log.GetLogger().Infof("password login successful, tenant=%s, role=%s", respBody.TenantID, respBody.Role)
	return respBody.Token, nil
}

// parseIamToken parses an IAM JWT token and extracts key claims
func parseIamToken(token string) (map[string]interface{}, error) {
	parsed, err := jwtauth.ParseJWT(token)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	return map[string]interface{}{
		"tenant_id": parsed.Payload.Sub,
		"role":      parsed.Payload.Role,
		"exp":       parsed.Payload.Exp,
	}, nil
}

// generateState generates a random state string for CSRF protection
func generateState() string {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	payload := hex.EncodeToString(nonce) + "." + strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, stateSigningKey)
	_, _ = mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func isValidOAuthState(state, storedState string, cookieErr error) bool {
	if state == "" {
		return false
	}
	if cookieErr == nil && storedState != "" && state == storedState {
		return true
	}

	parts := strings.Split(state, ".")
	if len(parts) != 3 {
		return false
	}

	payload := parts[0] + "." + parts[1]
	sig, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, stateSigningKey)
	_, _ = mac.Write([]byte(payload))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return false
	}

	issuedAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	if issuedAt > now+60 {
		return false
	}
	if now-issuedAt > int64(stateTTL/time.Second) {
		return false
	}

	return true
}

func mustGenerateStateSigningKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("failed to initialize oauth state signing key: %v", err))
	}
	return key
}

// LoginPageHandler redirects to the external OIDC login flow.
func (h *Handler) LoginPageHandler(c *gin.Context) {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	next := url.QueryEscape(getRequestedPostAuthPath(c))
	loginURL := pathPrefix + "/auth/login?next=" + next
	c.Redirect(http.StatusTemporaryRedirect, loginURL)
}

// RegisterPageHandler redirects to the external registration flow.
func (h *Handler) RegisterPageHandler(c *gin.Context) {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	next := url.QueryEscape(getRequestedPostAuthPath(c))
	registerURL := pathPrefix + "/auth/register?next=" + next
	c.Redirect(http.StatusTemporaryRedirect, registerURL)
}

// TokenExchangePageHandler renders a page for exchanging Casdoor ID token to IAM token
func (h *Handler) TokenExchangePageHandler(c *gin.Context) {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	homeURL := pathPrefix + "/"
	directTokenURL := pathPrefix + "/auth/token/direct"
	terminalBaseURL := pathPrefix + "/terminal"
	functionsBaseURL := pathPrefix + "/functions"

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>YuanRong Token Exchange</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		* {
			margin: 0;
			padding: 0;
			box-sizing: border-box;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
			min-height: 100vh;
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 20px;
		}
		.container {
			background: white;
			border-radius: 12px;
			box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
			max-width: 760px;
			width: 100%%;
			padding: 32px;
		}
		h1 {
			font-size: 28px;
			color: #2d3748;
			margin-bottom: 10px;
		}
		.subtitle {
			font-size: 14px;
			color: #718096;
			margin-bottom: 16px;
		}
		textarea, input {
			width: 100%%;
			border: 1px solid #cbd5e0;
			border-radius: 8px;
			padding: 12px;
			font-size: 14px;
			font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
		}
		input {
			min-height: 44px;
			margin-bottom: 10px;
		}
		.password-wrap {
			display: flex;
			gap: 8px;
			align-items: center;
			margin-bottom: 10px;
		}
		.password-wrap input {
			margin-bottom: 0;
			flex: 1;
		}
		.password-toggle {
			min-height: 44px;
			padding: 0 12px;
			border: 1px solid #cbd5e0;
			border-radius: 8px;
			background: #edf2f7;
			font-size: 13px;
			cursor: pointer;
		}
		textarea {
			min-height: 140px;
			resize: vertical;
		}
		textarea:focus, input:focus {
			outline: none;
			border-color: #667eea;
		}
		.button-row {
			display: flex;
			gap: 10px;
			flex-wrap: wrap;
			margin-top: 14px;
		}
		button, .link-btn {
			padding: 10px 16px;
			border: none;
			border-radius: 8px;
			cursor: pointer;
			font-size: 14px;
			text-decoration: none;
			display: inline-block;
		}
		.primary {
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
			color: white;
		}
		.secondary {
			background: #edf2f7;
			color: #2d3748;
		}
		.result {
			margin-top: 16px;
			padding: 12px;
			border-radius: 8px;
			font-size: 13px;
			line-height: 1.6;
			display: none;
		}
		.result.ok {
			background: #f0fff4;
			color: #2f855a;
			border: 1px solid #9ae6b4;
		}
		.result.err {
			background: #fff5f5;
			color: #c53030;
			border: 1px solid #feb2b2;
		}
		.token-output {
			margin-top: 12px;
			display: none;
		}
		.token-output textarea {
			min-height: 140px;
		}
		.footer-links {
			margin-top: 16px;
			display: flex;
			gap: 10px;
			flex-wrap: wrap;
		}
		.home-link {
			margin-top: 14px;
			display: inline-block;
			font-size: 14px;
			color: #667eea;
			text-decoration: none;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>Token Exchange</h1>
		<p class="subtitle">Enter username and password to get an IAM token directly. No Casdoor token is exposed to users.</p>
		<input id="username" type="text" placeholder="Username (e.g. testuser)" autocomplete="username">
		<div class="password-wrap">
			<input id="password" type="password" placeholder="Password" autocomplete="current-password">
			<button type="button" class="password-toggle" onclick="togglePassword()" id="passwordToggle">Show</button>
		</div>
		<div class="button-row">
			<button class="primary" onclick="exchangeToken()">Get Token</button>
			<button class="secondary" onclick="clearInput()">Clear</button>
		</div>

		<div id="result" class="result"></div>

		<div id="tokenOutput" class="token-output">
			<textarea id="iamToken" readonly></textarea>
			<div class="footer-links">
				<button class="secondary" onclick="copyToken()">Copy IAM Token</button>
				<a class="link-btn primary" id="goTerminal" href="%s">Open Terminal with Token</a>
				<a class="link-btn secondary" id="goFunctions" href="%s">Open Functions with Token</a>
			</div>
		</div>

		<a class="home-link" href="%s">← Back to Home</a>
	</div>

	<script>
		const directTokenUrl = '%s';
		const terminalBaseUrl = '%s';
		const functionsBaseUrl = '%s';

		function setResult(message, ok) {
			const result = document.getElementById('result');
			result.style.display = 'block';
			result.className = ok ? 'result ok' : 'result err';
			result.textContent = message;
		}

		function clearInput() {
			document.getElementById('username').value = '';
			document.getElementById('password').value = '';
			document.getElementById('password').type = 'password';
			document.getElementById('passwordToggle').textContent = 'Show';
			document.getElementById('iamToken').value = '';
			document.getElementById('result').style.display = 'none';
			document.getElementById('tokenOutput').style.display = 'none';
		}

		function togglePassword() {
			const passwordInput = document.getElementById('password');
			const toggleBtn = document.getElementById('passwordToggle');
			if (passwordInput.type === 'password') {
				passwordInput.type = 'text';
				toggleBtn.textContent = 'Hide';
			} else {
				passwordInput.type = 'password';
				toggleBtn.textContent = 'Show';
			}
		}

		async function exchangeToken() {
			const username = document.getElementById('username').value.trim();
			const password = document.getElementById('password').value;
			if (!username || !password) {
				setResult('Please enter both username and password.', false);
				return;
			}

			try {
				setResult('Signing in and requesting IAM token...', true);
				const resp = await fetch(directTokenUrl, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ username: username, password: password })
				});

				const data = await resp.json();
				if (!resp.ok) {
					setResult(data.error || ('Token request failed, HTTP ' + resp.status), false);
					return;
				}

				const accessToken = data.access_token || '';
				document.getElementById('iamToken').value = accessToken;
				document.getElementById('tokenOutput').style.display = 'block';

				const terminalUrl = new URL(terminalBaseUrl, window.location.origin);
				terminalUrl.searchParams.set('token', accessToken);
				document.getElementById('goTerminal').href = terminalUrl.toString();

				const functionsUrl = new URL(functionsBaseUrl, window.location.origin);
				functionsUrl.searchParams.set('token', accessToken);
				document.getElementById('goFunctions').href = functionsUrl.toString();

				setResult('Success. You can copy the token or jump directly.', true);
			} catch (err) {
				setResult('Request failed: ' + err, false);
			}
		}

		async function copyToken() {
			const token = document.getElementById('iamToken').value;
			if (!token) {
				setResult('There is no IAM token to copy.', false);
				return;
			}
			try {
				await navigator.clipboard.writeText(token);
				setResult('IAM token copied to clipboard.', true);
			} catch (err) {
				setResult('Copy failed. Please copy from the text box manually.', false);
			}
		}
</script>
</body>
</html>`, terminalBaseURL, functionsBaseURL, homeURL, directTokenURL, terminalBaseURL, functionsBaseURL)

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// TokenPageHandler renders a page for viewing and copying the current IAM token.
func (h *Handler) TokenPageHandler(c *gin.Context) {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	homeURL := pathPrefix + "/"
	terminalURL := pathPrefix + "/terminal"
	functionsURL := pathPrefix + "/functions"
	loginURL := pathPrefix + "/auth/login?next=" + url.QueryEscape(pathPrefix+"/auth/token-page")

	iamToken, err := c.Cookie(CookieIamToken)
	if err != nil || iamToken == "" {
		c.Redirect(http.StatusTemporaryRedirect, loginURL)
		return
	}

	terminalURL = appendTokenToRedirect(terminalURL, iamToken)
	functionsURL = appendTokenToRedirect(functionsURL, iamToken)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>YuanRong API Token</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
			min-height: 100vh;
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 20px;
		}
		.container {
			background: white;
			border-radius: 12px;
			box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
			max-width: 760px;
			width: 100%%;
			padding: 32px;
		}
		h1 {
			font-size: 28px;
			color: #2d3748;
			margin-bottom: 10px;
		}
		.subtitle {
			font-size: 14px;
			color: #718096;
			margin-bottom: 16px;
			line-height: 1.6;
		}
		textarea {
			width: 100%%;
			min-height: 180px;
			border: 1px solid #cbd5e0;
			border-radius: 8px;
			padding: 12px;
			font-size: 13px;
			font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
			resize: vertical;
		}
		.button-row {
			display: flex;
			gap: 10px;
			flex-wrap: wrap;
			margin-top: 14px;
			margin-bottom: 16px;
		}
		button, .link-btn {
			padding: 10px 16px;
			border: none;
			border-radius: 8px;
			cursor: pointer;
			font-size: 14px;
			text-decoration: none;
			display: inline-block;
		}
		.primary {
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
			color: white;
		}
		.secondary {
			background: #edf2f7;
			color: #2d3748;
		}
		pre {
			margin-top: 12px;
			padding: 12px;
			background: #0f172a;
			color: #e2e8f0;
			border-radius: 8px;
			overflow-x: auto;
			font-size: 12px;
			line-height: 1.5;
		}
		.result {
			margin-top: 14px;
			padding: 12px;
			border-radius: 8px;
			font-size: 13px;
			display: none;
		}
		.result.ok {
			background: #f0fff4;
			color: #2f855a;
			border: 1px solid #9ae6b4;
		}
		.result.err {
			background: #fff5f5;
			color: #c53030;
			border: 1px solid #feb2b2;
		}
		.footer-link {
			margin-top: 16px;
			display: inline-block;
			color: #667eea;
			text-decoration: none;
			font-size: 14px;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>API Token</h1>
		<p class="subtitle">Use this IAM token for API calls. It is also wired into the quick links below for tools that still expect a query token.</p>
		<textarea id="iamToken" readonly>%s</textarea>
		<div class="button-row">
			<button class="primary" onclick="copyToken()">Copy IAM Token</button>
			<a class="link-btn secondary" href="%s">Open Terminal</a>
			<a class="link-btn secondary" href="%s">Open Functions</a>
			<a class="link-btn secondary" href="%s">Sign In Again</a>
		</div>
		<pre>curl -H "X-Auth: %s" http://wyc.pc:18888/api/instances</pre>
		<div id="result" class="result"></div>
		<a class="footer-link" href="%s">← Back to Home</a>
	</div>
	<script>
		async function copyToken() {
			const token = document.getElementById('iamToken').value;
			const result = document.getElementById('result');
			try {
				await navigator.clipboard.writeText(token);
				result.style.display = 'block';
				result.className = 'result ok';
				result.textContent = 'IAM token copied to clipboard.';
			} catch (err) {
				result.style.display = 'block';
				result.className = 'result err';
				result.textContent = 'Copy failed. Please copy it manually.';
			}
		}
	</script>
</body>
</html>`, iamToken, terminalURL, functionsURL, loginURL, iamToken, homeURL)

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func renderAuthPage(c *gin.Context, title, subtitle, actionPath, buttonText string) {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	actionURL := pathPrefix + actionPath
	if !strings.Contains(actionPath, "next=") {
		if next := url.QueryEscape(getRequestedPostAuthPath(c)); next != "" {
			separator := "?"
			if strings.Contains(actionURL, "?") {
				separator = "&"
			}
			actionURL += separator + "next=" + next
		}
	}
	homeURL := pathPrefix + "/"

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>YuanRong %s</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		* {
			margin: 0;
			padding: 0;
			box-sizing: border-box;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
			min-height: 100vh;
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 20px;
		}
		.container {
			background: white;
			border-radius: 12px;
			box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
			max-width: 520px;
			width: 100%%;
			padding: 48px 36px;
			text-align: center;
		}
		h1 {
			font-size: 30px;
			color: #2d3748;
			margin-bottom: 14px;
		}
		.subtitle {
			color: #718096;
			line-height: 1.7;
			margin-bottom: 28px;
		}
		.action {
			display: inline-block;
			width: 100%%;
			text-decoration: none;
			background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
			color: white;
			padding: 14px 20px;
			border-radius: 8px;
			font-weight: 600;
			font-size: 16px;
		}
		.action:hover {
			opacity: 0.95;
		}
		.back-link {
			display: inline-block;
			color: #667eea;
			text-decoration: none;
			margin-top: 18px;
			font-size: 14px;
		}
		.back-link:hover {
			text-decoration: underline;
		}
		.tip {
			margin-top: 20px;
			font-size: 13px;
			color: #a0aec0;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>%s</h1>
		<p class="subtitle">%s</p>
		<a class="action" href="%s">%s</a>
		<a class="back-link" href="%s">← 返回首页</a>
		<p class="tip">认证成功后会自动跳转回平台首页。</p>
	</div>
</body>
</html>`, title, title, subtitle, actionURL, buttonText, homeURL)

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// getRedirectURI constructs the OAuth2 redirect URI
func getRedirectURI(c *gin.Context) string {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	return getBaseURL(c) + pathPrefix + "/auth/callback"
}

func getLogoutRedirectURI(c *gin.Context) string {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	return getBaseURL(c) + pathPrefix + "/auth/login-page"
}

// getBaseURL constructs the base URL from the request
func getBaseURL(c *gin.Context) string {
	host := c.Request.Host
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}

	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}

	return scheme + "://" + host
}

func getRequestedPostAuthPath(c *gin.Context) string {
	if next := sanitizeRelativePath(c.Query("next")); next != "" {
		return next
	}
	return defaultPostAuthPath(c)
}

func resolvePostAuthRedirect(c *gin.Context, next string) string {
	if sanitized := sanitizeRelativePath(next); sanitized != "" {
		return sanitized
	}
	return defaultPostAuthPath(c)
}

func appendTokenToRedirect(target, token string) string {
	if target == "" || token == "" {
		return target
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}

	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func defaultPostAuthPath(c *gin.Context) string {
	pathPrefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	return pathPrefix + "/terminal"
}

func sanitizeRelativePath(next string) string {
	if next == "" {
		return ""
	}
	if !strings.HasPrefix(next, "/") {
		return ""
	}
	if strings.HasPrefix(next, "//") {
		return ""
	}
	return next
}

func isSecureRequest(c *gin.Context) bool {
	proto := strings.ToLower(c.GetHeader("X-Forwarded-Proto"))
	if proto != "" {
		return proto == "https"
	}
	return c.Request.TLS != nil
}
