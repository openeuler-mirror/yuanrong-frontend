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
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"frontend/pkg/frontend/common/jwtauth"
)

// mintJWT builds an unsigned-but-well-formed X-Auth token (Header.Payload.sig);
// the router only decodes locally (validateIAM off), so the signature is opaque.
func mintJWT(sub string, exp uint64) string {
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	header := enc(jwtauth.JWTHeader{Alg: "HS256", Typ: "JWT"})
	payload := enc(jwtauth.JWTPayload{Sub: sub, Exp: exp})
	return header + "." + payload + ".sig"
}

// doAuth issues a request carrying an X-Auth header.
func doAuth(s *Server, method, target, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set(jwtauth.HeaderXAuth, token)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

const farFuture = 9999999999 // ~year 2286, never expired in tests

// With auth on, a request without X-Auth is rejected 401 before resolving.
func TestAuthMissingTokenIs401(t *testing.T) {
	s := New(fakeResolver{target: targetTo(t, "http://127.0.0.1:1")})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuth(s, http.MethodGet, "/default-rtt/50090/healthz", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// An expired token is rejected 401.
func TestAuthExpiredTokenIs401(t *testing.T) {
	s := New(fakeResolver{target: targetTo(t, "http://127.0.0.1:1")})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuth(s, http.MethodGet, "/default-rtt/50090/healthz", mintJWT("default", 1))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// A token whose tenant (Sub) does not own the target sandbox is rejected 403.
// The owning tenant comes from the resolved target (set from the instance key),
// not from the request path.
func TestAuthTenantMismatchIs403(t *testing.T) {
	tg := targetTo(t, "http://127.0.0.1:1")
	tg.Tenant = "tenant-a"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuth(s, http.MethodGet, "/default-rtt/50090/healthz", mintJWT("tenant-b", farFuture))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// When the resolver cannot attribute a tenant to the route, authorization must
// fail closed instead of exposing an unattributed sandbox.
func TestAuthUnknownTenantFailsClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)}) // target.Tenant == ""
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuth(s, http.MethodGet, "/default-rtt/50090/healthz", mintJWT("tenant-b", farFuture))
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (fail closed on unknown tenant)", rec.Code)
	}
}

// A valid token for the RRT built-in port passes auth AND the platform token is
// forwarded to the backend (RRT re-authenticates).
func TestAuthValidTokenForwardedToRRTPort(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get(jwtauth.HeaderXAuth)
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	token := mintJWT("default", farFuture)
	rec := doAuth(s, http.MethodGet, "/default-rtt/50090/healthz", token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotAuth != token {
		t.Errorf("backend X-Auth = %q, want token forwarded to RRT port", gotAuth)
	}
}

func TestFrontendAuthenticatedRRTPortSkipsRouterAuthAndStripsToken(t *testing.T) {
	var gotAuth, gotQuery, gotInternal string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get(jwtauth.HeaderXAuth)
		gotQuery = r.URL.Query().Get("token")
		gotInternal = r.Header.Get(internalSrcHeader)
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	req := httptest.NewRequest(http.MethodGet, "/default-rtt/50090/healthz?token=secret&keep=1", nil)
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set(internalSrcHeader, internalSrcAuthenticatedValue)
	req.Header.Set(internalTenantHeader, "default")
	req.Header.Set(jwtauth.HeaderXAuth, "frontend-token")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotAuth != "" || gotQuery != "" || gotInternal != "" {
		t.Fatalf("backend saw leaked auth/internal headers: X-Auth=%q token=%q internal=%q", gotAuth, gotQuery, gotInternal)
	}
}

func TestFrontendAuthenticatedRRTPortRejectsTenantMismatch(t *testing.T) {
	tg := targetTo(t, "http://127.0.0.1:1")
	tg.Tenant = "tenant-a"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	req := httptest.NewRequest(http.MethodGet, "/victim-rtt/50090/healthz", nil)
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set(internalSrcHeader, internalSrcAuthenticatedValue)
	req.Header.Set(internalTenantHeader, "tenant-b")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestFrontendAuthenticatedRRTPortRejectsMissingTenant(t *testing.T) {
	tg := targetTo(t, "http://127.0.0.1:1")
	tg.Tenant = "tenant-a"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	req := httptest.NewRequest(http.MethodGet, "/victim-rtt/50090/healthz", nil)
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set(internalSrcHeader, internalSrcAuthenticatedValue)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestSpoofedFrontendInternalHeaderOffLoopbackStillRequiresToken(t *testing.T) {
	s := New(fakeResolver{target: targetTo(t, "http://127.0.0.1:1")})
	s.SetAuth(true, false, 50090, 8765)
	req := httptest.NewRequest(http.MethodGet, "/default-rtt/50090/healthz", nil)
	req.RemoteAddr = "203.0.113.10:43210"
	req.Header.Set(internalSrcHeader, internalSrcAuthenticatedValue)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// For a user service port the platform token must be stripped before proxying,
// so the user's service never sees it.
func TestAuthTokenStrippedForUserPort(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get(jwtauth.HeaderXAuth)
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765) // RRT port is 50090; 8080 below is a user port
	rec := doAuth(s, http.MethodGet, "/default-app/8080/api", mintJWT("default", farFuture))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotAuth != "" {
		t.Errorf("backend X-Auth = %q, want empty (stripped for user port)", gotAuth)
	}
}

// The ?token= query fallback (used by WebSocket clients that cannot set
// headers) authenticates, but for a user service port the token must be
// stripped from the forwarded URL too — not just the header — or it leaks.
func TestAuthQueryTokenStrippedForUserPort(t *testing.T) {
	var gotHdr, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHdr = r.Header.Get(jwtauth.HeaderXAuth)
		gotQuery = r.URL.Query().Get("token")
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765) // 8080 below is a user port
	// token only in query (no header), plus an unrelated query param to keep.
	token := mintJWT("default", farFuture)
	rec := doAuth(s, http.MethodGet, "/default-app/8080/api?token="+token+"&keep=1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotHdr != "" || gotQuery != "" {
		t.Errorf("backend saw token (hdr=%q query=%q), want both empty (stripped)", gotHdr, gotQuery)
	}
}

// For the RRT built-in port the ?token= query is forwarded (RRT re-authenticates).
func TestAuthQueryTokenForwardedToRRTPort(t *testing.T) {
	var gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("token")
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	token := mintJWT("default", farFuture)
	rec := doAuth(s, http.MethodGet, "/default-rtt/50090/healthz?token="+token, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotQuery != token {
		t.Errorf("backend query token = %q, want forwarded to RRT port", gotQuery)
	}
}

// With auth disabled the router preserves its original passthrough: no token is
// required and X-Auth, if present, is forwarded untouched.
func TestAuthDisabledPassesThrough(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get(jwtauth.HeaderXAuth)
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)}) // auth off by default
	rec := doAuth(s, http.MethodGet, "/default-app/8080/api", "passthrough-token")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotAuth != "passthrough-token" {
		t.Errorf("backend X-Auth = %q, want untouched passthrough", gotAuth)
	}
}

