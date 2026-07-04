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
	var gotPath, gotBody, gotAuth, gotQueryToken, gotInternal string
	rrt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotPath = r.URL.Path
		gotBody = string(body)
		gotAuth = r.Header.Get(jwtauth.HeaderXAuth)
		gotQueryToken = r.URL.Query().Get("token")
		gotInternal = r.Header.Get(internalSrcHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer rrt.Close()

	target := targetTo(t, rrt.URL)
	target.Tenant = "default"
	router := New(fakeResolver{target: target})
	router.SetAuth(true, false, 50090, 8765)
	routerServer := httptest.NewServer(router)
	defer routerServer.Close()

	routerURL, err := url.Parse(routerServer.URL)
	if err != nil {
		t.Fatalf("parse router URL: %v", err)
	}
	frontendProxy := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
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
	}}
	frontend := httptest.NewServer(frontendProxy)
	defer frontend.Close()

	body := []byte(`{"action":"file.exists","args":{"path":"/"}}`)
	req, err := http.NewRequest(http.MethodPost, frontend.URL+"/direct/default-rtt/50090/invoke?token=secret&keep=1", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(jwtauth.HeaderXAuth, mintJWT("default", farFuture))
	resp, err := frontend.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %q", resp.StatusCode, payload)
	}
	if gotPath != "/invoke" {
		t.Fatalf("RRT path = %q, want /invoke", gotPath)
	}
	if gotBody != string(body) {
		t.Fatalf("RRT body = %q, want %q", gotBody, body)
	}
	if gotAuth != "" || gotQueryToken != "" || gotInternal != "" {
		t.Fatalf("RRT saw leaked credentials/internal marker: X-Auth=%q token=%q internal=%q", gotAuth, gotQueryToken, gotInternal)
	}
}
