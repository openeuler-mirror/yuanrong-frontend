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

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/config"
)

var tokenProxyTenantIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var tokenProxyTokenValidator = jwtauth.ValidateWithIamServer

const (
	tokenProxyTTLDecimalBase = 10
	ttlNoExpiration          = -1
)

// RequireTokenProxyHandler proxies iam-server token require after checking that the caller is system tenant 0.
func (h *Handler) RequireTokenProxyHandler(c *gin.Context) {
	if !validateOperatorToken(c) {
		return
	}

	tenantID := c.GetHeader(iamHeaderTenantID)
	if err := validateTargetTokenProxyTenantID(tenantID); err != nil {
		auditTokenProxyOperation(c, "require", tenantID, "failure", http.StatusBadRequest)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role := c.GetHeader(iamHeaderRole)
	if role != developerRole {
		auditTokenProxyOperation(c, "require", tenantID, "failure", http.StatusBadRequest)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%s must be %s", iamHeaderRole, developerRole)})
		return
	}
	ttl, err := parseTokenProxyTTL(c.GetHeader(iamHeaderTTL))
	if err != nil {
		auditTokenProxyOperation(c, "require", tenantID, "failure", http.StatusBadRequest)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.tokenProxyClient.RequireToken(c.Request.Context(), tenantID, role, ttl, tokenProxyTraceID(c))
	if err != nil {
		writeTokenProxyIAMError(c, "require", tenantID, err)
		return
	}
	writeTokenProxyResponse(c, "require", tenantID, response)
}

// AbandonTokenProxyHandler keeps the route explicit while declaring that developer token revoke is unsupported.
func (h *Handler) AbandonTokenProxyHandler(c *gin.Context) {
	tenantID := c.GetHeader(iamHeaderTenantID)
	if err := validateTargetTokenProxyTenantID(tenantID); err != nil {
		auditTokenProxyOperation(c, "abandon", tenantID, "failure", http.StatusBadRequest)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	auditTokenProxyOperation(c, "abandon", tenantID, "failure", http.StatusNotImplemented)
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "developer token abandon is not supported; token expiration depends on TTL",
	})
}

func validateOperatorToken(c *gin.Context) bool {
	token := c.GetHeader(iamHeaderAuth)
	parsed, err := jwtauth.ParseJWT(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token: " + err.Error()})
		return false
	}
	if config.GetConfig().IamConfig.Addr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "iam-server address is not configured"})
		return false
	}
	if err := tokenProxyTokenValidator(token, tokenProxyTraceID(c)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return false
	}
	if parsed.Payload.IsExpired(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
		return false
	}
	if parsed.Payload.Sub != systemTenantID || parsed.Payload.Role != developerRole {
		c.JSON(http.StatusForbidden, gin.H{"error": "developer system tenant token is required"})
		return false
	}
	return true
}

func parseTokenProxyTTL(ttlHeader string) (*int64, error) {
	if ttlHeader == "" {
		return nil, nil
	}
	ttl, err := strconv.ParseInt(ttlHeader, tokenProxyTTLDecimalBase, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be an integer", iamHeaderTTL)
	}
	if ttl == 0 || ttl < ttlNoExpiration {
		return nil, fmt.Errorf("%s must be -1 or a positive integer", iamHeaderTTL)
	}
	return &ttl, nil
}

func validateTokenProxyTenantID(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("%s header is required", iamHeaderTenantID)
	}
	if !tokenProxyTenantIDPattern.MatchString(tenantID) {
		return fmt.Errorf("%s must match [A-Za-z0-9._-]{1,128}", iamHeaderTenantID)
	}
	return nil
}

func validateTargetTokenProxyTenantID(tenantID string) error {
	if err := validateTokenProxyTenantID(tenantID); err != nil {
		return err
	}
	if tenantID == systemTenantID {
		return errors.New("tenant 0 is reserved")
	}
	return nil
}

func tokenProxyTraceID(c *gin.Context) string {
	if traceID := c.GetHeader(constant.HeaderTraceID); traceID != "" {
		return traceID
	}
	return c.GetHeader(constant.HeaderRequestID)
}

func writeTokenProxyIAMError(c *gin.Context, operation string, tenantID string, err error) {
	var iamErr *TokenProxyIAMError
	if !errors.As(err, &iamErr) {
		auditTokenProxyOperation(c, operation, tenantID, "failure", http.StatusBadGateway)
		c.JSON(http.StatusBadGateway, gin.H{"error": "iam-server request failed"})
		return
	}

	switch iamErr.StatusCode {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError:
		auditTokenProxyOperation(c, operation, tenantID, "failure", iamErr.StatusCode)
		c.JSON(iamErr.StatusCode, gin.H{"error": "iam-server returned error"})
	default:
		auditTokenProxyOperation(c, operation, tenantID, "failure", http.StatusBadGateway)
		c.JSON(http.StatusBadGateway, gin.H{"error": "iam-server returned error"})
	}
}

func writeTokenProxyResponse(c *gin.Context, operation string, tenantID string, response *TokenProxyResponse) {
	if response == nil {
		auditTokenProxyOperation(c, operation, tenantID, "failure", http.StatusBadGateway)
		c.JSON(http.StatusBadGateway, gin.H{"error": "iam-server request failed"})
		return
	}
	for key, values := range response.Headers {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	auditTokenProxyOperation(c, operation, tenantID, "success", http.StatusOK)
	c.Data(http.StatusOK, response.Headers.Get("Content-Type"), response.Body)
}

func auditTokenProxyOperation(c *gin.Context, operation string, tenantID string, result string, statusCode int) {
	callerTenant := "unknown"
	if parsed, err := jwtauth.ParseJWT(c.GetHeader(iamHeaderAuth)); err == nil {
		callerTenant = parsed.Payload.Sub
	}
	log.GetLogger().Infof("token proxy audit operation=%s caller_tenant=%s target_tenant=%s "+
		"result=%s status=%d request_id=%s", operation, callerTenant, tenantID, result, statusCode,
		tokenProxyTraceID(c))
}
