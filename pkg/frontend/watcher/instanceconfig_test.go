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

package watcher

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/frontend/instanceconfigmanager"
)

// TestInstanceConfigHandler 是 GoConvey 测试入口
func TestInstanceConfigHandler(t *testing.T) {
	convey.Convey("Given an instanceConfigHandler function", t, func() {
		calledUpdate := false
		calledDelete := false
		defer gomonkey.ApplyFunc(instanceconfigmanager.ProcessUpdate, func(event *etcd3.Event, logger api.FormatLogger) {
			calledUpdate = true
		}).Reset()
		defer gomonkey.ApplyFunc(instanceconfigmanager.ProcessDelete, func(event *etcd3.Event, logger api.FormatLogger) {
			calledDelete = true
		}).Reset()
		// 测试变量
		event := &etcd3.Event{
			Type:  etcd3.PUT,
			Key:   "/instance/config/key1",
			Value: []byte(`{"config": "value"}`),
			Rev:   123,
		}

		convey.Convey("When event type is PUT", func() {
			event.Type = etcd3.PUT
			event.Key = "/instance/config/key1"
			event.Value = []byte(`{"config": "value"}`)

			instanceConfigHandler(event)

			convey.Convey("Then ProcessUpdate should be called", func() {
				convey.So(calledUpdate, convey.ShouldBeTrue)
			})

		})

		convey.Convey("When event type is DELETE", func() {
			event.Type = etcd3.DELETE
			event.Key = "/instance/config/key2"

			instanceConfigHandler(event)
			convey.Convey("Then ProcessDelete should be called", func() {
				convey.So(calledDelete, convey.ShouldBeTrue)
			})
		})

		convey.Convey("When event type is ERROR", func() {
			event.Type = etcd3.ERROR
			event.Value = []byte("some error message")
			calledUpdate = false
			calledDelete = false
			instanceConfigHandler(event)
			convey.So(calledDelete, convey.ShouldBeFalse)
			convey.So(calledUpdate, convey.ShouldBeFalse)
		})

		convey.Convey("When event type is unsupported", func() {
			event.Type = 999 // 任意不支持的类型
			calledUpdate = false
			calledDelete = false
			instanceConfigHandler(event)
			convey.So(calledDelete, convey.ShouldBeFalse)
			convey.So(calledUpdate, convey.ShouldBeFalse)
		})
	})
}
