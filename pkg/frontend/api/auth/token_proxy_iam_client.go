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
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"frontend/pkg/common/faas_common/constant"
)

const (
	developerRole            = "developer"
	systemTenantID           = "0"
	iamHeaderAuth            = "X-Auth"
	iamHeaderTenantID        = "X-Tenant-ID"
	iamHeaderRole            = "X-Role"
	iamHeaderTTL             = "X-TTL"
	iamHeaderExpiredTimeSpan = "X-Expired-Time-Span"

	iamRequireTokenPath = "/iam-server/v1/token/require"
)

// TokenProxyIAMClient calls iam-server token endpoints after frontend authorization.
type TokenProxyIAMClient interface {
	RequireToken(ctx context.Context, tenantID, role string, ttl *int64, traceID string) (*TokenProxyResponse, error)
}

// TokenProxyResponse preserves successful iam-server response headers and body.
type TokenProxyResponse struct {
	Headers http.Header
	Body    []byte
}

// TokenProxyIAMError preserves non-200 iam-server response status.
type TokenProxyIAMError struct {
	StatusCode int
	Message    string
}

func (e *TokenProxyIAMError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("iam server returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("iam server returned status %d: %s", e.StatusCode, e.Message)
}

type defaultTokenProxyIAMClient struct {
	iamServerAddr string
	httpClient    *http.Client
}

// NewDefaultTokenProxyIAMClient creates a token proxy IAM client backed by net/http.
func NewDefaultTokenProxyIAMClient(iamServerAddr string) TokenProxyIAMClient {
	return &defaultTokenProxyIAMClient{
		iamServerAddr: strings.TrimRight(iamServerAddr, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *defaultTokenProxyIAMClient) RequireToken(ctx context.Context, tenantID, role string, ttl *int64,
	traceID string) (*TokenProxyResponse, error) {
	req, err := c.newRequest(ctx, iamRequireTokenPath, traceID)
	if err != nil {
		return nil, err
	}
	req.Header.Set(iamHeaderTenantID, tenantID)
	req.Header.Set(iamHeaderRole, role)
	if ttl != nil {
		req.Header.Set(iamHeaderTTL, strconv.FormatInt(*ttl, 10))
	}
	return c.doOK(req)
}

func (c *defaultTokenProxyIAMClient) newRequest(ctx context.Context, path string,
	traceID string) (*http.Request, error) {
	if c.iamServerAddr == "" {
		return nil, fmt.Errorf("iam-server address not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+c.iamServerAddr+path, nil)
	if err != nil {
		return nil, err
	}
	if traceID != "" {
		req.Header.Set(constant.HeaderTraceID, traceID)
	}
	return req, nil
}

func (c *defaultTokenProxyIAMClient) doOK(req *http.Request) (*TokenProxyResponse, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &TokenProxyIAMError{
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(string(body)),
		}
	}
	return &TokenProxyResponse{
		Headers: resp.Header.Clone(),
		Body:    body,
	}, nil
}
