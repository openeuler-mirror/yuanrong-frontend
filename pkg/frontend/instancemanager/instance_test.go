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

package instancemanager

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/logger/log"
	commontype "frontend/pkg/common/faas_common/types"
)

func TestRouteOnlyInstanceExpires(t *testing.T) {
	instanceID := "route-only-expiring"
	recordRouteOnlyInstanceWithTTL("tenant/function/$latest", instanceID, "proxy-owner", time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for SnapshotRouteOnlyInstance(instanceID).Present && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if SnapshotRouteOnlyInstance(instanceID).Present {
		t.Fatal("route-only instance did not expire")
	}
}

func TestRouteOnlyInstanceSnapshotIsPayloadFreeAndReadOnly(t *testing.T) {
	instanceID := "route-snapshot-instance"
	RecordRouteOnlyInstance("tenant/function/$latest", instanceID, "proxy-owner")
	defer RemoveRouteOnlyInstance(instanceID)

	snapshot := SnapshotRouteOnlyInstance(instanceID)
	if !snapshot.Present || snapshot.InstanceID != instanceID ||
		snapshot.Function != "tenant/function/$latest" || snapshot.FunctionProxyID != "proxy-owner" {
		t.Fatalf("unexpected route snapshot: %+v", snapshot)
	}

	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal route snapshot: %v", err)
	}
	if string(encoded) == "" || SnapshotRouteOnlyInstance(instanceID).Present != true {
		t.Fatalf("snapshot must remain present after read: %s", encoded)
	}
	if strings.Contains(string(encoded), "payload") {
		t.Fatalf("route snapshot leaked payload field: %s", encoded)
	}
}

func TestRemoveRouteOnlyInstanceReturnsBeforeAndAfterSnapshots(t *testing.T) {
	instanceID := "route-remove-instance"
	RecordRouteOnlyInstance("tenant/function/$latest", instanceID, "proxy-owner")

	change := RemoveRouteOnlyInstanceWithSnapshot(instanceID)

	if !change.Before.Present || change.Before.FunctionProxyID != "proxy-owner" {
		t.Fatalf("unexpected before snapshot: %+v", change.Before)
	}
	if change.After.Present || change.After.InstanceID != instanceID {
		t.Fatalf("unexpected after snapshot: %+v", change.After)
	}
	if SnapshotRouteOnlyInstance(instanceID).Present {
		t.Fatal("route snapshot still present after removal")
	}
}

func TestInstanceQueueAddInstance(t *testing.T) {
	convey.Convey("test functionInstanceQueue addInstance", t, func() {
		var wg sync.WaitGroup
		expectedInstanceQueueSize := 5
		instanceTestQueue := &functionInstanceQueue{
			lock:      sync.RWMutex{},
			instances: make(map[string]*commontype.InstanceSpecification, expectedInstanceQueueSize),
		}

		for i := 0; i < expectedInstanceQueueSize; i++ {
			wg.Add(1)
			instance := &commontype.InstanceSpecification{
				InstanceID: "functionInstanceQueue" + strconv.Itoa(i),
			}
			go func(instance *commontype.InstanceSpecification) {
				defer wg.Done()
				instanceTestQueue.addInstance(instance, log.GetLogger())
			}(instance)
		}

		wg.Wait()
		convey.So(instanceTestQueue.size(), convey.ShouldEqual, expectedInstanceQueueSize)
	})
}

func TestInstanceQueueDelInstance(t *testing.T) {
	convey.Convey("test functionInstanceQueue delInstance", t, func() {
		var wg sync.WaitGroup
		expectedInstanceQueueSize := 5
		instanceMap := make(map[string]*commontype.InstanceSpecification, expectedInstanceQueueSize)
		for i := 0; i < expectedInstanceQueueSize; i++ {
			instanceID := "functionInstanceQueue" + strconv.Itoa(i)
			instanceMap[instanceID] = &commontype.InstanceSpecification{
				InstanceID: instanceID,
			}
		}
		instanceTestQueue := &functionInstanceQueue{
			lock:      sync.RWMutex{},
			instances: instanceMap,
		}

		instances := make([]*commontype.InstanceSpecification, 0, len(instanceMap))
		for _, instance := range instanceMap {
			instances = append(instances, instance)
		}
		for _, v := range instances {
			wg.Add(1)
			go func(instance *commontype.InstanceSpecification) {
				defer wg.Done()
				instanceTestQueue.delInstance(instance, log.GetLogger())
			}(v)
		}

		wg.Wait()
		convey.So(instanceTestQueue.size(), convey.ShouldEqual, 0)
	})
}

