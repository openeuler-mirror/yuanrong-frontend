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

// Package snerror -
package snerror

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSnError(t *testing.T) {
	convey.Convey("New", t, func() {
		snErr := New(1000, "test error")
		convey.So(snErr.Code(), convey.ShouldEqual, 1000)
		convey.So(snErr.Error(), convey.ShouldEqual, "test error")
		res := IsUserError(snErr)
		convey.So(res, convey.ShouldEqual, false)
	})
	convey.Convey("NewWithError", t, func() {
		snErr := NewWithError(1000, fmt.Errorf("test error"))
		convey.So(snErr.Code(), convey.ShouldEqual, 1000)
		convey.So(snErr.Error(), convey.ShouldEqual, "test error")
	})
}

func TestConvertBadResponse_Convey(t *testing.T) {
	convey.Convey("测试ConvertBadResponse函数", t, func() {
		convey.Convey("当body为空时", func() {
			err := ConvertBadResponse([]byte{})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Code(), convey.ShouldEqual, 0)
			convey.So(err.Error(), convey.ShouldEqual, "empty response body")
		})

		convey.Convey("当body为nil时", func() {
			err := ConvertBadResponse(nil)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Code(), convey.ShouldEqual, 0)
			convey.So(err.Error(), convey.ShouldEqual, "empty response body")
		})

		convey.Convey("当body是有效的JSON时", func() {
			badResp := BadResponse{
				Code:    500,
				Message: "Internal Server Error",
			}
			body, _ := json.Marshal(badResp)
			err := ConvertBadResponse(body)

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Code(), convey.ShouldEqual, 500)
			convey.So(err.Error(), convey.ShouldEqual, "Internal Server Error")
		})

		convey.Convey("当body是无效的JSON时", func() {
			body := []byte("invalid json content")
			err := ConvertBadResponse(body)

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Code(), convey.ShouldEqual, 0)
			convey.So(err.Error(), convey.ShouldContainSubstring, "failed to parse error response")
			convey.So(err.Error(), convey.ShouldContainSubstring, "invalid json content")
		})

		convey.Convey("当code为0且message为空时，使用原始body", func() {
			badResp := BadResponse{
				Code:    0,
				Message: "",
			}
			body, _ := json.Marshal(badResp)
			err := ConvertBadResponse(body)

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Code(), convey.ShouldEqual, 0)
			convey.So(err.Error(), convey.ShouldEqual, string(body))
		})
	})
}
