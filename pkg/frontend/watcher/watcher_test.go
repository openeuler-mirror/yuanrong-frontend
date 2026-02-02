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
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
)

func TestStartWatch(t *testing.T) {

	convey.Convey("StartWatch", t, func() {
		patches := []*gomonkey.Patches{
			gomonkey.ApplyFunc(startWatchScheduler, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchRemoteClientLease, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchFunctionMeta, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchFunctionCR, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchCAEFunctionMeta, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchAlias, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchTenantQOS, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchInstanceInfo, func(stopCh <-chan struct{}) {}),
			gomonkey.ApplyFunc(startWatchInstanceConfig, func(stopCh <-chan struct{}) {}),
		}
		defer func() {
			for _, patch := range patches {
				time.Sleep(100 * time.Millisecond)
				patch.Reset()
			}
		}()
		convey.Convey("success ", func() {
			err := StartWatch(make(chan struct{}))
			convey.So(err, convey.ShouldBeNil)
		})

	})
}
