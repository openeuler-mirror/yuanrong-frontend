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

package leaseadaptor

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReportRecord1(t *testing.T) {
	Convey("Test ReportRecord", t, func() {
		record := &ReportRecord{}

		Convey("When recording normal request", func() {
			record.recordRequest(time.Second)
			report := record.report(false)
			So(report.ProcReqNum, ShouldEqual, 1)
			So(record.totalDuration, ShouldEqual, time.Second.Milliseconds())
			So(report.MaxProcTime, ShouldEqual, time.Second.Milliseconds())
		})

		Convey("When recording abnormal", func() {
			record.recordAbnormal()
			report := record.report(false)
			So(report.IsAbnormal, ShouldBeTrue)
		})

		Convey("When resetting report", func() {
			record.recordRequest(time.Second)
			record.recordAbnormal()
			report := record.report(true)
			So(report.ProcReqNum, ShouldEqual, 1)
			So(report.IsAbnormal, ShouldBeTrue)

			// After reset
			report = record.report(false)
			So(report.ProcReqNum, ShouldEqual, 0)
			So(report.IsAbnormal, ShouldBeTrue)
		})
	})
}

func TestReportRecord(t *testing.T) {
	Convey("Given a new ReportRecord", t, func() {
		record := &ReportRecord{}

		Convey("When recording a request", func() {
			duration := 50 * time.Millisecond
			record.recordRequest(duration)

			Convey("Then the metrics should be updated correctly", func() {
				report := record.report(false)
				So(report.ProcReqNum, ShouldEqual, 1)
				So(report.AvgProcTime, ShouldEqual, 50)
				So(report.MaxProcTime, ShouldEqual, 50)
			})
		})

		Convey("When recording multiple requests", func() {
			durations := []time.Duration{
				30 * time.Millisecond,
				50 * time.Millisecond,
				70 * time.Millisecond,
			}
			for _, d := range durations {
				record.recordRequest(d)
			}

			Convey("Then the metrics should be calculated correctly", func() {
				report := record.report(false)
				So(report.ProcReqNum, ShouldEqual, 3)
				So(report.AvgProcTime, ShouldEqual, 50)
				So(report.MaxProcTime, ShouldEqual, 70)
			})
		})

		Convey("When recording abnormal status", func() {
			record.recordAbnormal()

			Convey("Then the report should indicate abnormal", func() {
				report := record.report(false)
				So(report.IsAbnormal, ShouldBeTrue)
			})
		})

		Convey("When reporting with reset", func() {
			record.recordRequest(100 * time.Millisecond)
			report := record.report(true)

			Convey("Then it should return correct metrics", func() {
				So(report.ProcReqNum, ShouldEqual, 1)
				So(report.MaxProcTime, ShouldEqual, 100)
			})

			Convey("And the counters should be reset", func() {
				reportAfterReset := record.report(false)
				So(reportAfterReset.ProcReqNum, ShouldEqual, 0)
				So(reportAfterReset.MaxProcTime, ShouldEqual, 100)
			})
		})
	})
}
