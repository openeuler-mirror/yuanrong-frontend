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

package datasystemclient

import (
	"os"
	"sync/atomic"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"

	mockUtils "frontend/pkg/common/faas_common/utils"
)

func TestInitDataSystemHealthCheck(t *testing.T) {
	convey.Convey("Test initDataSystemHealthCheck", t, func() {
		defer gomonkey.ApplyFunc(dataSystemHealthCheck, func() {
			return
		}).Reset()
		convey.Convey("when NODE_IP is empty", func() {
			initDataSystemHealthCheck("", "")
			convey.So(startHealthCheck.Load(), convey.ShouldBeFalse)
		})
		convey.Convey("test set dataSystem status, when NODE_IP is not equal dataSystemStatusCache", func() {
			defer gomonkey.ApplyFunc(os.Getenv, func(key string) string {
				return "0.0.0.0"
			}).Reset()
			initDataSystemHealthCheck("127.0.0.1", "")
			convey.So(startHealthCheck.Load(), convey.ShouldBeFalse)
		})
		convey.Convey("test set dataSystem status, when NODE_IP is equal dataSystemStatusCache", func() {
			defer gomonkey.ApplyFunc(os.Getenv, func(key string) string {
				return "0.0.0.0"
			}).Reset()
			initDataSystemHealthCheck("0.0.0.0", dataSystemStatusReady)
			convey.So(startHealthCheck.Load(), convey.ShouldBeTrue)
		})
	})
}

func TestIsLocalDataSystemStatusReady(t *testing.T) {
	convey.Convey("test IsLocalDataSystemStatusReady", t, func() {
		convey.Convey("when startHealthCheck is false", func() {
			startHealthCheck.Store(false)
			IsLocalDataSystemStatusReady()
		})
		convey.Convey("when healthCheckResult is false", func() {
			startHealthCheck.Store(true)
			healthCheckResult.Store(false)
			result := IsLocalDataSystemStatusReady()
			convey.So(result, convey.ShouldBeFalse)
		})
		convey.Convey("when healthCheckResult is true", func() {
			startHealthCheck.Store(true)
			healthCheckResult.Store(true)
			result := IsLocalDataSystemStatusReady()
			convey.So(result, convey.ShouldBeTrue)
		})
	})
}

func TestSetStreamEnable(t *testing.T) {
	convey.Convey("test SetStreamEnable", t, func() {
		convey.Convey("test set streamEnable", func() {
			SetStreamEnable(false)
			convey.So(streamEnable.Load(), convey.ShouldBeFalse)
		})
	})
}

func TestIsShutdownFronted(t *testing.T) {
	convey.Convey("test is shout down frontend", t, func() {
		originalStreamEnable := streamEnable.Load()
		originalHealthCheckResult := healthCheckResult.Load()
		defer func() {
			streamEnable.Store(originalStreamEnable)
			healthCheckResult.Store(originalHealthCheckResult)
		}()
		streamEnable.Store(true)

		convey.Convey("when streamEnable is false, skip shutdown", func() {
			streamEnable.Store(false)
			result := isShutdownFronted()
			convey.So(result, convey.ShouldBeFalse)
		})

		convey.Convey("when readiness failure num is not more than failureThreshold, skip shutdown", func() {
			atomic.SwapInt32(&readinessFailureThreshold, failureThreshold)
			healthCheckResult.Store(true)
			result := isShutdownFronted()
			convey.So(result, convey.ShouldBeFalse)
		})

		convey.Convey("when readiness failure num is more than failureThreshold", func() {
			atomic.SwapInt32(&readinessFailureThreshold, failureThreshold)
			healthCheckResult.Store(false)
			result := isShutdownFronted()
			convey.So(result, convey.ShouldBeTrue)
		})
	})
}

func TestDestroy(t *testing.T) {
	convey.Convey("test destroy frontend, when watch dataSystem", t, func() {
		convey.Convey("destroy success", func() {
			defer gomonkey.ApplyMethodFunc(&os.Process{}, "Signal", func(sig os.Signal) error {
				return nil
			}).Reset()
			destroy()
			convey.So("", convey.ShouldBeEmpty)
		})
	})
}

func TestDataSystemHealthCheck(t *testing.T) {
	convey.Convey("Test DataSystemHealthCheck", t, func() {
		defer gomonkey.ApplyFunc(destroy, func() {
			return
		}).Reset()
		localClientLibruntime = &mockUtils.FakeLibruntimeSdkClient{}
		convey.Convey("when streamEnable is false", func() {
			SetStreamEnable(false)
			defer gomonkey.ApplyMethodFunc(localClientLibruntime, "IsDsHealth", func() bool {
				return false
			}).Reset()
			dataSystemHealthCheck()
			convey.So("", convey.ShouldBeEmpty)
		})
	})
}
