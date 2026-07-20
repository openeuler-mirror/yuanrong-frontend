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

package resources

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubActiveMasterClient struct {
	addr string
}

func (s stubActiveMasterClient) GetActiveMasterAddr() string {
	return s.addr
}

func TestQueryHandlerForwardsProtobufRequestToActiveMaster(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/global-scheduler/resources" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Type"); got != "protobuf" {
			t.Fatalf("expected Type header protobuf, got %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		if string(body) != "request-bytes" {
			t.Fatalf("expected forwarded request body, got %q", string(body))
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusAccepted)
		if _, err := w.Write([]byte("response-bytes")); err != nil {
			t.Fatalf("write upstream response: %v", err)
		}
	}))
	defer upstream.Close()

	oldNewClient := newActiveMasterClient
	oldHTTPClient := resourceHTTPClient
	defer func() {
		newActiveMasterClient = oldNewClient
		resourceHTTPClient = oldHTTPClient
	}()
	newActiveMasterClient = func() activeMasterClient {
		return stubActiveMasterClient{addr: strings.TrimPrefix(upstream.URL, "http://")}
	}
	resourceHTTPClient = upstream.Client()

	r := gin.New()
	r.GET("/global-scheduler/resources", QueryHandler)
	req := httptest.NewRequest(http.MethodGet, "/global-scheduler/resources", strings.NewReader("request-bytes"))
	req.Header.Set("Type", "protobuf")
	resp := httptest.NewRecorder()

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != "application/protobuf" {
		t.Fatalf("expected Content-Type application/protobuf, got %q", got)
	}
	if got := resp.Body.String(); got != "response-bytes" {
		t.Fatalf("expected response body forwarded, got %q", got)
	}
}

func TestQueryHandlerReturnsServiceUnavailableWhenActiveMasterMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldNewClient := newActiveMasterClient
	defer func() { newActiveMasterClient = oldNewClient }()
	newActiveMasterClient = func() activeMasterClient { return stubActiveMasterClient{} }

	r := gin.New()
	r.GET("/global-scheduler/resources", QueryHandler)
	req := httptest.NewRequest(http.MethodGet, "/global-scheduler/resources", nil)
	resp := httptest.NewRecorder()

	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, resp.Code)
	}
}