func TestInstanceQueueGetSize(t *testing.T) {
	convey.Convey("test functionInstanceQueue size", t, func() {
		instanceSliceAddSize := 5
		instanceSliceDelSize := 3
		instanceTestQueue := &functionInstanceQueue{
			lock:      sync.RWMutex{},
			instances: make(map[string]*commontype.InstanceSpecification, instanceSliceAddSize),
		}
		instanceSlice := make([]*commontype.InstanceSpecification, instanceSliceAddSize)
		for i := 0; i < instanceSliceAddSize; i++ {
			instanceID := "functionInstanceQueue" + strconv.Itoa(i)
			instanceSlice[i] = &commontype.InstanceSpecification{
				InstanceID: instanceID,
			}
		}

		var wg sync.WaitGroup
		for _, v := range instanceSlice {
			wg.Add(1)
			go func(instance *commontype.InstanceSpecification) {
				defer wg.Done()
				instanceTestQueue.addInstance(instance, log.GetLogger())
			}(v)
		}
		wg.Wait()

		for i := 0; i < instanceSliceDelSize; i++ {
			wg.Add(1)
			go func(instance *commontype.InstanceSpecification) {
				defer wg.Done()
				instanceTestQueue.delInstance(instance, log.GetLogger())
			}(instanceSlice[i])
		}
		wg.Wait()
		convey.So(instanceTestQueue.size(), convey.ShouldEqual, instanceSliceAddSize-instanceSliceDelSize)
	})
}

func TestFunctionInstanceMapAddInstance(t *testing.T) {
	convey.Convey("test functionInstanceMap addInstance", t, func() {
		convey.Convey("multiple resKeys, check if there are corresponding multiple instancequeues.", func() {
			instanceSlice := []*commontype.InstanceSpecification{
				{
					InstanceID: "functionInstanceQueue",
					CreateOptions: map[string]string{
						"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
					},
				},
				{
					InstanceID: "functionInstanceQueue",
					CreateOptions: map[string]string{
						"RESOURCE_SPEC_NOTE": "{\"cpu\":110,\"memory\":110}",
					},
				},
				{
					InstanceID: "functionInstanceQueue",
					CreateOptions: map[string]string{
						"RESOURCE_SPEC_NOTE": "{\"cpu\":120,\"memory\":120}",
					},
				},
			}
			expectedFuncInstMapSize := len(instanceSlice)
			functionInstanceTestQueue := &functionInstanceMap{
				lock:           sync.RWMutex{},
				instanceQueues: make(map[string]*functionInstanceQueue, expectedFuncInstMapSize),
			}

			var wg sync.WaitGroup
			for _, v := range instanceSlice {
				wg.Add(1)
				go func(instance *commontype.InstanceSpecification) {
					defer wg.Done()
					functionInstanceTestQueue.addInstance(instance, log.GetLogger())
				}(v)
			}

			wg.Wait()
			convey.So(functionInstanceTestQueue.size(), convey.ShouldEqual, expectedFuncInstMapSize)
		})

		convey.Convey("resKey format is incorrect, add instance failed", func() {
			instanceSlice := []*commontype.InstanceSpecification{
				{
					InstanceID: "functionInstanceQueue",
					CreateOptions: map[string]string{
						"RESOURCE_SPEC_NOTE": "{\"cpu\":\"abc\",\"memory\":\"100\"}",
					},
				},
			}
			expectedFuncInstMapSize := len(instanceSlice)
			functionInstanceTestQueue := &functionInstanceMap{
				lock:           sync.RWMutex{},
				instanceQueues: make(map[string]*functionInstanceQueue, expectedFuncInstMapSize),
			}

			var wg sync.WaitGroup
			for _, v := range instanceSlice {
				wg.Add(1)
				go func(instance *commontype.InstanceSpecification) {
					defer wg.Done()
					functionInstanceTestQueue.addInstance(instance, log.GetLogger())
				}(v)
			}

			wg.Wait()
			convey.So(functionInstanceTestQueue.size(), convey.ShouldEqual, 0)
		})

		convey.Convey("For the same resKey, functionInstanceMap instanceQueues size meets the expectation.", func() {
			instanceSlice := []*commontype.InstanceSpecification{
				{
					InstanceID: "instanceQueue0",
					CreateOptions: map[string]string{
						"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
					},
				},
				{
					InstanceID: "instanceQueue1",
					CreateOptions: map[string]string{
						"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
					},
				},
			}
			functionInstanceTestQueue := &functionInstanceMap{
				lock:           sync.RWMutex{},
				instanceQueues: make(map[string]*functionInstanceQueue, 1),
			}

			for _, v := range instanceSlice {
				functionInstanceTestQueue.addInstance(v, log.GetLogger())
			}

			// InstanceID不同，RESOURCE_SPEC_NOTE相同，预期结果如下
			convey.So(functionInstanceTestQueue.size(), convey.ShouldEqual, 1)
			convey.So(functionInstanceTestQueue.instanceQueues["cpu-100-mem-100-storage-0-cstRes--cstResSpec--invokeLabel-"].size(), convey.ShouldEqual, 2)
		})
	})
}

