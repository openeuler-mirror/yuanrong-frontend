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

	. "github.com/smartystreets/goconvey/convey"

	commontypes "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/types"
)

func TestConvertInvokeTag(t *testing.T) {
	Convey("Test convertInvokeTag function", t, func() {
		Convey("When header contains valid invoke tag", func() {
			ctx := &types.InvokeProcessContext{
				ReqHeader: map[string]string{
					httpconstant.HeaderInvokeTag: `{"key1":"value1","key2":"value2"}`,
				},
				TraceID: "test-trace-id",
			}

			result := convertInvokeTag(ctx)
			So(result, ShouldNotBeEmpty)
			So(result["key1"], ShouldEqual, "value1")
			So(result["key2"], ShouldEqual, "value2")
		})

		Convey("When header contains empty invoke tag", func() {
			ctx := &types.InvokeProcessContext{
				ReqHeader: map[string]string{
					httpconstant.HeaderInvokeTag: "",
				},
				TraceID: "test-trace-id",
			}

			result := convertInvokeTag(ctx)
			So(result, ShouldBeEmpty)
		})

		Convey("When header does not contain invoke tag", func() {
			ctx := &types.InvokeProcessContext{
				ReqHeader: make(map[string]string),
				TraceID:   "test-trace-id",
			}

			result := convertInvokeTag(ctx)
			So(result, ShouldBeEmpty)
		})
	})
}

func TestGetTimeout(t *testing.T) {
	Convey("Test getTimeout function", t, func() {
		Convey("When context timeout is not zero", func() {
			result := getTimeout(100, 200)
			So(result, ShouldEqual, 200)
		})

		Convey("When context timeout is zero", func() {
			result := getTimeout(100, 0)
			So(result, ShouldEqual, 100)
		})
	})
}

func TestMakeAcquireOption(t *testing.T) {
	Convey("Test makeAcquireOption function", t, func() {
		mockFuncSpec := &commontypes.FuncSpec{
			FuncMetaSignature: "test-signature",
		}

		Convey("With minimal context", func() {
			ctx := &types.InvokeProcessContext{
				TraceID:        "test-trace-id",
				AcquireTimeout: 0,
				TrafficLimited: false,
				ReqHeader:      make(map[string]string),
			}

			option, err := makeAcquireOption(ctx, mockFuncSpec)
			So(err, ShouldBeNil)
			So(option.TraceID, ShouldEqual, "test-trace-id")
			So(option.FuncSig, ShouldEqual, "test-signature")
			So(option.InvokeTag, ShouldBeEmpty)
		})

		Convey("With full context", func() {
			ctx := &types.InvokeProcessContext{
				TraceID:        "test-trace-id",
				AcquireTimeout: 500,
				TrafficLimited: true,
				ReqHeader: map[string]string{
					"x-pool-label":       "test-pool",
					"x-instance-label":   "test-instance",
					"x-instance-session": `{"session":"test"}`,
					"x-invoke-tag":       `{"key":"value"}`,
				},
			}

			option, err := makeAcquireOption(ctx, mockFuncSpec)
			So(err, ShouldBeNil)
			So(option.PoolLabel, ShouldEqual, "test-pool")
			So(option.InstanceLabel, ShouldEqual, "test-instance")
			So(option.Timeout, ShouldEqual, 500)
			So(option.TrafficLimited, ShouldBeTrue)
			So(option.InvokeTag, ShouldContainKey, "key")
		})

	})
}
