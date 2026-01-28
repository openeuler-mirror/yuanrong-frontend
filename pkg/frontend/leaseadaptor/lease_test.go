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

	"github.com/smartystreets/goconvey/convey"

	commontypes "frontend/pkg/common/faas_common/types"
)

func TestInstanceLease(t *testing.T) {
	convey.Convey("Test InstanceLease", t, func() {
		info := &commontypes.InstanceAllocationInfo{}
		option := &commontypes.AcquireOption{}
		lease := newInstanceLease(info, option)

		convey.Convey("When creating new lease", func() {
			convey.So(lease.available.Load(), convey.ShouldBeTrue)
			convey.So(lease.exited.Load(), convey.ShouldBeFalse)
			convey.So(lease.beginRelease.Load(), convey.ShouldBeFalse)
			convey.So(lease.stopCh, convey.ShouldNotBeNil)
		})

		convey.Convey("When claiming lease", func() {
			result := lease.claim()
			convey.So(result, convey.ShouldBeTrue)
			convey.So(lease.available.Load(), convey.ShouldBeFalse)
			convey.So(lease.claimTime.IsZero(), convey.ShouldBeFalse)

			convey.Convey("When claiming already claimed lease", func() {
				result = lease.claim()
				convey.So(result, convey.ShouldBeFalse)
			})
		})

		convey.Convey("When freeing lease normally", func() {
			lease.claim()
			lease.free(false, true)
			convey.So(lease.available.Load(), convey.ShouldBeTrue)
			convey.So(lease.claimTime.IsZero(), convey.ShouldBeTrue)
		})

		convey.Convey("When freeing lease abnormally", func() {
			lease.free(true, false)
			convey.So(lease.exited.Load(), convey.ShouldBeTrue)
			convey.So(lease.available.Load(), convey.ShouldBeFalse)
		})

		convey.Convey("When destroying lease", func() {
			lease.destroy()
			convey.So(lease.exited.Load(), convey.ShouldBeTrue)
			convey.So(lease.available.Load(), convey.ShouldBeFalse)
		})
	})
}