func TestFunctionInstanceMapDelInstance(t *testing.T) {
	convey.Convey("test functionInstanceMap delInstance", t, func() {
		instance1 := &commontype.InstanceSpecification{
			InstanceID: "instanceQueue1",
			CreateOptions: map[string]string{
				"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
			},
		}
		instance2 := &commontype.InstanceSpecification{
			InstanceID: "instanceQueue2",
			CreateOptions: map[string]string{
				"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
			},
		}

		resKey := "cpu-100-mem-100-storage-0-cstRes--cstResSpec--invokeLabel-"
		functionInstanceTestQueue := &functionInstanceMap{
			lock:           sync.RWMutex{},
			instanceQueues: make(map[string]*functionInstanceQueue, 1),
		}
		functionInstanceTestQueue.addInstance(instance1, log.GetLogger())
		functionInstanceTestQueue.addInstance(instance2, log.GetLogger())

		convey.Convey("delete the instanceQueues of resKey correctly ", func() {
			functionInstanceTestQueue.delInstance(instance1, log.GetLogger())
			convey.So(functionInstanceTestQueue.size(), convey.ShouldEqual, 1)
			convey.So(functionInstanceTestQueue.instanceQueues[resKey].size(), convey.ShouldEqual, 1)
		})

		convey.Convey("resKey is not in instanceQueues, skip delete", func() {
			otherInstance := &commontype.InstanceSpecification{
				InstanceID: "functionInstanceQueue",
				CreateOptions: map[string]string{
					"RESOURCE_SPEC_NOTE": "{\"cpu\":110,\"memory\":110}",
				},
			}
			functionInstanceTestQueue.delInstance(otherInstance, log.GetLogger())
			convey.So(functionInstanceTestQueue.size(), convey.ShouldEqual, 1)
		})

		convey.Convey("", func() {
			functionInstanceTestQueue.delInstance(instance1, log.GetLogger())
			functionInstanceTestQueue.delInstance(instance2, log.GetLogger())
			convey.So(functionInstanceTestQueue.size(), convey.ShouldEqual, 0)
		})
	})
}

