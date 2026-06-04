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
	"errors"
	"sync"
)

// ErrRouteNotFound is returned by RouteCache.Get when no route matches the key.
var ErrRouteNotFound = errors.New("route not found")

// cacheEntry records the owning instanceID alongside the target so that
// DeleteInstance only removes keys an instance still owns (matters when two
// instances sanitize to the same RouteKey; last writer wins).
type cacheEntry struct {
	owner  string
	target *RouteTarget
}

// RouteCache holds resolved sandbox routes keyed by RouteKey, tracked per
// instanceID so an instance's routes can be replaced or removed as a unit.
// All methods are safe for concurrent use.
type RouteCache struct {
	mu     sync.RWMutex
	routes map[RouteKey]cacheEntry
	byInst map[string]map[RouteKey]struct{}
}

// NewRouteCache returns an empty cache.
func NewRouteCache() *RouteCache {
	return &RouteCache{
		routes: make(map[RouteKey]cacheEntry),
		byInst: make(map[string]map[RouteKey]struct{}),
	}
}

// Get returns a copy of the target for key, or ErrRouteNotFound.
func (c *RouteCache) Get(key RouteKey) (*RouteTarget, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.routes[key]
	if !ok {
		return nil, ErrRouteNotFound
	}
	return copyTarget(e.target), nil
}

// PutInstance replaces all routes owned by id with ts. Keys the instance no
// longer owns are removed.
func (c *RouteCache) PutInstance(id string, ts []*RouteTarget) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeInstanceLocked(id)
	if len(ts) == 0 {
		return
	}
	keys := make(map[RouteKey]struct{}, len(ts))
	for _, t := range ts {
		c.routes[t.Key] = cacheEntry{owner: id, target: copyTarget(t)}
		keys[t.Key] = struct{}{}
	}
	c.byInst[id] = keys
}

// DeleteInstance removes every route owned by id.
func (c *RouteCache) DeleteInstance(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeInstanceLocked(id)
}

// Size returns the number of routes currently held.
func (c *RouteCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.routes)
}

// removeInstanceLocked drops id's keys, but only those routes id still owns
// (a later PutInstance from another instance may have taken over the key).
func (c *RouteCache) removeInstanceLocked(id string) {
	for key := range c.byInst[id] {
		if e, ok := c.routes[key]; ok && e.owner == id {
			delete(c.routes, key)
		}
	}
	delete(c.byInst, id)
}

// copyTarget deep-copies the target (including the URL) so callers cannot
// mutate cached state through the returned pointer.
func copyTarget(t *RouteTarget) *RouteTarget {
	cp := *t
	if t.TargetURL != nil {
		u := *t.TargetURL
		cp.TargetURL = &u
	}
	return &cp
}
