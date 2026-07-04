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
	"testing"

	"github.com/gin-gonic/gin"

	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/types"
)

func TestDefaultIsHTTPUploadStream(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if IsHTTPUploadStream(req) {
		t.Fatal("empty content-type should not be upload stream")
	}
	req.Header.Set(httpconstant.ContentTypeHeaderKey, httpconstant.MultipartFormContentType+"; boundary=x")
	if !IsHTTPUploadStream(req) {
		t.Fatal("multipart content-type should be upload stream")
	}
	if IsHTTPUploadStream(nil) {
		t.Fatal("nil request should not be upload stream")
	}
}

func TestDefaultBuildStreamContext(t *testing.T) {
	ctx := &gin.Context{Request: &http.Request{}, Writer: nil}
	processCtx := &types.InvokeProcessContext{}
	BuildStreamContext(ctx, processCtx)
	if processCtx.StreamInfo == nil {
		t.Fatal("stream info should be initialized")
	}
	if processCtx.StreamInfo.ReqStream != ctx.Request.Body {
		t.Fatal("request stream should come from gin request")
	}
}
