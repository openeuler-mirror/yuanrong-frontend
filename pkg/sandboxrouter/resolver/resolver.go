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

// Package resolver maps a RouteKey to a backend target. The first-phase
// resolver derives routes from InstanceInfo watch events; a future Redis-backed
// resolver can implement the same interface.
package resolver

import (
	"context"

	"frontend/pkg/sandboxrouter/route"
)

// RouteResolver resolves a RouteKey to its backend target. ctx is honoured by
// resolvers that do I/O (e.g. a future Redis resolver); the watch+cache
// resolver answers from memory and ignores it.
type RouteResolver interface {
	Start(stopCh <-chan struct{}) error
	Resolve(ctx context.Context, key route.RouteKey) (*route.RouteTarget, error)
}
