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

import (
	"sync"
	"sync/atomic"
	"time"

	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/utils"
)

func newInstanceLease(info *types.InstanceAllocationInfo, acquireOption *types.AcquireOption) *InstanceLease {
	lease := &InstanceLease{
		InstanceAllocationInfo: info,
		reportRecord:           &ReportRecord{},
		acquireOption:          acquireOption,
		claimTime:              time.Time{},
		available:              atomic.Bool{},
		stopCh:                 make(chan struct{}, 0),
		exited:                 atomic.Bool{},
		beginRelease:           atomic.Bool{},
		schedulerInstanceId:    "",
		reacquire:              false,
		RWMutex:                sync.RWMutex{},
	}
	lease.available.Store(true)
	return lease
}

// InstanceLease holds a lease of an invokable instanceID acquired from instance scheduler
type InstanceLease struct {
	*types.InstanceAllocationInfo
	reportRecord  *ReportRecord
	acquireOption *types.AcquireOption
	claimTime     time.Time
	available     atomic.Bool
	stopCh        chan struct{}
	exited        atomic.Bool
	beginRelease  atomic.Bool

	schedulerInstanceId string
	reacquire           bool
	sync.RWMutex
}

func (il *InstanceLease) report(reset bool) *InstanceReport {
	return il.reportRecord.report(reset)
}

func (il *InstanceLease) claim() bool {
	il.Lock()
	if !il.available.Load() {
		il.Unlock()
		return false
	}
	il.available.Store(false)
	il.beginRelease.Store(false)
	il.claimTime = time.Now()
	il.Unlock()
	return true
}

func (il *InstanceLease) free(abnormal bool, record bool) {
	if abnormal {
		il.reportRecord.recordAbnormal()
		il.destroy()
		return
	}
	var claimTime time.Time
	il.Lock()
	il.available.Store(true)
	// if claimTime is zero then it's already freed and should not record request
	if !il.claimTime.IsZero() {
		claimTime = il.claimTime
		il.claimTime = time.Time{}
	}
	il.Unlock()
	if record && !claimTime.IsZero() {
		il.reportRecord.recordRequest(time.Now().Sub(claimTime))
	}
}

func (il *InstanceLease) destroy() {
	il.exited.Store(true)
	il.available.Store(false)
	utils.SafeCloseChannel(il.stopCh)
}
