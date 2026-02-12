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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	httputilclient "frontend/pkg/common/httputil/http/client"
	"frontend/pkg/frontend/config"
	"strings"
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
	Exp  int64  `json:"exp"`            // Expiration: timestamp (required)
	Role string `json:"role,omitempty"` // Role: user role (optional)
}

// ParsedJWT contains the parsed JWT components
type ParsedJWT struct {
	Header    *JWTHeader
	Payload   *JWTPayload
	Signature string
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

// ValidateWithIamServer sends a request to IAM server to validate the token
func ValidateWithIamServer(authHeader string, traceID string) error {
	iamServerAddress := config.GetConfig().IamConfig.Addr
	if iamServerAddress == "" {
		log.GetLogger().Warnf("IAM server address is not configured, skipping IAM validation, traceID %s", traceID)
		return nil
	}
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
