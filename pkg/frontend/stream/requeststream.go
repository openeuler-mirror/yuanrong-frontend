//go:build module

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

const (
	readOneMB = 1024 * 1024
)

var upStreamContentTypes = []string{httpconstant.FormContentType, httpconstant.MultipartFormContentType}

// BuildStreamContext -
func BuildStreamContext(ctx *gin.Context, processCtx *types.InvokeProcessContext) {
	processCtx.StreamInfo = &types.StreamInvokeInfo{
		ReqStream: ctx.Request.Body,
		// 下载流请求是一个普通的http请求，无法区分，因此所有场景rspStream都需要透传
		RspStream: ctx.Writer,
	}
}

// HTTPStreamInvokeHandler -
func HTTPStreamInvokeHandler(ctx interface{}, timeout interface{}) error {
	return nil
}

// IsHTTPUploadStream -
func IsHTTPUploadStream(r *http.Request) bool {
	contentType := r.Header.Get(httpconstant.ContentTypeHeaderKey)
	if len(contentType) < 1 {
		return false
	}
	for _, k := range upStreamContentTypes {
		if strings.Contains(contentType, k) {
			return true
		}
	}
	return false
}
