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
	"errors"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/types"
)

const (
	failedRetrySubscribeSleepTime = 1 * time.Second
	zeroElementSleepTimeLimit     = 15 * time.Second
)

var (
	responseStreamMap          sync.Map
	frontendResponseStreamName = os.Getenv("HOSTNAME")
	errRecNodata               = errors.New("receive no data")
)

// ResponseStream -
type ResponseStream struct {
	processContext *types.InvokeProcessContext
	stopChan       *types.StreamStopChan
	isStream       bool
	processTimes   int32
}

// NewStopChan -
func NewStopChan() *types.StreamStopChan {
	return &types.StreamStopChan{C: make(chan struct{})}
}

// GetFrontendResponseStreamName -
func GetFrontendResponseStreamName() string {
	return frontendResponseStreamName
}

// responseInfo -
type responseInfo struct {
	StatusCode         int                 `json:"statusCode"`
	RequestID          string              `json:"requestID"`
	ResponseHeaders    map[string][]string `json:"responseHeaders"`
	ResponseStreamName string              `json:"responseStreamName"`
}

// RegisterResponse -
func RegisterResponse(ctx *types.InvokeProcessContext) bool {
	if !config.GetConfig().StreamEnable {
		return false
	}
	// 流下载请求是普通http请求无法区分，等监听流收到数据才能确定是下载流请求。所有http请求都要先注册响应流
	ctx.StreamInfo.ResponseStreamName = uuid.New().String()
	responseStopChan := NewStopChan()
	r := &ResponseStream{processContext: ctx, stopChan: responseStopChan}
	responseStreamMap.Store(ctx.StreamInfo.ResponseStreamName, r)
	ctx.StreamInfo.ResponseStopChan = responseStopChan
	return true
}

// ReleaseResponse -
func ReleaseResponse(responseStreamName string) {
	if !config.GetConfig().StreamEnable {
		return
	}
	responseStreamMap.Delete(responseStreamName)
}

// CheckIsResponseStream -
func CheckIsResponseStream(responseStreamName string) bool {
	if !config.GetConfig().StreamEnable {
		return false
	}
	v, ok := responseStreamMap.Load(responseStreamName)
	if !ok {
		return false
	}
	res, ok := v.(*ResponseStream)
	if !ok {
		return false
	}
	return res.isStream
}

// StartListenFrontendResponseStream -
func StartListenFrontendResponseStream(stopCh <-chan struct{}) error {
	return nil
}
