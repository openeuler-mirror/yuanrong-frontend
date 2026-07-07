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

// Package jwtauth provides JWT authentication utilities
package jwtauth

import (
	"container/list"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	httputilclient "frontend/pkg/common/httputil/http/client"
	"frontend/pkg/frontend/config"
)

const (
	// HeaderXAuth is the header key for JWT authentication
	HeaderXAuth = "X-Auth"
	// RoleDeveloper is the developer role
	RoleDeveloper = "developer"
	// RoleUser is the user role
	RoleUser = "user"
)

// JWTHeader represents the JWT header structure
type JWTHeader struct {
	Alg string `json:"alg"` // Algorithm: HMAC-SHA256
	Typ string `json:"typ"` // Type: JWT
}

// JWTPayload represents the JWT payload structure
type JWTPayload struct {
	Sub  string `json:"sub"`            // Subject: tenant ID (required)
	Exp  int64  `json:"exp"`            // Expiration: unix timestamp, or -1/0 for non-expiring IAM tokens
	Role string `json:"role,omitempty"` // Role: user role (optional)
}

// ParsedJWT contains the parsed JWT components
type ParsedJWT struct {
	Header    *JWTHeader
	Payload   *JWTPayload
	Signature string
}

// IsExpired reports whether the payload has expired at the provided time.
// Non-positive exp values are treated as non-expiring tokens.
func (payload *JWTPayload) IsExpired(now time.Time) bool {
	if payload == nil || payload.Exp <= 0 {
		return false
	}
	return now.Unix() > payload.Exp
}

// ParseJWT parses the X-Auth header value which is in the format Header.Payload.Signature
// All parts are base64 encoded
func ParseJWT(authHeader string) (*ParsedJWT, error) {
	if authHeader == "" {
		return nil, fmt.Errorf("X-Auth header is empty")
	}
	// Split by dot to get three parts
	parts := strings.Split(authHeader, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected Header.Payload.Signature, got %d parts", len(parts))
	}
	// Decode header
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		// Try standard base64 encoding if URL encoding fails
		headerBytes, err = base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("failed to decode header: %v", err)
		}
	}
	var header JWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal header: %v", err)
	}
	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Try standard base64 encoding if URL encoding fails
		payloadBytes, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to decode payload: %v", err)
		}
	}
	var payload JWTPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %v", err)
	}
	log.GetLogger().Debugf("function submitter is %s, role is %s", payload.Sub, payload.Role)
	return &ParsedJWT{
		Header:    &header,
		Payload:   &payload,
		Signature: parts[2],
	}, nil
}

// ValidateTenantID checks if the tenant ID in the payload matches the expected tenant ID
func (jwt *ParsedJWT) ValidateTenantID(expectedTenantID string) error {
	if jwt.Payload.Sub == "" {
		return fmt.Errorf("subject (sub) is required but missing in JWT payload")
	}
	if jwt.Payload.Sub != expectedTenantID {
		return fmt.Errorf("tenant ID mismatch: expected %s, got %s", expectedTenantID, jwt.Payload.Sub)
	}
	return nil
}

// iamCacheTTL is the time-to-live for cached IAM validation results.
const iamCacheTTL = 30 * time.Second

// iamValidationCacheSize is the maximum number of successfully validated tokens cached locally.
const iamValidationCacheSize = 4096

type iamValidationLRUCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
}

type iamValidationCacheEntry struct {
	token       string
	validatedAt time.Time
}

func newIAMValidationLRUCache(capacity int, ttl time.Duration) *iamValidationLRUCache {
	return &iamValidationLRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (c *iamValidationLRUCache) get(token string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[token]
	if !ok {
		return false
	}
	entry := element.Value.(*iamValidationCacheEntry)
	if now.Sub(entry.validatedAt) >= c.ttl {
		c.order.Remove(element)
		delete(c.items, token)
		return false
	}
	c.order.MoveToFront(element)
	return true
}

func (c *iamValidationLRUCache) add(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.items[token]; ok {
		entry := element.Value.(*iamValidationCacheEntry)
		entry.validatedAt = time.Now()
		c.order.MoveToFront(element)
		return
	}
	entry := &iamValidationCacheEntry{
		token:       token,
		validatedAt: time.Now(),
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
	delete(c.items, last.Value.(*iamValidationCacheEntry).token)
}

func (c *iamValidationLRUCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// iamFlight represents an in-flight IAM validation request (singleflight).
type iamFlight struct {
	wg  sync.WaitGroup
	err error
}

var (
	iamValidationCache = newIAMValidationLRUCache(iamValidationCacheSize, iamCacheTTL)

	iamFlightMu sync.Mutex
	iamFlights  = make(map[string]*iamFlight)
)

// iamValidator is the function that performs the actual IAM server call.
// Replaceable in tests.
var iamValidator = doValidateWithIamServer

// ValidateWithIamServer validates a token with the IAM server,
// using a short-lived cache and singleflight to avoid redundant calls.
func ValidateWithIamServer(authHeader string, traceID string) error {
	iamServerAddress := config.GetConfig().IamConfig.Addr
	if iamServerAddress == "" {
		log.GetLogger().Warnf("IAM server address is not configured, skipping IAM validation, traceID %s", traceID)
		return nil
	}

	// 1. Check cache
	if iamValidationCache.get(authHeader, time.Now()) {
		return nil
	}

	// 2. Singleflight: if another goroutine is already validating this token, wait for it
	iamFlightMu.Lock()
	if f, ok := iamFlights[authHeader]; ok {
		iamFlightMu.Unlock()
		f.wg.Wait()
		return f.err
	}
	f := &iamFlight{}
	f.wg.Add(1)
	iamFlights[authHeader] = f
	iamFlightMu.Unlock()

	// 3. Call IAM server
	err := iamValidator(authHeader, traceID)

	// 4. Cache successful result
	if err == nil {
		iamValidationCache.add(authHeader)
	}

	// 5. Complete singleflight
	f.err = err
	f.wg.Done()

	iamFlightMu.Lock()
	delete(iamFlights, authHeader)
	iamFlightMu.Unlock()

	return err
}

// doValidateWithIamServer performs the actual HTTP call to the IAM server.
func doValidateWithIamServer(authHeader string, traceID string) error {
	iamServerAddress := config.GetConfig().IamConfig.Addr
	url := "http://" + strings.TrimSuffix(iamServerAddress, "/") + "/iam-server/v1/token/auth"
	client := httputilclient.GetInstance()
	headers := map[string]string{
		HeaderXAuth:            authHeader,
		constant.HeaderTraceID: traceID,
	}
	response, err := client.Get(url, headers)
	if err != nil {
		return fmt.Errorf("failed to send request to IAM server: %v, url is %s", err, url)
	}
	if response == nil {
		return fmt.Errorf("IAM server returned non-200 status code")
	}
	return nil
}

// resetIamCache clears the IAM validation cache. Used in tests.
func resetIamCache() {
	iamValidationCache.reset()

	iamFlightMu.Lock()
	iamFlights = make(map[string]*iamFlight)
	iamFlightMu.Unlock()
}
