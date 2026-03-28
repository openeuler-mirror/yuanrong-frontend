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

package responsehandler

import (
	"encoding/json"
	"fmt"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/frontend/types"
)

func generateRespBodyToUser(innerCode int, message json.RawMessage) ([]byte, error) {
	if innerCode == statuscode.InnerResponseSuccessCode {
		return message, nil
	}
	return createErrorResponseBody(innerCode, message, "")
}

func createErrorResponseBody(errorCode int, message json.RawMessage, traceID string) ([]byte, error) {
	body, err := marshalJSONResponse(errorCode, message, traceID)
	if err != nil {
		log.GetLogger().Infof("message is not json format")
		body, err = marshalStringResponse(errorCode, string(message), traceID)
		if err != nil {
			return []byte{}, fmt.Errorf("failed to marshal response data: %s", err)
		}
	}
	return body, nil
}

func marshalJSONResponse(errorCode int, message json.RawMessage, traceID string) ([]byte, error) {
	body, err := json.Marshal(struct {
		Code    int             `json:"code"`
		Message json.RawMessage `json:"message"`
		TraceID string          `json:"trace_id,omitempty"`
	}{
		Code:    errorCode,
		Message: message,
		TraceID: traceID,
	})
	return body, err
}

func marshalStringResponse(errorCode int, message string, traceID string) ([]byte, error) {
	body, err := json.Marshal(struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		TraceID string `json:"trace_id,omitempty"`
	}{
		Code:    errorCode,
		Message: message,
		TraceID: traceID,
	})
	return body, err
}

func buildResponse(ctx *types.InvokeProcessContext, innerCode int, message interface{}) {
	var data []byte
	var err error
	stringMessage, ok := message.(string)
	if ok {
		data, err = marshalStringResponse(innerCode, stringMessage, ctx.TraceID)
		if err != nil {
			log.GetLogger().Errorf("failed to marshal string response data, traceID: %s, err: %s",
				ctx.TraceID, err.Error())
			return
		}
	}
	jsonMessage, ok := message.(json.RawMessage)
	if ok {
		data, err = marshalJSONResponse(innerCode, jsonMessage, ctx.TraceID)
		if err != nil {
			log.GetLogger().Errorf("failed to marshal json response data, traceID: %s, err: %s",
				ctx.TraceID, err.Error())
			return
		}
	}
	ctx.RespBody = data
}