// ── policy: user-forwarded ports and tunnel are public; RRT is gated ────────

// A user-forwarded service port is PUBLIC: a request with no token is proxied
// (not 401). Port forwarding exists to expose a service, so requiring the
// platform JWT would defeat it.
func TestAuthUserPortPublicNoToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuth(s, http.MethodGet, "/default-app/8080/api", "") // no token
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (user-forwarded port is public)", rec.Code)
	}
}

// The reverse-tunnel WS port is public at the router layer: SDK tunnel defaults
// to ws:// and intentionally sends no platform JWT on that hop.
func TestAuthTunnelPortMissingTokenIsAllowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuthProto(s, http.MethodGet, "/default-rtt/8765/healthz", "", "https")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (tunnel is public)", rec.Code)
	}
}

// The frontend keeps /tunnel unauthenticated for compatibility. Its local
// source marker must not accidentally turn the public tunnel into a tenant-
// authenticated control route.
func TestFrontendAuthenticatedTunnelRemainsPublic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)})
	s.SetAuth(true, false, 50090, 8765)
	req := httptest.NewRequest(http.MethodGet, "/default-rtt/8765/", nil)
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set(internalSrcHeader, internalSrcAuthenticatedValue)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (frontend tunnel remains public)", rec.Code)
	}
}

// A valid token for the tunnel port is accepted but stripped before proxying,
// like user service ports; the tunnel server must not see platform credentials.
func TestAuthTunnelPortValidTokenOK(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get(jwtauth.HeaderXAuth)
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	tg := targetTo(t, upstream.URL)
	tg.Tenant = "default"
	s := New(fakeResolver{target: tg})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuth(s, http.MethodGet, "/default-rtt/8765/healthz", mintJWT("default", farFuture))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if gotAuth != "" {
		t.Errorf("backend X-Auth = %q, want stripped for tunnel port", gotAuth)
	}
}

// doAuthProto is doAuth plus an X-Forwarded-Proto header (TLS-hop signal).
func doAuthProto(s *Server, method, target, token, proto string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if token != "" {
		req.Header.Set(jwtauth.HeaderXAuth, token)
	}
	if proto != "" {
		req.Header.Set("X-Forwarded-Proto", proto)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// The reverse-tunnel WS over a PLAINTEXT hop is also public.
func TestAuthTunnelPlaintextNoTokenAllowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	s := New(fakeResolver{target: targetTo(t, upstream.URL)})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuthProto(s, http.MethodGet, "/default-rtt/8765/", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (tunnel plaintext exempt from JWT)", rec.Code)
	}
}

// The rrt control port is never exempt: plaintext + no token is still 401.
func TestAuthRRTPlaintextNoTokenStill401(t *testing.T) {
	s := New(fakeResolver{target: targetTo(t, "http://127.0.0.1:1")})
	s.SetAuth(true, false, 50090, 8765)
	rec := doAuthProto(s, http.MethodGet, "/default-rtt/50090/healthz", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (rrt port always requires token)", rec.Code)
	}
}
