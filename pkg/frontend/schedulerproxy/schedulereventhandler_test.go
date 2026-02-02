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

// Package schedulerproxy -
package schedulerproxy

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/uuid"
	"frontend/pkg/frontend/config"
)

func mockEtcdEvent(instanceName, instanceId string, isLease bool) *etcd3.Event {
	etcdPrefix := "/sn/faas-scheduler/instances///"
	etcdkey := etcdPrefix + instanceName

	if isLease {
		etcdkey += "/" + uuid.New().String()
	}
	instanceInfo := &types.InstanceSpecification{
		InstanceID: instanceId,
	}
	return &etcd3.Event{
		Type:      etcd3.PUT,
		Key:       etcdkey,
		Value:     getBytes(instanceInfo),
		PrevValue: nil,
		Rev:       0,
		ETCDType:  "",
	}
}

func getBytes(info *types.InstanceSpecification) []byte {
	bytes, _ := json.Marshal(info)
	return bytes
}

func TestProcessSchedulerEvent(t *testing.T) {
	instance1Event := mockEtcdEvent("instanceName1", "instanceId1", false)
	instance2Event := mockEtcdEvent("instanceName2", "instanceId2", false)
	instance1LeaseEvent := mockEtcdEvent("instanceName1", "instanceId1", true)
	instance2LeaseEvent := mockEtcdEvent("instanceName2", "instanceId2", true)

	schedulerMap := make(map[string]*SchedulerNodeInfo, 0)
	defer gomonkey.ApplyMethod(reflect.TypeOf(Proxy), "Add", func(_ *ProxyManager, scheduler *SchedulerNodeInfo, _ api.FormatLogger) {
		schedulerMap[scheduler.InstanceInfo.InstanceName] = scheduler
	}).Reset()

	defer gomonkey.ApplyMethod(reflect.TypeOf(Proxy), "Remove", func(_ *ProxyManager, schedulerInstance *types.InstanceInfo, _ api.FormatLogger) {
		delete(schedulerMap, schedulerInstance.InstanceName)
	}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(Proxy), "ExistInstanceName", func(_ *ProxyManager, instanceName string) bool {
		_, ok := schedulerMap[instanceName]
		return ok
	}).Reset()
	convey.Convey("Test module scheduler ProcessUpdate", t, func() {
		oldType := config.GetConfig().SchedulerKeyPrefixType
		defer func() {
			config.GetConfig().SchedulerKeyPrefixType = oldType
		}()

		config.GetConfig().SchedulerKeyPrefixType = "module"

		m := &EventManagerInfo{
			leaseIds:       make(map[string]map[string]time.Time),
			schedulerInfos: make(map[string]*types.InstanceInfo),
			RWMutex:        sync.RWMutex{},
		}
		m.ProcessUpdate(instance1Event, log.GetLogger())
		m.ProcessUpdate(instance2Event, log.GetLogger())

		convey.So(len(schedulerMap), convey.ShouldEqual, 0)

		m.ProcessUpdate(instance1LeaseEvent, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 1)

		m.ProcessUpdate(instance2LeaseEvent, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 2)

		m.ProcessDelete(instance1LeaseEvent, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 1)

		m.ProcessDelete(instance2Event, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 0)
		m.ProcessDelete(instance1Event, log.GetLogger())
		m.ProcessDelete(instance2LeaseEvent, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 0)
	})

	convey.Convey("Test function scheduler ProcessUpdate", t, func() {
		oldType := config.GetConfig().SchedulerKeyPrefixType
		config.GetConfig().SchedulerKeyPrefixType = "function"
		defer func() {
			config.GetConfig().SchedulerKeyPrefixType = oldType
		}()
		schedulerMap = make(map[string]*SchedulerNodeInfo)
		m := &EventManagerInfo{
			leaseIds:       make(map[string]map[string]time.Time),
			schedulerInfos: make(map[string]*types.InstanceInfo),
			RWMutex:        sync.RWMutex{},
		}

		m.ProcessUpdate(instance1Event, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 1)
		m.ProcessUpdate(instance2LeaseEvent, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 1)
		m.ProcessUpdate(instance2Event, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 2)

		m.ProcessDelete(instance1LeaseEvent, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 2)
		m.ProcessDelete(instance2Event, log.GetLogger())
		m.ProcessDelete(instance1Event, log.GetLogger())
		convey.So(len(schedulerMap), convey.ShouldEqual, 0)
	})
}
