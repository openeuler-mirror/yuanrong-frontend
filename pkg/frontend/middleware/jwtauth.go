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

// Package middleware -
package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

// JWTAuthMiddleware validates JWT token from X-Auth header
// Deprecated: Use JWTAuthMiddlewareWithRoles instead
func JWTAuthMiddleware() gin.HandlerFunc {
	return JWTAuthMiddlewareWithRoles([]string{jwtauth.RoleDeveloper})
}

// JWTAuthMiddlewareWithRoles validates JWT token and checks if user role is in allowedRoles
func JWTAuthMiddlewareWithRoles(allowedRoles []string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Skip if JWT authentication is disabled
		if !config.GetConfig().IamConfig.EnableFuncTokenAuth {
			ctx.Next()
			return
		}

		// Get trace ID for logging
		traceID := ctx.Request.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = ctx.Request.Header.Get("X-Request-ID")
		}

		// Get JWT token from X-Auth header
		authHeader := ctx.Request.Header.Get(jwtauth.HeaderXAuth)
		if authHeader == "" {
			// No token provided, allow request to continue (optional authentication)
			ctx.Next()
			return
		}

		// Parse JWT to get role
		parsedJWT, err := jwtauth.ParseJWT(authHeader)
		if err != nil {
			log.GetLogger().Errorf("JWT parsing failed, traceID %s: %v", traceID, err)
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("authentication failed: %v", err),
			})
			ctx.Abort()
			return
		}

		// Check role: verify if role is in allowed roles list
		role := parsedJWT.Payload.Role
		roleAllowed := false
		for _, allowedRole := range allowedRoles {
			if role == allowedRole {
				roleAllowed = true
				break
			}
		}
		if !roleAllowed {
			log.GetLogger().Errorf("JWT role validation failed, role %s is not in allowed roles %v, traceID %s", role, allowedRoles, traceID)
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("authentication failed: role %s is not allowed", role),
			})
			ctx.Abort()
			return
		}

		log.GetLogger().Debugf("JWT authentication passed, role: %s, traceID %s", role, traceID)

		// Validate with IAM server
		if err := jwtauth.ValidateWithIamServer(authHeader, traceID); err != nil {
			log.GetLogger().Errorf("IAM server validation failed, traceID %s: %v", traceID, err)
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("authentication failed: %v", err),
			})
			ctx.Abort()
			return
		}

		log.GetLogger().Infof("IAM server validation passed, traceID %s", traceID)

		// Store user info in context for downstream handlers
		ctx.Set("jwt_role", role)
		ctx.Set("jwt_tenant_id", parsedJWT.Payload.Sub)

		ctx.Next()
	}
}
