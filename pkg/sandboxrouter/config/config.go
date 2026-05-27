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
// of the frontend config; when absent or Enabled=false the router does not start.
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
}

// Defaults for unset fields.
const (
	defaultListenIP           = "0.0.0.0"
	defaultListenPort         = 8080
	defaultRouteBackend       = "instanceinfo-watch"
	defaultResolveTimeoutMs   = 500
	defaultIdleTimeoutSeconds = 610
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
}
