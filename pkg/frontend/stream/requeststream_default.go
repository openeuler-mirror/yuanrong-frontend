//go:build !module && !function

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

package stream

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/types"
)

var defaultUpStreamContentTypes = []string{httpconstant.FormContentType, httpconstant.MultipartFormContentType}

// BuildStreamContext populates stream fields for the normal frontend build.
//
// The module build has the same behavior behind the "module" tag. This default
// implementation keeps ordinary `go test` / IDE builds from compiling an empty
// stream package when neither "module" nor "function" tags are supplied.
func BuildStreamContext(ctx interface{}, processCtx *types.InvokeProcessContext) {
	processCtx.StreamInfo = &types.StreamInvokeInfo{}
	ginCtx, ok := ctx.(*gin.Context)
	if !ok || ginCtx == nil || ginCtx.Request == nil {
		return
	}
	processCtx.StreamInfo.ReqStream = ginCtx.Request.Body
	processCtx.StreamInfo.RspStream = ginCtx.Writer
}

// HTTPStreamInvokeHandler is a no-op placeholder; stream forwarding is wired by
// deployment-specific builds.
func HTTPStreamInvokeHandler(ctx interface{}, timeout interface{}) error {
	return nil
}

// IsHTTPUploadStream reports whether the request content type is upload-stream
// capable.
func IsHTTPUploadStream(r interface{}) bool {
	req, ok := r.(*http.Request)
	if !ok || req == nil {
		return false
	}
	contentType := req.Header.Get(httpconstant.ContentTypeHeaderKey)
	if contentType == "" {
		return false
	}
	for _, key := range defaultUpStreamContentTypes {
		if strings.Contains(contentType, key) {
			return true
		}
	}
	return false
}
