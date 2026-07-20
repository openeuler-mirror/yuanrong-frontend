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

package config

import "testing"

const (
	testListenPort         = 9090
	testResolveTimeoutMs   = 200
	testIdleTimeoutSeconds = 30
)

func TestApplyDefaultsFillsZeroValues(t *testing.T) {
	c := &SandboxRouterConfig{Enabled: true}
	c.ApplyDefaults()

	if c.ListenIP != "0.0.0.0" {
		t.Errorf("ListenIP = %q, want 0.0.0.0", c.ListenIP)
	}
	if c.ListenPort != 8080 {
		t.Errorf("ListenPort = %d, want 8080", c.ListenPort)
	}
	if c.RouteBackend != "instanceinfo-watch" {
		t.Errorf("RouteBackend = %q, want instanceinfo-watch", c.RouteBackend)
	}
	if c.ResolveTimeoutMs != defaultResolveTimeoutMs {
		t.Errorf("ResolveTimeoutMs = %d, want %d", c.ResolveTimeoutMs, defaultResolveTimeoutMs)
	}
	if c.IdleTimeoutSeconds != defaultIdleTimeoutSeconds {
		t.Errorf("IdleTimeoutSeconds = %d, want %d", c.IdleTimeoutSeconds, defaultIdleTimeoutSeconds)
	}
}

func TestApplyDefaultsKeepsSetValues(t *testing.T) {
	c := &SandboxRouterConfig{
		ListenIP:           "127.0.0.1",
		ListenPort:         testListenPort,
		RouteBackend:       "redis",
		ResolveTimeoutMs:   testResolveTimeoutMs,
		IdleTimeoutSeconds: testIdleTimeoutSeconds,
	}
	c.ApplyDefaults()

	if c.ListenIP != "127.0.0.1" || c.ListenPort != testListenPort || c.RouteBackend != "redis" ||
		c.ResolveTimeoutMs != testResolveTimeoutMs || c.IdleTimeoutSeconds != testIdleTimeoutSeconds {
		t.Errorf("ApplyDefaults overwrote set values: %+v", c)
	}
}
