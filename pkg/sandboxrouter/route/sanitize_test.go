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

package route

import (
	"strings"
	"testing"
)

// TestSanitizeID locks the cross-language contract with the C++
// TraefikRouteCache::SanitizeID (functionsystem/.../traefik_route_cache.cpp).
// The mapping MUST be byte-identical, otherwise sandboxrouter and
// FunctionMaster/Traefik compute different safeIDs and routes won't match.
func TestSanitizeID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "abc", "abc"},
		{"empty", "", ""},
		{"single at", "@", "-at-"},
		{"at expands", "a@b", "a-at-b"},
		{"multiple at", "a@b@c", "a-at-b-at-c"},
		{"slash dot underscore", "a/b.c_d", "a-b-c-d"},
		{"mixed", "user@host/path.v_1", "user-at-host-path-v-1"},
		// @ is expanded to -at- BEFORE truncation; 100 '@' -> 400 chars -> cut to 200.
		{"truncate after expansion", strings.Repeat("@", 100), strings.Repeat("-at-", 50)},
		// 250 plain chars truncate to exactly 200.
		{"truncate plain", strings.Repeat("a", 250), strings.Repeat("a", 200)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeID(c.in)
			if got != c.want {
				t.Errorf("SanitizeID(%q) = %q, want %q", c.in, got, c.want)
			}
			if len(got) > 200 {
				t.Errorf("SanitizeID(%q) length = %d, must be <= 200", c.in, len(got))
			}
		})
	}
}

// TestExtractIP locks the contract with C++ TraefikRouteCache::ExtractIP:
// it splits on the FIRST ':' and returns "" when there is no ':'.
func TestExtractIP(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ip and port", "10.0.1.23:22423", "10.0.1.23"},
		{"no colon returns empty", "10.0.1.23", ""},
		{"empty", "", ""},
		{"leading colon", ":8080", ""},
		{"first colon wins", "host:1:2", "host"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractIP(c.in); got != c.want {
				t.Errorf("ExtractIP(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
