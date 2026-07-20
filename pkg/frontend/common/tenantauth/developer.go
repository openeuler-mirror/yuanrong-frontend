/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
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

// Package tenantauth authenticates developer JWT tokens and caches validated identities.
package tenantauth

import (
	"container/list"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"frontend/pkg/frontend/common/jwtauth"
)

const (
	// SystemTenantID identifies the tenant with system-wide privileges.
	SystemTenantID = "0"

	validatedDeveloperJWTCacheSize = 1024
	validatedDeveloperJWTCacheTTL  = 30 * time.Second
)

var developerJWTCache = newValidatedDeveloperJWTCache(validatedDeveloperJWTCacheSize, validatedDeveloperJWTCacheTTL)

// DeveloperIdentity is the authenticated tenant and role extracted from a developer JWT.
type DeveloperIdentity struct {
	TenantID       string
	Role           string
	IsSystemTenant bool
}

type validatedDeveloperJWTCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
}

type validatedDeveloperJWTCacheEntry struct {
	token       string
	identity    DeveloperIdentity
	validatedAt time.Time
	expiresAt   int64
}

func newValidatedDeveloperJWTCache(capacity int, ttl time.Duration) *validatedDeveloperJWTCache {
	return &validatedDeveloperJWTCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (c *validatedDeveloperJWTCache) get(token string, now time.Time) (DeveloperIdentity, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[token]
	if !ok {
		return DeveloperIdentity{}, false
	}
	entry, ok := element.Value.(*validatedDeveloperJWTCacheEntry)
	if !ok {
		c.order.Remove(element)
		delete(c.items, token)
		return DeveloperIdentity{}, false
	}
	if now.Sub(entry.validatedAt) >= c.ttl || (&jwtauth.JWTPayload{Exp: entry.expiresAt}).IsExpired(now) {
		c.order.Remove(element)
		delete(c.items, token)
		return DeveloperIdentity{}, false
	}
	c.order.MoveToFront(element)
	return entry.identity, true
}

func (c *validatedDeveloperJWTCache) add(token string, identity DeveloperIdentity, expiresAt int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.items[token]; ok {
		if entry, ok := element.Value.(*validatedDeveloperJWTCacheEntry); ok {
			entry.identity = identity
			entry.expiresAt = expiresAt
			entry.validatedAt = time.Now()
			c.order.MoveToFront(element)
			return
		}
		c.order.Remove(element)
		delete(c.items, token)
	}
	entry := &validatedDeveloperJWTCacheEntry{
		token:       token,
		identity:    identity,
		validatedAt: time.Now(),
		expiresAt:   expiresAt,
	}
	element := c.order.PushFront(entry)
	c.items[token] = element
	if c.order.Len() <= c.capacity {
		return
	}
	last := c.order.Back()
	if last == nil {
		return
	}
	c.order.Remove(last)
	if entry, ok := last.Value.(*validatedDeveloperJWTCacheEntry); ok {
		delete(c.items, entry.token)
	}
}

// AuthenticateDeveloperToken validates a developer JWT and returns its caller identity.
func AuthenticateDeveloperToken(token string, traceID string) (DeveloperIdentity, int, error) {
	if token == "" {
		return DeveloperIdentity{}, http.StatusForbidden, errors.New("missing JWT token")
	}
	if identity, ok := developerJWTCache.get(token, time.Now()); ok {
		return identity, http.StatusOK, nil
	}

	parsed, err := jwtauth.ParseJWT(token)
	if err != nil {
		return DeveloperIdentity{}, http.StatusUnauthorized, fmt.Errorf("invalid JWT token: %w", err)
	}
	if parsed.Payload == nil || parsed.Payload.Sub == "" {
		return DeveloperIdentity{}, http.StatusForbidden, errors.New("missing caller tenant in JWT token")
	}
	if parsed.Payload.IsExpired(time.Now()) {
		return DeveloperIdentity{}, http.StatusUnauthorized, errors.New("JWT token is expired")
	}
	if err := jwtauth.ValidateWithIamServer(token, traceID); err != nil {
		return DeveloperIdentity{}, http.StatusUnauthorized, fmt.Errorf("IAM server validation failed: %w", err)
	}
	if parsed.Payload.Role != jwtauth.RoleDeveloper {
		return DeveloperIdentity{}, http.StatusForbidden, errors.New("caller role is not authorized")
	}

	identity := DeveloperIdentity{
		TenantID:       parsed.Payload.Sub,
		Role:           parsed.Payload.Role,
		IsSystemTenant: parsed.Payload.Sub == SystemTenantID,
	}
	developerJWTCache.add(token, identity, parsed.Payload.Exp)
	return identity, http.StatusOK, nil
}
