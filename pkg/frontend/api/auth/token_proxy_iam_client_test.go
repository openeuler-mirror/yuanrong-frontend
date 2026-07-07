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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"frontend/pkg/common/faas_common/constant"
)

func newTestTokenProxyIAMClient(serverURL string) TokenProxyIAMClient {
	return NewDefaultTokenProxyIAMClient(strings.TrimPrefix(serverURL, "http://"))
}

func TestDefaultTokenProxyIAMClientRequireToken(t *testing.T) {
	ttl := int64(3600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/iam-server/v1/token/require" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get(iamHeaderTenantID); got != "dev-001" {
			t.Fatalf("unexpected tenant header: %s", got)
		}
		if got := r.Header.Get(iamHeaderRole); got != developerRole {
			t.Fatalf("unexpected role header: %s", got)
		}
		if got := r.Header.Get(iamHeaderTTL); got != "3600" {
			t.Fatalf("unexpected ttl header: %s", got)
		}
		if got := r.Header.Get(constant.HeaderTraceID); got != "trace-001" {
			t.Fatalf("unexpected trace header: %s", got)
		}

		w.Header().Set(iamHeaderAuth, "developer-token")
		w.Header().Set(iamHeaderExpiredTimeSpan, "1782988800")
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client := newTestTokenProxyIAMClient(server.URL)
	result, err := client.RequireToken(context.Background(), "dev-001", developerRole, &ttl, "trace-001")
	if err != nil {
		t.Fatalf("RequireToken returned error: %v", err)
	}
	if result.Headers.Get(iamHeaderAuth) != "developer-token" {
		t.Fatalf("unexpected token header: %s", result.Headers.Get(iamHeaderAuth))
	}
	if string(result.Body) != "OK" {
		t.Fatalf("unexpected body: %q", string(result.Body))
	}
}

func TestDefaultTokenProxyIAMClientRequiresConfiguredIAMServerAddress(t *testing.T) {
	client := NewDefaultTokenProxyIAMClient("")
	_, err := client.RequireToken(context.Background(), "dev-001", developerRole, nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "iam-server address not configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultTokenProxyIAMClientReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestTokenProxyIAMClient(server.URL)
	_, err := client.RequireToken(context.Background(), "dev-001", developerRole, nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
	var iamErr *TokenProxyIAMError
	if !errors.As(err, &iamErr) {
		t.Fatalf("expected TokenProxyIAMError, got %T", err)
	}
	if iamErr.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", iamErr.StatusCode)
	}
	if iamErr.Message != "not found" {
		t.Fatalf("unexpected message: %q", iamErr.Message)
	}
}
