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

// Package api wraps different api versions, and can be easily switched between different versions
// API provides http handlers used by fast-http, the handlers should only do http context checking and should dispatch
// the actual logic to
package api

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	faasconstant "frontend/pkg/common/faas_common/constant"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
	routerconfig "frontend/pkg/sandboxrouter/config"
)

var cfg = `{
		"slaQuota":1000,
		"functionCapability":1,
		"authenticationEnable":false,
		"trafficLimitDisable":true,
		"http":{
		"resptimeout":5,
		"workerInstanceReadTimeOut":5,
		"maxRequestBodySize":6
		},
		"dataSystemConfig":{
		"uploadWriteMode":"NoneL2Cache",
		"executeWriteMode":"NoneL2Cache",
		"uploadTTLSec":86400,
		"executeTTLSec":1800,
		"timeoutMs":60000
		},
		"businessType":1
	}`

func TestInitRoute(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	type args struct {
		r *gin.Engine
	}
	tests := []struct {
		name string
		args args
	}{
		{"case1 init route caas", args{r: gin.New()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitRoute(tt.args.r)
		})
	}
}

func TestSandboxDirectRouteForwardsToLocalSandboxRouter(t *testing.T) {
	patches := gomonkey.ApplyFunc(jwtauth.ParseJWT, func(string) (*jwtauth.ParsedJWT, error) {
		return &jwtauth.ParsedJWT{Payload: &jwtauth.JWTPayload{Sub: "tenant-a", Role: jwtauth.RoleUser}}, nil
	})
	patches.ApplyFunc(jwtauth.ValidateWithIamServer, func(string, string) error { return nil })
	defer patches.Reset()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	seen := make(chan *http.Request, 1)
	backend := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})}
	go func() { _ = backend.Serve(listener) }()
	defer backend.Close()

	config.InitFunctionConfig([]byte(fmt.Sprintf(`{
		"slaQuota":1000,
		"functionCapability":1,
		"authenticationEnable":false,
		"trafficLimitDisable":true,
		"businessType":1,
		"iamConfig":{"enableFuncTokenAuth":true},
		"sandboxRouter":{"enabled":true,"listenIP":"127.0.0.1","listenPort":%d}
	}`, listener.Addr().(*net.TCPAddr).Port)))

	r := gin.New()
	InitRoute(r)

	server := httptest.NewServer(r)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/direct/demo/invoke?x=1&token=secret&tenant_id=default", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Auth", "token")
	req.Header.Set("X-Internal-Tenant", "spoofed-tenant")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set(faasconstant.CaaSHeaderTraceID, "trace-direct")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	got := <-seen
	if got.URL.Path != "/demo/50090/invoke" || got.URL.RawQuery != "x=1" {
		t.Fatalf("unexpected proxied URL: %s?%s", got.URL.Path, got.URL.RawQuery)
	}
	if got.Header.Get("X-Auth") != "" {
		t.Fatalf("X-Auth was forwarded to sandboxRouter")
	}
	if got.Header.Get("X-Internal-Src") != "1" {
		t.Fatalf("internal source marker was not set")
	}
	if got.Header.Get("X-Internal-Tenant") != "tenant-a" {
		t.Fatalf("internal tenant = %q, want validated JWT tenant", got.Header.Get("X-Internal-Tenant"))
	}
	if got.Header.Get("X-Forwarded-Proto") != "https" {
		t.Fatalf("X-Forwarded-Proto was not preserved: %q", got.Header.Get("X-Forwarded-Proto"))
	}
	if got.Header.Get(faasconstant.HeaderTraceID) != "trace-direct" {
		t.Fatalf("trace header was not forwarded: %q", got.Header.Get(faasconstant.HeaderTraceID))
	}
	if resp.Header.Get(faasconstant.HeaderTraceID) != "trace-direct" {
		t.Fatalf("trace header was not returned: %q", resp.Header.Get(faasconstant.HeaderTraceID))
	}
}

