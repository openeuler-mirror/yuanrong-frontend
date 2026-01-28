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
	"time"
)

// InstanceReport contains the necessary metric info
type InstanceReport struct {
	ProcReqNum  int64 `json:"procReqNum"`
	AvgProcTime int64 `json:"avgProcTime"`
	MaxProcTime int64 `json:"maxProcTime"`
	IsAbnormal  bool  `json:"isAbnormal"`
}

// ReportRecord is a counter to calculate the metric
type ReportRecord struct {
	// these two field will be accessed by only one go routine
	// the requests completed at the current report period
	requestsCount int64
	// the total time spent by the requests completed at the current report period
	totalDuration int64
	// the max of the time spent by all the requests yet
	maxDuration int64
	isAbnormal  bool
	sync.RWMutex
}

func (r *ReportRecord) recordAbnormal() {
	r.Lock()
	r.isAbnormal = true
	r.Unlock()
}

func (r *ReportRecord) recordRequest(duration time.Duration) {
	r.Lock()
	r.requestsCount++
	durationInMill := duration.Milliseconds()
	r.totalDuration += durationInMill
	if durationInMill > r.maxDuration {
		r.maxDuration = durationInMill
	}
	r.Unlock()
}

func (r *ReportRecord) report(reset bool) *InstanceReport {
	r.Lock()
	report := &InstanceReport{
		ProcReqNum:  r.requestsCount,
		MaxProcTime: r.maxDuration,
		IsAbnormal:  r.isAbnormal,
	}
	if r.requestsCount == 0 {
		report.AvgProcTime = -1
	} else {
		report.AvgProcTime = r.totalDuration / r.requestsCount
	}
	if reset {
		r.requestsCount = 0
		r.totalDuration = 0
	}
	r.Unlock()
	return report
}
