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

package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"frontend/pkg/frontend/common/jwtauth"
)

func TestFrontendDirectEndToEndDoesNotLeakTokenToRRT(t *testing.T) {
	seen := &frontendDirectSeen{}
	rrt := newFrontendDirectRRT(t, seen)
	defer rrt.Close()

	target := targetTo(t, rrt.URL)
	target.Tenant = "default"
	router := New(fakeResolver{target: target})
	router.SetAuth(true, false, proxyTestRRTPort, proxyTestTunnelWSPort)
	routerServer := httptest.NewServer(router)
	defer routerServer.Close()

	frontend := httptest.NewServer(newFrontendDirectProxy(t, routerServer.URL))
	defer frontend.Close()

	body := []byte(`{"action":"file.exists","args":{"path":"/"}}`)
	resp := sendFrontendDirectRequest(t, frontend.URL, body)
	defer resp.Body.Close()

	assertFrontendDirectResponse(t, resp, seen, body)
}

func newFrontendDirectProxy(t *testing.T, routerServerURL string) *httputil.ReverseProxy {
	t.Helper()
	routerURL, err := url.Parse(routerServerURL)
	if err != nil {
		t.Fatalf("parse router URL: %v", err)
	}
	return &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		in := pr.In
		pr.SetURL(routerURL)
		pr.Out.URL.Path = strings.TrimPrefix(in.URL.Path, "/direct")
		pr.Out.URL.RawQuery = in.URL.RawQuery
		if q := pr.Out.URL.Query(); q.Has("token") {
			q.Del("token")
			pr.Out.URL.RawQuery = q.Encode()
		}
		pr.SetXForwarded()
		pr.Out.Header.Del(jwtauth.HeaderXAuth)
		pr.Out.Header.Set(internalSrcHeader, internalSrcAuthenticatedValue)
		pr.Out.Header.Set(internalTenantHeader, "default")
	}}
}

func sendFrontendDirectRequest(t *testing.T, frontendURL string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(
		http.MethodPost,
		frontendURL+"/direct/default-rtt/50090/invoke?token=secret&keep=1",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(jwtauth.HeaderXAuth, mintJWT(t, "default", farFuture))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func assertFrontendDirectResponse(
	t *testing.T,
	resp *http.Response,
	seen *frontendDirectSeen,
	body []byte,
) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		t.Fatalf("status = %d, body = %q", resp.StatusCode, payload)
	}
	if seen.path != "/invoke" {
		t.Fatalf("RRT path = %q, want /invoke", seen.path)
	}
	if seen.body != string(body) {
		t.Fatalf("RRT body = %q, want %q", seen.body, body)
	}
	if seen.auth != "" || seen.queryToken != "" || seen.internal != "" {
		t.Fatalf(
			"RRT saw leaked credentials/internal marker: X-Auth=%q token=%q internal=%q",
			seen.auth,
			seen.queryToken,
			seen.internal,
		)
	}
}

type frontendDirectSeen struct {
	path       string
	body       string
	auth       string
	queryToken string
	internal   string
}

func newFrontendDirectRRT(t *testing.T, seen *frontendDirectSeen) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		seen.path = r.URL.Path
		seen.body = string(body)
		seen.auth = r.Header.Get(jwtauth.HeaderXAuth)
		seen.queryToken = r.URL.Query().Get("token")
		seen.internal = r.Header.Get(internalSrcHeader)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
}
