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
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	. "github.com/smartystreets/goconvey/convey"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/schedulerproxy"
)

func TestStartWatchScheduler(t *testing.T) {
	Convey("StartWatchScheduler", t, func() {
		patches := []*gomonkey.Patches{
			gomonkey.ApplyMethod(reflect.TypeOf(&etcd3.EtcdWatcher{}), "StartWatch", func(ew *etcd3.EtcdWatcher) {
			}),
			gomonkey.ApplyFunc(etcd3.GetRouterEtcdClient, func() *etcd3.EtcdClient {
				return &etcd3.EtcdClient{}
			}),
		}
		defer func() {
			for _, patch := range patches {
				time.Sleep(100 * time.Millisecond)
				patch.Reset()
			}
		}()
		startWatchScheduler(make(chan struct{}))
	})
}

func TestSchedulerFilter(t *testing.T) {
	Convey("TestModuleSchedulerFilter", t, func() {
		event := &etcd3.Event{
			Key: "/sn/faas-scheduler/instances/cluster001/7.218.100.25/faas-scheduler-59ddbc4b75-8xdjf",
		}
		So(schedulerFilter(event), ShouldBeFalse)
		event = &etcd3.Event{
			Key: "/sn/instance/business/yrk/tenant/0/function/scheduler/version/latest/defaultaz/requestID/3f079541-15fc-4009-8c41-50b2b2936772",
		}
		So(schedulerFilter(event), ShouldBeTrue)
	})
}

func TestSchedulerHandler(t *testing.T) {
	var (
		founded int32
		missed  int32
	)

	store := make(map[string]string)
	defer gomonkey.ApplyMethod(reflect.TypeOf(schedulerproxy.Proxy), "Add", func(_ *schedulerproxy.ProxyManager, i *schedulerproxy.SchedulerNodeInfo, _ api.FormatLogger) {
		store[i.InstanceInfo.InstanceName] = i.InstanceInfo.InstanceID
		atomic.AddInt32(&founded, 1)
	}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(schedulerproxy.Proxy), "Remove", func(_ *schedulerproxy.ProxyManager, i *types.InstanceInfo, _ api.FormatLogger) {
		delete(store, i.InstanceName)
		atomic.AddInt32(&missed, 1)
	}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(schedulerproxy.Proxy), "Exist", func(_ *schedulerproxy.ProxyManager, instanceName string, instanceId string) bool {
		id, ok := store[instanceName]
		if !ok {
			return false
		}
		if id != instanceId {
			return false
		}
		return true
	}).Reset()
	defer gomonkey.ApplyMethod(reflect.TypeOf(schedulerproxy.Proxy), "ExistInstanceName", func(_ *schedulerproxy.ProxyManager, instanceName string) bool {
		_, ok := store[instanceName]
		return ok
	}).Reset()

	events := []etcd3.Event{
		{
			Type: etcd3.PUT,
			Key:  "/sn/faas-scheduler/instances/cluster001/7.218.100.25/faas-scheduler-59ddbc4b75-8xdjf",
			Value: []byte(`{
    "instanceID": "faas-scheduler-59ddbc4b75-8xdjf",
    "instanceStatus": {
        "code": 3,
        "msg": "running"
    }}`),
		},
		{
			Type: etcd3.DELETE,
			Key:  "/sn/faas-scheduler/instances/cluster001/7.218.100.25/faas-scheduler-59ddbc4b75-8xdjf",
		},
		{
			Type: etcd3.PUT,
			Key:  "/business/yrk/tenant/1/function/xxxxscheduler/version/latest/defaultaz/3f079541-15fc-4009-8c41-50b2b2936772",
		},
		{
			Type: etcd3.DELETE,
			Key:  "/business/yrk/tenant/1/function/xxxxscheduler/version/latest/defaultaz/3f079541-15fc-4009-8c41-50b2b2936772",
		},
	}

	oldType := config.GetConfig().SchedulerKeyPrefixType
	config.GetConfig().SchedulerKeyPrefixType = "function"
	Convey("Test function scheduler Handler", t, func() {
		for _, event := range events {
			schedulerHandler(&event)
		}
		So(atomic.LoadInt32(&founded), ShouldEqual, 1)
		So(atomic.LoadInt32(&missed), ShouldEqual, 1)
	})
	config.GetConfig().SchedulerKeyPrefixType = oldType
}
