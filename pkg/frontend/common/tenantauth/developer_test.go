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

package tenantauth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/types"
)

func tenantAuthTestJWT(t *testing.T, sub, role string) string {
	t.Helper()
	headerBytes, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal jwt header: %v", err)
	}
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"sub":  sub,
		"role": role,
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal jwt payload: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payloadBytes) + ".signature"
}

func configureTenantAuthTestIAM(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	iam := httptest.NewServer(handler)
	prevConfig := *config.GetConfig()
	prevCache := developerJWTCache
	config.SetConfig(types.Config{
		IamConfig: types.IamConfig{
			Addr: strings.TrimPrefix(iam.URL, "http://"),
		},
	})
	developerJWTCache = newValidatedDeveloperJWTCache(32, validatedDeveloperJWTCacheTTL)
	t.Cleanup(func() {
		iam.Close()
		config.SetConfig(prevConfig)
		developerJWTCache = prevCache
	})
}

func TestAuthenticateDeveloperTokenCachesSuccessfulIAMValidation(t *testing.T) {
	validateCalls := 0
	configureTenantAuthTestIAM(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iam-server/v1/token/auth" {
			t.Fatalf("unexpected IAM path %q", r.URL.Path)
		}
		validateCalls++
		w.WriteHeader(http.StatusOK)
	})

	token := tenantAuthTestJWT(t, "user1", jwtauth.RoleDeveloper)
	for i := 0; i < 2; i++ {
		identity, status, err := AuthenticateDeveloperToken(token, "trace")
		if err != nil {
			t.Fatalf("request %d AuthenticateDeveloperToken error: %v", i, err)
		}
		if status != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i, status)
		}
		if identity.TenantID != "user1" || identity.Role != jwtauth.RoleDeveloper {
			t.Fatalf("request %d unexpected identity: %+v", i, identity)
		}
	}
	if validateCalls != 1 {
		t.Fatalf("IAM validation calls = %d, want 1", validateCalls)
	}
}

func TestAuthenticateDeveloperTokenRejectsNonDeveloperRole(t *testing.T) {
	configureTenantAuthTestIAM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, status, err := AuthenticateDeveloperToken(tenantAuthTestJWT(t, "user1", jwtauth.RoleUser), "trace")
	if err == nil {
		t.Fatal("expected non-developer role to be rejected")
	}
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}