func TestGlobalInstancesMapAddInstance(t *testing.T) {
	convey.Convey("test globalInstancesMap addInstance", t, func() {
		funcKey := "c53626012ba84727b938ca8bf03108ef/0@default@zscaetest/latest"
		instanceSlice := []*commontype.InstanceSpecification{
			{
				InstanceID: "functionInstanceQueue",
				CreateOptions: map[string]string{
					"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
				},
			},
			{
				InstanceID: "functionInstanceQueue",
				CreateOptions: map[string]string{
					"RESOURCE_SPEC_NOTE": "{\"cpu\":110,\"memory\":110}",
				},
			},
			{
				InstanceID: "functionInstanceQueue",
				CreateOptions: map[string]string{
					"RESOURCE_SPEC_NOTE": "{\"cpu\":120,\"memory\":120}",
				},
			},
		}
		expectedFuncInstMapSize := len(instanceSlice)

		convey.Convey("add instance success", func() {
			globalInstanceTestQueue := &FunctionInstancesMap{
				lock:         sync.RWMutex{},
				instancesMap: make(map[string]*functionInstanceMap, 1),
			}
			for _, v := range instanceSlice {
				globalInstanceTestQueue.addInstance(funcKey, v, log.GetLogger())
			}

			convey.So(len(globalInstanceTestQueue.instancesMap), convey.ShouldEqual, 1)
			convey.So(globalInstanceTestQueue.instancesMap[funcKey].size(), convey.ShouldEqual, expectedFuncInstMapSize)
		})

		convey.Convey("add instance success in the concurrent scenario", func() {
			globalInstanceTestQueue := &FunctionInstancesMap{
				lock:         sync.RWMutex{},
				instancesMap: make(map[string]*functionInstanceMap, 1),
			}
			var wg sync.WaitGroup
			for _, v := range instanceSlice {
				wg.Add(1)
				go func(v *commontype.InstanceSpecification) {
					defer wg.Done()
					globalInstanceTestQueue.addInstance(funcKey, v, log.GetLogger())
				}(v)
			}

			wg.Wait()
			convey.So(len(globalInstanceTestQueue.instancesMap), convey.ShouldEqual, 1)
			convey.So(globalInstanceTestQueue.instancesMap[funcKey].size(), convey.ShouldEqual, expectedFuncInstMapSize)
		})
	})
}

func TestGlobalInstancesMapDelInstance(t *testing.T) {
	convey.Convey("test globalInstancesMap delInstance", t, func() {
		funcKey := "c53626012ba84727b938ca8bf03108ef/0@default@zscaetest/latest"
		instanceSlice := []*commontype.InstanceSpecification{
			{
				InstanceID: "functionInstanceQueue",
				CreateOptions: map[string]string{
					"RESOURCE_SPEC_NOTE": "{\"cpu\":100,\"memory\":100}",
				},
			},
			{
				InstanceID: "functionInstanceQueue",
				CreateOptions: map[string]string{
					"RESOURCE_SPEC_NOTE": "{\"cpu\":110,\"memory\":110}",
				},
			},
		}
		globalInstanceTestQueue := &FunctionInstancesMap{
			lock:         sync.RWMutex{},
			instancesMap: make(map[string]*functionInstanceMap, 1),
		}
		for _, v := range instanceSlice {
			globalInstanceTestQueue.addInstance(funcKey, v, log.GetLogger())
		}

		convey.Convey("delete instance success", func() {
			globalInstanceTestQueue.delInstance(funcKey, instanceSlice[0], log.GetLogger())

			convey.So(len(globalInstanceTestQueue.instancesMap), convey.ShouldEqual, 1)
			convey.So(globalInstanceTestQueue.instancesMap[funcKey].size(), convey.ShouldEqual, 1)
		})

		convey.Convey("delete duplicate instance success", func() {
			globalInstanceTestQueue.delInstance(funcKey, instanceSlice[0], log.GetLogger())
			globalInstanceTestQueue.delInstance(funcKey, instanceSlice[0], log.GetLogger())

			convey.So(len(globalInstanceTestQueue.instancesMap), convey.ShouldEqual, 1)
			convey.So(globalInstanceTestQueue.instancesMap[funcKey].size(), convey.ShouldEqual, 1)
		})

		convey.Convey("delete all instance success", func() {
			for _, v := range instanceSlice {
				globalInstanceTestQueue.delInstance(funcKey, v, log.GetLogger())
			}

			convey.So(globalInstanceTestQueue.instancesMap, convey.ShouldBeEmpty)
		})

		convey.Convey("delete instance success in the concurrent scenario", func() {
			var wg sync.WaitGroup
			for _, v := range instanceSlice {
				wg.Add(1)
				go func(v *commontype.InstanceSpecification) {
					defer wg.Done()
					globalInstanceTestQueue.delInstance(funcKey, v, log.GetLogger())
				}(v)
			}

			wg.Wait()
			convey.So(globalInstanceTestQueue.instancesMap, convey.ShouldBeEmpty)
		})
	})
}

