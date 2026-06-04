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

import "testing"

// TestParsePathValid covers the entry format /{safeID}/{port}[/path].
// The port segment is the sandbox containerPort, matching the Traefik
// PathPrefix(`/{safeID}/{containerPort}`) rule.
func TestParsePathValid(t *testing.T) {
	cases := []struct {
		name         string
		path         string
		wantID       string
		wantPort     uint16
		wantStripped string
	}{
		{"no trailing", "/inst-a/8080", "inst-a", 8080, "/"},
		{"trailing slash", "/inst-a/8080/", "inst-a", 8080, "/"},
		{"sub path", "/inst-a/8080/api/foo", "inst-a", 8080, "/api/foo"},
		{"deep path", "/inst-a/8080/a/b/c", "inst-a", 8080, "/a/b/c"},
		{"sanitized id with dashes", "/user-at-host-v-1/443", "user-at-host-v-1", 443, "/"},
		{"max port", "/inst/65535", "inst", 65535, "/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParsePath(c.path)
			if err != nil {
				t.Fatalf("ParsePath(%q) unexpected error: %v", c.path, err)
			}
			if got.Key.SafeInstanceID != c.wantID || got.Key.Port != c.wantPort {
				t.Errorf("ParsePath(%q) key = %+v, want {%q,%d}", c.path, got.Key, c.wantID, c.wantPort)
			}
			if got.StrippedPath != c.wantStripped {
				t.Errorf("ParsePath(%q) stripped = %q, want %q", c.path, got.StrippedPath, c.wantStripped)
			}
		})
	}
}

// TestParsePathInvalid: anything that can't resolve to a real (id,port) is an
// error; the proxy maps these to 404 to stay consistent with Traefik.
func TestParsePathInvalid(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"root", "/"},
		{"empty", ""},
		{"id only", "/inst-a"},
		{"non numeric port", "/inst-a/not-port"},
		{"port zero", "/inst-a/0"},
		{"port overflow", "/inst-a/70000"},
		{"empty id segment", "//8080"},
		{"empty port segment", "/inst-a//foo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParsePath(c.path); err == nil {
				t.Errorf("ParsePath(%q) expected error, got nil", c.path)
			}
		})
	}
}
