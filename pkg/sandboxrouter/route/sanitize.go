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

// Package route computes sandbox routing facts from InstanceInfo. SanitizeID
// and ExtractIP replicate the C++ TraefikRouteCache helpers byte-for-byte.
package route

import "strings"

// maxRouterNameLen mirrors C++ MAX_ROUTER_NAME_LEN; safeID is truncated to it.
const maxRouterNameLen = 200

// SanitizeID replicates TraefikRouteCache::SanitizeID: '@' becomes "-at-",
// then '/', '.', '_' each become '-', then the result is truncated (by bytes)
// to maxRouterNameLen. Order matters: '@' expansion happens before truncation.
func SanitizeID(id string) string {
	result := strings.ReplaceAll(id, "@", "-at-")
	result = strings.Map(func(r rune) rune {
		switch r {
		case '/', '.', '_':
			return '-'
		default:
			return r
		}
	}, result)
	if len(result) > maxRouterNameLen {
		result = result[:maxRouterNameLen]
	}
	return result
}

// ExtractIP replicates TraefikRouteCache::ExtractIP: it returns the substring
// before the first ':', or "" when there is no ':'. It does not handle IPv6,
// matching the C++ behaviour exactly.
func ExtractIP(addr string) string {
	if i := strings.IndexByte(addr, ':'); i >= 0 {
		return addr[:i]
	}
	return ""
}
