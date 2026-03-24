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
	"strings"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/constants"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

// isBrowserRequest checks if the request is from a browser
func isBrowserRequest(ctx *gin.Context) bool {
	accept := ctx.GetHeader("Accept")
	return strings.Contains(accept, "text/html")
}

// respondAuthError responds with appropriate error based on client type
func respondAuthError(ctx *gin.Context, message string) {
	if isBrowserRequest(ctx) {
		// Get path prefix for correct form action
		pathPrefix := ctx.GetHeader("X-Forwarded-Prefix")

		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Authentication Required</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            max-width: 450px;
            width: 100%%;
            padding: 40px;
        }
        .icon {
            font-size: 64px;
            text-align: center;
            margin-bottom: 20px;
        }
        h1 {
            font-size: 24px;
            color: #2d3748;
            margin-bottom: 12px;
            text-align: center;
        }
        .message {
            color: #e53e3e;
            background: #fff5f5;
            padding: 12px 16px;
            border-radius: 6px;
            margin-bottom: 24px;
            font-size: 14px;
            border-left: 4px solid #e53e3e;
        }
        .form-group {
            margin-bottom: 20px;
        }
        .form-label {
            display: block;
            margin-bottom: 8px;
            font-weight: 500;
            color: #4a5568;
            font-size: 14px;
        }
        .form-input {
            width: 100%%;
            padding: 10px 12px;
            border: 1px solid #cbd5e0;
            border-radius: 6px;
            font-size: 14px;
            font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
        }
        .form-input:focus {
            outline: none;
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }
        .btn {
            width: 100%%;
            padding: 12px;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            border: none;
            border-radius: 6px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .btn:hover {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }
        .help-text {
            margin-top: 16px;
            text-align: center;
            font-size: 13px;
            color: #718096;
        }
        .back-link {
            display: inline-block;
            margin-top: 16px;
            text-align: center;
            width: 100%%;
            color: #667eea;
            text-decoration: none;
            font-size: 14px;
        }
        .back-link:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">🔒</div>
        <h1>Authentication Required</h1>
        <div class="message">%s</div>
        
        <form id="authForm" onsubmit="submitToken(event)">
            <div class="form-group">
                <label class="form-label">Enter JWT Token</label>
                <input type="text" class="form-input" id="tokenInput" placeholder="eyJhbGciOiJIUzI1NiIs..." required>
            </div>
            <button type="submit" class="btn">Verify &amp; Continue</button>
        </form>
        
        <div class="help-text">
            Token can be obtained via the CLI tool's token-require command
        </div>
        <a href="%s/" class="back-link">← Back to Home</a>
    </div>
    
    <script>
        function submitToken(event) {
            event.preventDefault();
            const token = document.getElementById('tokenInput').value.trim();
            if (token) {
                // Append token to current URL and reload
                const url = new URL(window.location.href);
                url.searchParams.set('token', token);
                window.location.href = url.toString();
            }
        }
    </script>
</body>
</html>`, message, pathPrefix)

		ctx.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(html))
	} else {
		// API client: return JSON error
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": message,
		})
	}
	ctx.Abort()
}

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

		// Get JWT token from X-Auth header or token query parameter
		authHeader := ctx.Request.Header.Get(jwtauth.HeaderXAuth)
		if authHeader == "" {
			// Try to get token from query parameter
			authHeader = ctx.Query("token")
		}
		if authHeader == "" {
			if cookie, err := ctx.Cookie("iam_token"); err == nil {
				authHeader = cookie
			}
		}
		if authHeader == "" {
			respondAuthError(ctx, "Authentication failed: no token provided. Please enter a valid JWT token.")
			return
		}

		// Parse JWT to get role
		parsedJWT, err := jwtauth.ParseJWT(authHeader)
		if err != nil {
			log.GetLogger().Errorf("JWT parsing failed, traceID %s: %v", traceID, err)
			respondAuthError(ctx, fmt.Sprintf("Authentication failed: invalid or malformed token (%v)", err))
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
			respondAuthError(ctx, fmt.Sprintf("Authentication failed: role %s is not authorized to access this resource", role))
			return
		}

		log.GetLogger().Debugf("JWT authentication passed, role: %s, traceID %s", role, traceID)

		// Validate with IAM server
		if err := jwtauth.ValidateWithIamServer(authHeader, traceID); err != nil {
			log.GetLogger().Errorf("IAM server validation failed, traceID %s: %v", traceID, err)
			respondAuthError(ctx, fmt.Sprintf("Authentication failed: IAM server validation failed (%v)", err))
			return
		}

		log.GetLogger().Debugf("IAM server validation passed, traceID %s", traceID)

		// If JWT token contains tenant information, replace the tenant in header
		if parsedJWT.Payload.Sub != "" {
			tenantFromJWT := parsedJWT.Payload.Sub

			// Replace both X-Tenant-ID and X-Tenant-Id headers with the tenant from JWT
			ctx.Request.Header.Set(constants.HeaderTenantID, tenantFromJWT)
			ctx.Request.Header.Set(constants.HeaderTenantId, tenantFromJWT)

			// Replace tenant_id query parameter in URL with tenant from JWT
			queryValues := ctx.Request.URL.Query()
			queryValues.Set("tenant_id", tenantFromJWT)
			ctx.Request.URL.RawQuery = queryValues.Encode()

			log.GetLogger().Debugf("Replaced tenant in header/query with JWT tenant: %s, traceID %s", tenantFromJWT, traceID)
		}

		// Store user info in context for downstream handlers
		ctx.Set("jwt_role", role)

		ctx.Next()
	}
}
