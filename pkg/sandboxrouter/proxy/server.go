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

// Package proxy is the sandbox router data plane: it parses the inbound
// /{safeID}/{port}/path, resolves a backend, and reverse-proxies to it with
// Traefik-consistent status codes.
package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"

	"frontend/pkg/sandboxrouter/route"
)

// Resolver is the subset of the route resolver the proxy consumes. Declared
// here (consumer side) so the proxy stays free of the etcd-backed resolver
// package and remains unit-testable.
type Resolver interface {
	Resolve(ctx context.Context, key route.RouteKey) (*route.RouteTarget, error)
}

// reqInfo is the per-request routing decision passed to the reverse proxy via
// the request context.
type reqInfo struct {
	parsed *route.ParsedRequest
	target *route.RouteTarget
}

type ctxKey int

const reqInfoKey ctxKey = 0

// Server parses, resolves, and reverse-proxies sandbox traffic.
type Server struct {
	resolver  Resolver
	transport *http.Transport
	proxy     *httputil.ReverseProxy
}

// New builds a Server over the given resolver.
func New(r Resolver) *Server {
	s := &Server{resolver: r, transport: &http.Transport{}}
	s.proxy = &httputil.ReverseProxy{
		Rewrite:      s.rewrite,
		Transport:    roundTripperFunc(func(req *http.Request) (*http.Response, error) { return s.transport.RoundTrip(req) }),
		ErrorHandler: errorHandler,
	}
	return s
}

// ServeHTTP parses the path, resolves the backend, and proxies. Error codes
// follow Traefik: an unmatched route is 404 (never 400); a resolver that is
// erroring (not "not found") is 503; upstream failures are mapped in
// errorHandler to 502/504.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parsed, err := route.ParsePath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r) // 404: no router would match this shape
		return
	}

	target, err := s.resolver.Resolve(r.Context(), parsed.Key)
	if errors.Is(err, route.ErrRouteNotFound) {
		http.NotFound(w, r) // 404
		return
	}
	if err != nil {
		http.Error(w, "route unavailable", http.StatusServiceUnavailable) // 503
		return
	}

	ctx := context.WithValue(r.Context(), reqInfoKey, &reqInfo{parsed: parsed, target: target})
	s.proxy.ServeHTTP(w, r.WithContext(ctx))
}

// rewrite rewrites the outbound request to the resolved target, stripping the
// /{safeID}/{port} prefix and preserving the query string.
func (s *Server) rewrite(pr *httputil.ProxyRequest) {
	info := pr.In.Context().Value(reqInfoKey).(*reqInfo)

	pr.SetURL(info.target.TargetURL)
	pr.Out.URL.Path = info.parsed.StrippedPath
	pr.Out.URL.RawPath = ""
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetXForwarded()
	pr.Out.Header.Set("X-Instance-Id", info.parsed.Key.SafeInstanceID)
	pr.Out.Header.Set("X-Instance-Port", strconv.Itoa(int(info.parsed.Key.Port)))
}

// errorHandler maps upstream transport failures to Traefik-consistent codes:
// timeouts to 504 Gateway Timeout, everything else to 502 Bad Gateway.
func errorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		w.WriteHeader(http.StatusGatewayTimeout) // 504
		return
	}
	w.WriteHeader(http.StatusBadGateway) // 502
}

// roundTripperFunc adapts a function to http.RoundTripper so the proxy reads
// the Server's current transport at call time (lets tests swap it).
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
