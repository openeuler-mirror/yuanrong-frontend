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
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"frontend/pkg/sandboxrouter/route"
)

type fakeResolver struct {
	target *route.RouteTarget
	err    error
}

func (f fakeResolver) Resolve(_ context.Context, _ route.RouteKey) (*route.RouteTarget, error) {
	return f.target, f.err
}

func targetTo(t *testing.T, rawURL string) *route.RouteTarget {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return &route.RouteTarget{TargetURL: u, Scheme: u.Scheme}
}

func do(s *Server, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// Happy path: prefix stripped, query preserved, headers added, body proxied.
func TestProxyForwardsStrippedPath(t *testing.T) {
	var gotPath, gotQuery, gotInstID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotInstID = r.URL.Path, r.URL.RawQuery, r.Header.Get("X-Instance-Id")
		io.WriteString(w, "pong")
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)})
	rec := do(s, http.MethodGet, "/inst-a/8080/api/foo?x=1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "pong" {
		t.Errorf("body = %q, want pong", rec.Body.String())
	}
	if gotPath != "/api/foo" {
		t.Errorf("upstream path = %q, want /api/foo", gotPath)
	}
	if gotQuery != "x=1" {
		t.Errorf("upstream query = %q, want x=1", gotQuery)
	}
	if gotInstID != "inst-a" {
		t.Errorf("X-Instance-Id = %q, want inst-a", gotInstID)
	}
}

// Error codes must match Traefik: unmatched route -> 404, not 400.
func TestProxyErrorCodes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	cases := []struct {
		name     string
		resolver Resolver
		path     string
		want     int
	}{
		{"unparseable path", fakeResolver{target: targetTo(t, upstream.URL)}, "/", http.StatusNotFound},
		{"port not numeric", fakeResolver{target: targetTo(t, upstream.URL)}, "/inst/nope", http.StatusNotFound},
		{"route not found", fakeResolver{err: route.ErrRouteNotFound}, "/inst/8080/x", http.StatusNotFound},
		{"resolver unavailable", fakeResolver{err: errors.New("boom")}, "/inst/8080/x", http.StatusServiceUnavailable},
		{"upstream refused", fakeResolver{target: targetTo(t, "http://127.0.0.1:1")}, "/inst/8080/x", http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(New(c.resolver), http.MethodGet, c.path)
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

// Upstream timeout maps to 504, not 502.
func TestProxyUpstreamTimeoutIs504(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)})
	s.transport = &http.Transport{ResponseHeaderTimeout: 20 * time.Millisecond}

	rec := do(s, http.MethodGet, "/inst/8080/x")
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", rec.Code)
	}
}
