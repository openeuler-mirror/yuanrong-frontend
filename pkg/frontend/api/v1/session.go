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

package v1

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"frontend/pkg/common/faas_common/datasystemclient"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/statuscode"
	commontype "frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/invocation"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/types"
)

const (
	sessionIDParam        = "sessionId"
	agentSessionKeyPrefix = "yr:agent_session:v1"
	maxSessionKeyLength   = 64
)

var interruptHandler = newInterruptHandler()

// InterruptSessionHandler handles an interrupt session request by querying the session-bound instance first.
func InterruptSessionHandler(ctx *gin.Context) {
	traceID := httputil.InitTraceID(ctx)
	logger := log.GetLogger().With(zap.Any("traceId", traceID))
	logger.Infof("interrupt session handler receives one request")

	processCtx, err := buildProcessContext(ctx, traceID)
	if err != nil {
		logger.Errorf("failed to set processCtx req, error: %s", err.Error())
		writeHTTPResponse(ctx, processCtx)
		return
	}
	defer writeInterfaceLog(processCtx)

	if err = setInstanceSessionHeader(processCtx.ReqHeader, ctx.Param(sessionIDParam)); err != nil {
		logger.Errorf("failed to set session header, error: %s", err.Error())
		responsehandler.SetErrorInContextWithDefault(processCtx, err, statuscode.FrontendStatusBadRequest, err.Error())
		writeHTTPResponse(ctx, processCtx)
		return
	}
	processCtx.IsInterrupted = true

	if err = interruptHandler.Handle(processCtx); err != nil {
		logger.Errorf("interrupt failed,error: %s", err.Error())
	}
	writeHTTPResponse(ctx, processCtx)
}

// DeleteSessionHandler deletes the persisted session data directly from datasystem.
func DeleteSessionHandler(ctx *gin.Context) {
	traceID := httputil.InitTraceID(ctx)
	headers := readHeaders(ctx.Request.Header)
	funcURN, _, err := extractFunctionURN(ctx, headers)
	if err != nil {
		writeSessionError(ctx, http.StatusBadRequest, statuscode.FrontendStatusBadRequest, err)
		return
	}

	sessionID := ctx.Param(sessionIDParam)
	if sessionID == "" {
		writeSessionError(ctx, http.StatusBadRequest, statuscode.FrontendStatusBadRequest,
			fmt.Errorf("sessionId is empty"))
		return
	}

	funcKey := urnutils.CombineFunctionKey(funcURN.TenantID, funcURN.FuncName, funcURN.FuncVersion)
	sessionKey := buildSessionDataKey(funcKey, sessionID)
	if _, err = datasystemclient.KVGetWithRetry(sessionKey, &datasystemclient.Option{TenantID: funcURN.TenantID}, traceID); err != nil {
		if errors.Is(err, datasystemclient.ErrKeyNotFound) {
			writeSessionError(ctx, http.StatusNotFound, statuscode.FrontendStatusNotFound,
				fmt.Errorf("session not found"))
			return
		}
		writeSessionError(ctx, http.StatusInternalServerError, statuscode.InternalErrorCode, err)
		return
	}
	if err = datasystemclient.KVDelWithRetry(sessionKey, &datasystemclient.Option{TenantID: funcURN.TenantID}, traceID); err != nil {
		writeSessionError(ctx, http.StatusInternalServerError, statuscode.InternalErrorCode, err)
		return
	}
	ctx.Status(http.StatusOK)
}

func setInstanceSessionHeader(headers map[string]string, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("sessionId is empty")
	}
	session, err := json.Marshal(&commontype.InstanceSessionConfig{
		SessionID:   sessionID,
		SessionTTL:  0,
		Concurrency: 1,
	})
	if err != nil {
		return err
	}
	headers[httpconstant.HeaderInstanceSession] = string(session)
	return nil
}

func getInstanceSession(headers map[string]string) (*commontype.InstanceSessionConfig, error) {
	instanceSession := headers[httpconstant.HeaderInstanceSession]
	if instanceSession == "" {
		return nil, fmt.Errorf("instance session header is empty")
	}
	session := &commontype.InstanceSessionConfig{}
	if err := json.Unmarshal([]byte(instanceSession), session); err != nil {
		return nil, err
	}
	return session, nil
}

func newInterruptHandler() middleware.HandlerChain {
	handler := middleware.NewBaseHandler(handleInterruptRequest)
	handler.Use(
		middleware.GraceExitFilter,
		middleware.BodySizeChecker,
		middleware.TrafficLimiter,
		middleware.RequestAuthCheck,
	)
	return handler
}

func handleInterruptRequest(processCtx *types.InvokeProcessContext) error {
	session, err := getInstanceSession(processCtx.ReqHeader)
	if err != nil {
		responsehandler.SetErrorInContext(processCtx, statuscode.FrontendStatusBadRequest, err.Error())
		return err
	}

	instanceInfo, queryErr := leaseadaptor.QuerySession(processCtx.FuncKey, session.SessionID, processCtx.TraceID)
	if queryErr != nil {
		responsehandler.SetErrorInContext(processCtx, queryErr.Code(), queryErr.Error())
		return queryErr
	}

	return invocation.InvokeResolvedInstance(processCtx, instanceInfo.InstanceID)
}

func buildSessionDataKey(functionName, sessionID string) string {
	hash := sha256.Sum256([]byte(functionName + ":" + sessionID))
	sessionKey := agentSessionKeyPrefix + ":" + base64.RawURLEncoding.EncodeToString(hash[:])
	if len(sessionKey) > maxSessionKeyLength {
		return sessionKey[:maxSessionKeyLength]
	}
	return sessionKey
}

func writeSessionError(ctx *gin.Context, httpCode int, innerCode int, err error) {
	ctx.JSON(httpCode, types.InvokeErrorResponse{
		Code:    innerCode,
		Message: err.Error(),
	})
}
