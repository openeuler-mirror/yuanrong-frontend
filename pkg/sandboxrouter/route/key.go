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

// RouteKey identifies a sandbox port route. Port is the sandbox containerPort,
// matching both the entry path and the Traefik PathPrefix rule.
type RouteKey struct {
	SafeInstanceID string
	Port           uint16
}

// ParsedRequest is the result of parsing an inbound request path.
type ParsedRequest struct {
	Key          RouteKey
	StrippedPath string
}