func TestSandboxDirectRouteDefersAuthToRouterWhenFrontendJWTDisabled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	seen := make(chan *http.Request, 1)
	backend := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = backend.Serve(listener) }()
	defer backend.Close()

	config.InitFunctionConfig([]byte(fmt.Sprintf(`{
		"slaQuota":1000,
		"functionCapability":1,
		"authenticationEnable":false,
		"trafficLimitDisable":true,
		"businessType":1,
		"iamConfig":{"enableFuncTokenAuth":false},
		"sandboxRouter":{"enabled":true,"listenIP":"127.0.0.1","listenPort":%d}
	}`, listener.Addr().(*net.TCPAddr).Port)))

	r := gin.New()
	InitRoute(r)
	server := httptest.NewServer(r)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/direct/demo/invoke?token=query-token&tenant_id=spoofed", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(jwtauth.HeaderXAuth, "router-token")
	req.Header.Set("X-Internal-Src", "1")
	req.Header.Set("X-Internal-Tenant", "spoofed-tenant")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	got := <-seen
	if got.Header.Get(jwtauth.HeaderXAuth) != "router-token" {
		t.Fatalf("router X-Auth = %q, want original token", got.Header.Get(jwtauth.HeaderXAuth))
	}
	if got.URL.Query().Get("token") != "query-token" {
		t.Fatalf("router query token = %q, want original token", got.URL.Query().Get("token"))
	}
	if got.URL.Query().Get("tenant_id") != "" {
		t.Fatalf("untrusted tenant_id reached router: %q", got.URL.Query().Get("tenant_id"))
	}
	if got.Header.Get("X-Internal-Src") != "2" {
		t.Fatalf("internal source marker = %q, want deferred-auth marker", got.Header.Get("X-Internal-Src"))
	}
	if got.Header.Get("X-Internal-Tenant") != "" {
		t.Fatalf("untrusted internal tenant reached router: %q", got.Header.Get("X-Internal-Tenant"))
	}
}

func TestSandboxDirectRouterPathHidesControlPorts(t *testing.T) {
	cfg := &routerconfig.SandboxRouterConfig{RRTPort: 50090, TunnelPort: 8765}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "rrt invoke alias", in: "/direct/demo/invoke", want: "/demo/50090/invoke"},
		{name: "rrt health alias", in: "/direct/demo/healthz", want: "/demo/50090/healthz"},
		{name: "rrt upload alias", in: "/direct/demo/upload", want: "/demo/50090/upload"},
		{name: "rrt download alias", in: "/direct/demo/download", want: "/demo/50090/download"},
		{name: "explicit rrt port rejected", in: "/direct/demo/50090/invoke", want: "/"},
		{name: "explicit user port rejected", in: "/direct/demo/8080/", want: "/"},
		{name: "unknown operation rejected", in: "/direct/demo/admin", want: "/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sandboxDirectRouterPath(tc.in, cfg); got != tc.want {
				t.Fatalf("sandboxDirectRouterPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSandboxTunnelRouterPathHidesControlPort(t *testing.T) {
	cfg := &routerconfig.SandboxRouterConfig{TunnelPort: 8765}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "tunnel alias", in: "/tunnel/demo", want: "/demo/8765"},
		{name: "tunnel alias with tail", in: "/tunnel/demo/ws", want: "/demo/8765/ws"},
		{name: "legacy port form", in: "/tunnel/demo/8765", want: "/demo/8765"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sandboxTunnelRouterPath(tc.in, cfg); got != tc.want {
				t.Fatalf("sandboxTunnelRouterPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestInitRouteRegistersPosixWebSocket(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	r := gin.New()
	InitRoute(r)

	req := httptest.NewRequest(http.MethodGet, "/serverless/v1/posix/ws", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("POSIX WebSocket route is not registered")
	}
}

func TestInitRouteRegistersGlobalSchedulerResources(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	r := gin.New()
	InitRoute(r)

	if routeExists(r, http.MethodGet, "/global-scheduler/resources") {
		return
	}

	t.Fatalf("GET /global-scheduler/resources route is not registered")
}

func TestInitRouteRegistersProtectedTokenProxyRoutes(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	r := gin.New()
	InitRoute(r)

	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/auth/token/require"},
		{method: http.MethodGet, path: "/auth/token/abandon"},
	}

	for _, route := range routes {
		if !routeExists(r, route.method, route.path) {
			t.Fatalf("%s %s route is not registered", route.method, route.path)
		}
	}
}

func TestInitRouteDoesNotRegisterDeveloperTenantCRUDRoutes(t *testing.T) {
	config.InitFunctionConfig([]byte(cfg))
	r := gin.New()
	InitRoute(r)

	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/auth/tenants/developers"},
		{method: http.MethodGet, path: "/auth/tenants/developers/:tenant_id"},
		{method: http.MethodDelete, path: "/auth/tenants/developers/:tenant_id"},
		{method: http.MethodPatch, path: "/auth/tenants/developers/:tenant_id"},
	}
	for _, route := range routes {
		if routeExists(r, route.method, route.path) {
			t.Fatalf("%s %s route should not be registered", route.method, route.path)
		}
	}
}

func routeExists(r *gin.Engine, method string, path string) bool {
	for _, route := range r.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}
