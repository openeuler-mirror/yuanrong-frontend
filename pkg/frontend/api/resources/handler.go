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

// Package resources exposes cluster resource query handlers through frontend.
package resources

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/util"
)

const globalSchedulerResourcesPath = "/global-scheduler/resources"

type activeMasterClient interface {
	GetActiveMasterAddr() string
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	newActiveMasterClient          = func() activeMasterClient { return util.NewClient() }
	resourceHTTPClient    httpDoer = &http.Client{Timeout: 5 * time.Second}
)

// QueryHandler forwards cluster resource queries from frontend to the active function-master.
func QueryHandler(ctx *gin.Context) {
	activeMasterAddr := strings.TrimSpace(newActiveMasterClient().GetActiveMasterAddr())
	if activeMasterAddr == "" {
		ctx.String(http.StatusServiceUnavailable, "active function master is unavailable")
		return
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to read resource query body: %s", err.Error())
		ctx.String(http.StatusInternalServerError, err.Error())
		return
	}

	upstreamURL := normalizeActiveMasterURL(activeMasterAddr) + globalSchedulerResourcesPath
	req, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodGet, upstreamURL, bytes.NewReader(body))
	if err != nil {
		ctx.String(http.StatusInternalServerError, err.Error())
		return
	}
	copyResourceQueryHeaders(req.Header, ctx.Request.Header)

	resp, err := resourceHTTPClient.Do(req)
	if err != nil {
		log.GetLogger().Errorf("failed to query resources from active function master %s: %s", activeMasterAddr, err.Error())
		ctx.String(http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		ctx.String(http.StatusBadGateway, err.Error())
		return
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		ctx.Header("Content-Type", contentType)
	}
	ctx.Status(resp.StatusCode)
	_, _ = ctx.Writer.Write(respBody)
}

func normalizeActiveMasterURL(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return fmt.Sprintf("http://%s", strings.TrimRight(addr, "/"))
}

func copyResourceQueryHeaders(dst, src http.Header) {
	for _, key := range []string{"Type", "Content-Type", "X-Trace-ID", "X-Request-ID"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}
