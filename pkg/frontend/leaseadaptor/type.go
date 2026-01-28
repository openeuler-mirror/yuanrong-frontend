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

// Package leaseadaptor -
package leaseadaptor

// BatchRetainLeaseInfos -
type BatchRetainLeaseInfos struct {
	targetName       string
	infos            map[string]*BatchRetainLeaseInfo
	SchedulerAddress string
}

// BatchRetainLeaseInfo -
type BatchRetainLeaseInfo struct {
	ProcReqNum    int64  `json:"procReqNum"`
	AvgProcTime   int64  `json:"avgProcTime"` // millisecond
	MaxProcTime   int64  `json:"maxProcTime"`
	IsAbnormal    bool   `json:"isAbnormal"`
	ReacquireData []byte `json:"reacquireData"`
	FunctionKey   string `json:"functionKey"`
	PoolKey       string
}

// InstanceAllocationSucceedInfo -
type InstanceAllocationSucceedInfo struct {
	FuncKey    string `json:"funcKey"`
	FuncSig    string `json:"funcSig"`
	InstanceID string `json:"instanceID"`
	ThreadID   string `json:"threadID"`
}

// InstanceAllocationFailedInfo -
type InstanceAllocationFailedInfo struct {
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}
