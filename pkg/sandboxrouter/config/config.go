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

// Package config holds the sandbox router configuration.
package config

// SandboxRouterConfig configures the sandbox router. It is an optional section
// of the frontend config; when absent the router starts with default settings; set Enabled=false to disable it explicitly.
type SandboxRouterConfig struct {
	Enabled          bool   `json:"enabled" valid:"optional"`
	ListenIP         string `json:"listenIP" valid:"optional"`
	ListenPort       int    `json:"listenPort" valid:"optional"`
	RouteBackend     string `json:"routeBackend" valid:"optional"` // instanceinfo-watch (default); redis reserved for later
	ResolveTimeoutMs int    `json:"resolveTimeoutMs" valid:"optional"`

	IdleTimeoutSeconds int `json:"idleTimeoutSeconds" valid:"optional"`

	EnableTLSClientAuth bool   `json:"enableTLSClientAuth" valid:"optional"`
	TLSCAFile           string `json:"tlsCAFile" valid:"optional"`
	TLSCertFile         string `json:"tlsCertFile" valid:"optional"`
	TLSKeyFile          string `json:"tlsKeyFile" valid:"optional"`

	// Router-side JWT auth: when EnableJWTAuth, every request must carry a valid
	// JWT in X-Auth (authenticated, and authorized against the target sandbox's
	// tenant). ValidateIAM adds an IAM-server round-trip per request that
	// actually verifies the token; WITHOUT it the router only decodes the JWT
	// locally and checks expiry — it does NOT verify the signature, so a forged
	// token passes. Local-only mode is for clusters with token auth disabled
	// (e.g. AIO test); production MUST set ValidateIAM=true. RRTPort is the RRT
	// built-in atomic-ops container port: X-Auth is forwarded to it (RRT
	// re-authenticates) but stripped for user service ports so they never see
	// the platform token.
	EnableJWTAuth bool `json:"enableJWTAuth" valid:"optional"`
	ValidateIAM   bool `json:"validateIAM" valid:"optional"`
	RRTPort       int  `json:"rrtPort" valid:"optional"`
	// TunnelPort is the reverse-tunnel WS control port (defaults to 8765, the
	// SDK's TUNNEL_WS_PORT). Like RRTPort it requires a token; all other
	// (user-forwarded) ports are public.
	TunnelPort int `json:"tunnelPort" valid:"optional"`

	// Backend TLS for https-protocol sandbox ports (equivalent of FunctionMaster's
	// serversTransport). Defaults mirror the platform's yr-backend-tls@file, which
	// currently uses insecureSkipVerify; set BackendTLSVerify to opt into verification.
	BackendTLSVerify     bool   `json:"backendTLSVerify" valid:"optional"`   // false = skip verify (default)
	BackendTLSCAFile     string `json:"backendTLSCAFile" valid:"optional"`   // verify backend against this CA
	BackendTLSCertFile   string `json:"backendTLSCertFile" valid:"optional"` // client cert to backend (mTLS)
	BackendTLSKeyFile    string `json:"backendTLSKeyFile" valid:"optional"`
	BackendTLSServerName string `json:"backendTLSServerName" valid:"optional"` // SNI override
}

// Defaults for unset fields.
const (
	defaultListenIP           = "0.0.0.0"
	defaultListenPort         = 8080
	defaultRouteBackend       = "instanceinfo-watch"
	defaultResolveTimeoutMs   = 500
	defaultIdleTimeoutSeconds = 610
	defaultTunnelPort         = 8765
)

// ApplyDefaults fills zero-valued fields with their defaults, leaving any
// explicitly set value untouched.
func (c *SandboxRouterConfig) ApplyDefaults() {
	if c.ListenIP == "" {
		c.ListenIP = defaultListenIP
	}
	if c.ListenPort == 0 {
		c.ListenPort = defaultListenPort
	}
	if c.RouteBackend == "" {
		c.RouteBackend = defaultRouteBackend
	}
	if c.ResolveTimeoutMs == 0 {
		c.ResolveTimeoutMs = defaultResolveTimeoutMs
	}
	if c.IdleTimeoutSeconds == 0 {
		c.IdleTimeoutSeconds = defaultIdleTimeoutSeconds
	}
	if c.TunnelPort == 0 {
		c.TunnelPort = defaultTunnelPort
	}
}