func TestGetRandomInstanceWithoutUnexpectedInstance(t *testing.T) {
	convey.Convey("When testing GetRandomInstanceWithoutUnexpectedInstance", t, func() {
		// Setup test data
		setupTestEnv := func() *FunctionInstancesMap {
			testMap := &FunctionInstancesMap{
				instancesMap: make(map[string]*functionInstanceMap),
			}
			testMap.instancesMap["funcKey1"] = &functionInstanceMap{
				instanceQueues: make(map[string]*functionInstanceQueue),
			}
			testMap.instancesMap["funcKey1"].instanceQueues["resKey1"] = &functionInstanceQueue{
				instances: map[string]*commontype.InstanceSpecification{
					"inst001": {InstanceID: "inst001"},
					"inst002": {InstanceID: "inst002"},
					"inst003": {InstanceID: "inst003"},
				},
			}
			return testMap
		}

		convey.Convey("With valid funcKey and resKey", func() {
			testMap := setupTestEnv()

			convey.Convey("Should return random instance when no instances excluded", func() {
				instance := testMap.GetRandomInstanceWithoutUnexpectedInstance(
					"funcKey1", "resKey1", []string{}, log.GetLogger())
				convey.So(instance, convey.ShouldNotBeNil)
				convey.So([]string{"inst001", "inst002", "inst003"}, convey.ShouldContain, instance.InstanceID)
			})

			convey.Convey("Should return remaining instance after excluding some", func() {
				instance := testMap.GetRandomInstanceWithoutUnexpectedInstance(
					"funcKey1", "resKey1", []string{"inst001", "inst004"}, log.GetLogger())
				convey.So(instance, convey.ShouldNotBeNil)
				convey.So([]string{"inst002", "inst003"}, convey.ShouldContain, instance.InstanceID)
			})

			convey.Convey("Should return nil when all instances excluded", func() {
				instance := testMap.GetRandomInstanceWithoutUnexpectedInstance(
					"funcKey1", "resKey1",
					[]string{"inst001", "inst002", "inst003"},
					log.GetLogger())
				convey.So(instance, convey.ShouldBeNil)
			})
		})

		convey.Convey("With invalid parameters", func() {
			testMap := setupTestEnv()

			convey.Convey("Should return nil with invalid funcKey", func() {
				instance := testMap.GetRandomInstanceWithoutUnexpectedInstance(
					"invalidFunc", "resKey1", []string{}, log.GetLogger())
				convey.So(instance, convey.ShouldBeNil)
			})

			convey.Convey("Should return nil with invalid resKey", func() {
				instance := testMap.GetRandomInstanceWithoutUnexpectedInstance(
					"funcKey1", "invalidRes", []string{}, log.GetLogger())
				convey.So(instance, convey.ShouldBeNil)
			})

			convey.Convey("Should return nil when instance queue is empty", func() {
				testMap.instancesMap["funcKey1"].instanceQueues["resKey1"].instances =
					make(map[string]*commontype.InstanceSpecification)
				instance := testMap.GetRandomInstanceWithoutUnexpectedInstance(
					"funcKey1", "resKey1", []string{}, log.GetLogger())
				convey.So(instance, convey.ShouldBeNil)
			})
		})
	})
}
