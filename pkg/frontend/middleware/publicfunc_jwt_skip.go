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

package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/aliasroute"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/common"
	"frontend/pkg/frontend/functionmeta"
)

const (
	// skipJWTAuthKey is the context key to indicate JWT authentication should be skipped
	skipJWTAuthKey = "skip_jwt_for_public_function"
	// isInvokeURLKey is the context key to indicate if the request is for invoke URLs
	isInvokeURLKey = "is_invoke_url"
)

// InvokePreprocessMiddleware preprocesses invoke requests:
// 1. Detects if the URL is an invoke URL and sets a flag
// 2. For invoke URLs, checks if the function is public and sets a skip JWT flag
// This middleware should be applied before GlobalJWTAuthMiddleware.
func InvokePreprocessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Check if this is an invoke URL
		if isInvokeURL(path) {

			// Extract function key and check if it's public
			funcKey, err := extractFunctionKey(c)
			if err != nil {
				// If we can't extract function key, let the request continue
				log.GetLogger().Debugf("Failed to extract function key from URL %s: %v", path, err)
				c.Next()
				return
			}

			// If funcKey is empty, this might be a test or invalid URL - let the request continue
			if funcKey == "" {
				log.GetLogger().Debugf("Empty function key extracted from URL %s", path)
				c.Next()
				return
			}

			// Mark as invoke URL for downstream middleware
			c.Set(isInvokeURLKey, true)

			// Load function metadata to check if it's public
			funcSpec, ok := functionmeta.LoadFuncSpec(funcKey)
			if !ok {
				// If we can't load function spec, let the request continue
				log.GetLogger().Debugf("Failed to load function spec for %s", funcKey)
				c.Next()
				return
			}

			// If function is public, set flag to skip JWT authentication
			if funcSpec != nil && funcSpec.FuncMetaData.IsFuncPublic {
				log.GetLogger().Infof("Function %s is public, setting flag to skip JWT authentication", funcKey)
				c.Set(skipJWTAuthKey, true)
			}
		}

		c.Next()
	}
}

// PublicFunctionJWTSkipMiddleware is deprecated, use InvokePreprocessMiddleware instead
func PublicFunctionJWTSkipMiddleware() gin.HandlerFunc {
	return InvokePreprocessMiddleware()
}

// isInvokeURL checks if the path matches invoke URL patterns
func isInvokeURL(path string) bool {
	// Check for standard invoke URL: /serverless/v1/functions/{urn}/invocations
	if strings.HasPrefix(path, "/serverless/v1/functions/") && strings.HasSuffix(path, "/invocations") {
		return true
	}

	// Check for short invoke URL pattern: /invocations/{tenant-id}/{namespace}/{function}/
	if strings.HasPrefix(path, "/invocations/") {
		rest := strings.Trim(strings.TrimPrefix(path, "/invocations/"), "/")
		parts := strings.Split(rest, "/")
		if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
			return true
		}
	}

	// Check for deprecated short invoke URL pattern: /{tenant-id}/{namespace}/{function}/
	// Keep compatibility for legacy clients while avoiding known API prefixes.
	legacy := strings.Trim(path, "/")
	parts := strings.Split(legacy, "/")
	if len(parts) == 3 &&
		parts[0] != "" && parts[1] != "" && parts[2] != "" &&
		parts[0] != "serverless" &&
		parts[0] != "datasystem" &&
		parts[0] != "client" &&
		parts[0] != "frontend" &&
		parts[0] != "admin" &&
		parts[0] != "api" &&
		parts[0] != "terminal" &&
		parts[0] != "functions" &&
		parts[0] != "healthz" &&
		parts[0] != "invocations" {
		return true
	}

	return false
}

// extractFunctionKey extracts the function key from the request context
func extractFunctionKey(c *gin.Context) (string, error) {
	path := c.Request.URL.Path

	// Handle standard invoke URL: /serverless/v1/functions/{urn}/invocations
	if strings.HasPrefix(path, "/serverless/v1/functions/") && strings.HasSuffix(path, "/invocations") {
		plainURN := c.Param(common.FunctionUrnParam)
		if plainURN == "" {
			return "", nil
		}

		// Get headers for alias resolution
		params := make(map[string]string)
		for k := range c.Request.Header {
			params[strings.ToLower(k)] = c.Request.Header.Get(k)
		}

		// Resolve alias to get actual function URN
		functionURN := aliasroute.GetAliases().GetFuncVersionURNWithParams(plainURN, params)
		functionInfo, err := urnutils.GetFunctionInfo(functionURN)
		if err != nil {
			return "", err
		}

		return urnutils.CombineFunctionKey(functionInfo.TenantID, functionInfo.FuncName, functionInfo.FuncVersion), nil
	}

	// Handle short invoke URL: /invocations/{tenant-id}/{namespace}/{function}/
	tenantID := c.Param("tenant-id")
	namespace := c.Param("namespace")
	functionName := c.Param("function")

	if tenantID != "" && namespace != "" && functionName != "" {
		plainURN := urnutils.BuildFunctionShortURN(tenantID, namespace, functionName)

		// Get headers for alias resolution
		params := make(map[string]string)
		for k := range c.Request.Header {
			params[strings.ToLower(k)] = c.Request.Header.Get(k)
		}

		// Resolve alias to get actual function URN
		functionURN := aliasroute.GetAliases().GetFuncVersionURNWithParams(plainURN, params)
		functionInfo, err := urnutils.GetFunctionInfo(functionURN)
		if err != nil {
			return "", err
		}

		return urnutils.CombineFunctionKey(functionInfo.TenantID, functionInfo.FuncName, functionInfo.FuncVersion), nil
	}

	return "", nil
}

// ShouldSkipJWTAuth checks if JWT authentication should be skipped for the current request
func ShouldSkipJWTAuth(c *gin.Context) bool {
	if val, exists := c.Get(skipJWTAuthKey); exists {
		if skip, ok := val.(bool); ok {
			return skip
		}
	}
	return false
}

// IsInvokeURL checks if the current request is for an invoke URL
func IsInvokeURL(c *gin.Context) bool {
	if val, exists := c.Get(isInvokeURLKey); exists {
		if isInvoke, ok := val.(bool); ok {
			return isInvoke
		}
	}
	return false
}
