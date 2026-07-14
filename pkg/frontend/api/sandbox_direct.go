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

package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/tracer"
	frontendhttputil "frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/middleware"
	routerconfig "frontend/pkg/sandboxrouter/config"
)

const (
	sandboxDirectPrefix      = "/direct"
	sandboxTunnelPrefix      = "/tunnel"
	sandboxInternalSrcKey    = "X-Internal-Src"
	sandboxInternalSrcValue  = "1"
	sandboxInternalTenantKey = "X-Internal-Tenant"
	sandboxDefaultRRTPort    = 50090
)

// registerSandboxDirectRoute exposes the RRT direct-invoke route on the normal
// frontend endpoint. The request has already passed frontend JWT middleware;
// this proxy strips the platform token before forwarding to the local
// sandboxRouter listener so RRT never sees frontend credentials.
func registerSandboxDirectRoute(r *gin.Engine) {
	r.Any(sandboxDirectPrefix, sandboxTraceHandler(sandboxDirectProxyHandler(sandboxDirectRouterPath, true)))
	r.Any(sandboxDirectPrefix+"/*proxyPath", sandboxTraceHandler(sandboxDirectProxyHandler(sandboxDirectRouterPath, true)))
	r.Any(sandboxTunnelPrefix, sandboxDirectProxyHandler(sandboxTunnelRouterPath, false))
	r.Any(sandboxTunnelPrefix+"/*proxyPath", sandboxDirectProxyHandler(sandboxTunnelRouterPath, false))
}

func sandboxTraceHandler(handler gin.HandlerFunc) gin.HandlerFunc {
	wrapped := tracer.WrapGinHandler(handler)
	return func(c *gin.Context) {
		traceID := frontendhttputil.InitTraceID(c)
		c.Header(constant.HeaderTraceID, traceID)
		wrapped(c)
	}
}

func sandboxDirectRouterPath(path string, cfg *routerconfig.SandboxRouterConfig) string {
	routerPath := strings.TrimPrefix(path, sandboxDirectPrefix)
	if routerPath == "" {
		return "/"
	}
	trimmed := strings.TrimPrefix(routerPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || parts[0] == "" {
		return "/"
	}
	if _, err := strconv.ParseUint(parts[1], 10, 16); err == nil {
		return "/" // explicit ports are not allowed on the RRT control-plane route
	}

	switch parts[1] {
	case "invoke", "healthz", "upload", "download":
		return sandboxDirectControlPath(parts[0], sandboxDirectRRTPort(cfg), parts[1:])
	default:
		return "/"
	}
}

func sandboxTunnelRouterPath(path string, cfg *routerconfig.SandboxRouterConfig) string {
	routerPath := strings.TrimPrefix(path, sandboxTunnelPrefix)
	if routerPath == "" {
		return "/"
	}
	trimmed := strings.TrimPrefix(routerPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return routerPath
	}
	if len(parts) >= 2 {
		if _, err := strconv.ParseUint(parts[1], 10, 16); err == nil {
			return routerPath // compatibility for /tunnel/{safeID}/{port}/...
		}
	}
	return sandboxDirectControlPath(parts[0], cfg.TunnelPort, parts[1:])
}

func sandboxDirectControlPath(safeID string, port int, tail []string) string {
	if len(tail) == 0 {
		return fmt.Sprintf("/%s/%d", safeID, port)
	}
	return fmt.Sprintf("/%s/%d/%s", safeID, port, strings.Join(tail, "/"))
}

func sandboxDirectRRTPort(cfg *routerconfig.SandboxRouterConfig) int {
	if cfg.RRTPort != 0 {
		return cfg.RRTPort
	}
	return sandboxDefaultRRTPort
}

func sandboxDirectProxyHandler(
	pathMapper func(string, *routerconfig.SandboxRouterConfig) string,
	requireAuthenticatedTenant bool,
) gin.HandlerFunc {
	cfg := config.GetConfig().SandboxRouter
	if cfg == nil || !cfg.Enabled {
		return func(c *gin.Context) {
			c.String(http.StatusServiceUnavailable, "sandbox router is disabled")
		}
	}
	cfg.ApplyDefaults()
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", cfg.ListenPort))
	if err != nil {
		return func(c *gin.Context) {
			c.String(http.StatusInternalServerError, "invalid sandbox router target")
		}
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			in := pr.In
			forwardedProto := in.Header.Get("X-Forwarded-Proto")
			authenticatedTenant := in.Header.Get(sandboxInternalTenantKey)
			pr.SetURL(target)
			pr.Out.URL.Path = pathMapper(in.URL.Path, cfg)
			pr.Out.URL.RawPath = ""
			pr.Out.URL.RawQuery = in.URL.RawQuery
			if q := pr.Out.URL.Query(); q.Has("token") || q.Has("tenant_id") {
				q.Del("token")
				// JWT middleware injects tenant_id into the frontend request query for
				// API handlers. The /direct hop is already authenticated at frontend
				// and must not leak platform routing/auth params to sandboxRouter/RRT.
				q.Del("tenant_id")
				pr.Out.URL.RawQuery = q.Encode()
			}
			pr.SetXForwarded()
			pr.Out.Header.Del(jwtauth.HeaderXAuth)
			pr.Out.Header.Set(sandboxInternalSrcKey, sandboxInternalSrcValue)
			pr.Out.Header.Del(sandboxInternalTenantKey)
			if authenticatedTenant != "" {
				pr.Out.Header.Set(sandboxInternalTenantKey, authenticatedTenant)
			}
			if forwardedProto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", forwardedProto)
			}
		},
	}

	return func(c *gin.Context) {
		// Never trust an internal tenant marker supplied by the client. Rebuild it
		// exclusively from the JWT identity stored by the validated middleware.
		c.Request.Header.Del(sandboxInternalTenantKey)
		if tenant, ok := middleware.JWTAuthenticatedTenant(c); ok {
			c.Request.Header.Set(sandboxInternalTenantKey, tenant)
		} else if requireAuthenticatedTenant && config.GetConfig().IamConfig.EnableFuncTokenAuth {
			c.String(http.StatusForbidden, "authenticated tenant is required")
			return
		}
		started := time.Now()
		routerPath := pathMapper(c.Request.URL.Path, cfg)
		proxy.ServeHTTP(c.Writer, c.Request)
		log.GetLogger().Infof(
			"sandbox direct proxy method(%s) path(%s) routerPath(%s) status(%d) size(%d) durationMs(%d) traceID(%s)",
			c.Request.Method,
			c.Request.URL.Path,
			routerPath,
			c.Writer.Status(),
			c.Writer.Size(),
			time.Since(started).Milliseconds(),
			c.Request.Header.Get(constant.HeaderTraceID),
		)
	}
}
