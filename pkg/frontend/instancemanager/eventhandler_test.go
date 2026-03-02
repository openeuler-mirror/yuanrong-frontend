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
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/types"
)

func getCommonInsSpec() *types.InstanceSpecification {
	bytes := []byte("{\"instanceID\":\"5f000000-0000-4000-824c-75b4b7dae0a3\",\"requestID\":\"787b900780b2d80600\",\"runtimeID\":\"runtime-5f000000-0000-4000-824c-75b4b7dae0a3-0000000074dd\",\"runtimeAddress\":\"127.0.0.1:32568\",\"functionAgentID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"functionProxyID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"function\":\"default/0-system-faasExecutorGo1.x/$latest\",\"resources\":{\"resources\":{\"CPU\":{\"name\":\"CPU\",\"scalar\":{\"value\":500}},\"Memory\":{\"name\":\"Memory\",\"scalar\":{\"value\":500}}}},\"scheduleOption\":{\"schedPolicyName\":\"monopoly\",\"affinity\":{\"instanceAffinity\":{},\"resource\":{},\"instance\":{\"scope\":\"NODE\"}},\"initCallTimeOut\":305,\"resourceSelector\":{\"resource.owner\":\"1c50bc05-0000-4000-8000-00ed778a549c\"},\"extension\":{\"schedule_policy\":\"monopoly\"},\"range\":{},\"scheduleTimeoutMs\":\"5000\"},\"createOptions\":{\"INSTANCE_LABEL_NOTE\":\"\",\"DELEGATE_DECRYPT\":\"{\\\"accessKey\\\":\\\"\\\",\\\"authToken\\\":\\\"\\\",\\\"cryptoAlgorithm\\\":\\\"\\\",\\\"encrypted_user_data\\\":\\\"\\\",\\\"envKey\\\":\\\"\\\",\\\"environment\\\":\\\"\\\",\\\"secretKey\\\":\\\"\\\",\\\"securityAk\\\":\\\"\\\",\\\"securitySk\\\":\\\"\\\",\\\"securityToken\\\":\\\"\\\"}\",\"lifecycle\":\"detached\",\"resource.owner\":\"static_function\",\"FUNCTION_KEY_NOTE\":\"8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest\",\"ConcurrentNum\":\"1000\",\"tenantId\":\"8d86c63b22e24d9ab650878b75408ea6\",\"INSTANCE_TYPE_NOTE\":\"reserved\",\"init_call_timeout\":\"305\",\"call_timeout\":\"60\",\"RESOURCE_SPEC_NOTE\":\"{\\\"cpu\\\":500,\\\"invokeLabels\\\":\\\"\\\",\\\"memory\\\":500}\",\"DELEGATE_DIRECTORY_QUOTA\":\"512\",\"GRACEFUL_SHUTDOWN_TIME\":\"900\",\"DELEGATE_DIRECTORY_INFO\":\"/tmp\"},\"instanceStatus\":{\"code\":3,\"msg\":\"running\"},\"schedulerChain\":[\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"],\"parentID\":\"static_function\",\"parentFunctionProxyAID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt-LocalSchedInstanceCtrlActor@10.158.98.238:22423\",\"storageType\":\"local\",\"scheduleTimes\":1,\"deployTimes\":1,\"args\":[{\"value\":\"EkdAAVpDMTIzNDU2Nzg5MDEyMzQ1NjEyMzQ1Njc4OTAxMjM0NTYvMC1zeXN0ZW0tZmFhc0V4ZWN1dG9yR28xLngvJGxhdGVzdBplEgASBy9pbnZva2UYAiD///////////8BKGQwAUJHCAMSQzEyMzQ1Njc4OTAxMjM0NTYxMjM0NTY3ODkwMTIzNDU2LzAtc3lzdGVtLWZhYXNFeGVjdXRvckdvMS54LyRsYXRlc3Q=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiZnVuY01ldGFEYXRhIjp7Im5hbWUiOiIwQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5IiwiZnVuY3Rpb25Vcm4iOiJzbjpjbjp5cms6OGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTY6ZnVuY3Rpb246MEBkZWZhdWx0QGZ1bmM2YWM2NzQxYTAxMzM0MzIwODA5ZGZiN2RjMWU5ODA0OSIsImZ1bmN0aW9uVmVyc2lvblVybiI6InNuOmNuOnlyazo4ZDg2YzYzYjIyZTI0ZDlhYjY1MDg3OGI3NTQwOGVhNjpmdW5jdGlvbjowQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5OmxhdGVzdCIsInZlcnNpb24iOiJsYXRlc3QiLCJmdW5jdGlvblVwZGF0ZVRpbWUiOiIyMDI1LTA2LTIzIDIzOjQ0OjIyLjAwMCIsInJ1bnRpbWUiOiJjdXN0b20gaW1hZ2UiLCJoYW5kbGVyIjoiL2ludm9rZSIsInRpbWVvdXQiOjYwLCJzZXJ2aWNlIjoiZGVmYXVsdCIsInRlbmFudElkIjoiOGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTYiLCJidXNpbmVzc0lkIjoieXJrIiwicmV2aXNpb25JZCI6IjIwMjUwNjIzMTU0NDIyMDEyIiwiZnVuY19uYW1lIjoiZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5In0sImVudk1ldGFEYXRhIjp7ImVudmlyb25tZW50IjoiIn0sImluc3RhbmNlTWV0YURhdGEiOnsibWF4SW5zdGFuY2UiOjEwMCwibWluSW5zdGFuY2UiOjEsImNvbmN1cnJlbnROdW0iOjEwMDAsInNjYWxlUG9saWN5Ijoic3RhdGljRnVuY3Rpb24ifSwicmVzb3VyY2VNZXRhRGF0YSI6eyJjcHUiOjUwMCwibWVtb3J5Ijo1MDB9LCJjb2RlTWV0YURhdGEiOnsic3RvcmFnZV90eXBlIjoiIn0sImV4dGVuZGVkTWV0YURhdGEiOnsiaW5pdGlhbGl6ZXIiOnsiaW5pdGlhbGl6ZXJfdGltZW91dCI6MzAwLCJpbml0aWFsaXplcl9oYW5kbGVyIjoiIn0sImN1c3RvbV9jb250YWluZXJfY29uZmlnIjp7ImltYWdlIjoic3dyLmNuLXNvdXRod2VzdC0yLm15aHVhd2VpY2xvdWQuY29tL3dpc2VmdW5jdGlvbi9jdXN0b20taW1hZ2U6MS4xLjEzLjIwMjUwNTA2MTczNDEyIn19fQ==\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiY2FsbFJvdXRlIjoiaW52b2tlIiwicG9ydCI6ODAwMH0=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsic2NoZWR1bGVyRnVuY0tleSI6IiIsInNjaGVkdWxlcklETGlzdCI6W119\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"}],\"version\":\"3\",\"dataSystemHost\":\"10.158.97.96\",\"gracefulShutdownTime\":\"600\",\"tenantID\":\"8d86c63b22e24d9ab650878b75408ea6\",\"extensions\":{\"receivedTimestamp\":\"1750782213307\",\"podDeploymentName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz\",\"podName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"pid\":\"71\",\"podNamespace\":\"wisefunctionservice-495f57a3-09ee-44d2-87e5-a109cda4dc40\",\"createTimestamp\":\"1750782213\",\"updateTimestamp\":\"1750782231\"},\"unitID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"}")

	insSpec := &types.InstanceSpecification{}
	err := json.Unmarshal(bytes, insSpec)
	if err != nil {
		// 不会发生
	}
	return insSpec
}

func getCommonEventKey() string {
	return "/sn/instance/business/yrk/tenant/12345678901234561234567890123456/function/0-system-faasExecutorGo1.x/version/$latest/defaultaz/787b900780b2d80600/5f000000-0000-4000-824c-75b4b7dae0a3"
}

func getInsSpecBytes(insSpec *types.InstanceSpecification) []byte {
	bytes, _ := json.Marshal(insSpec)
	return bytes
}

func Test_ProcessInstanceUpdate(t *testing.T) {
	convey.Convey("Test_ProcessInstanceUpdate simple", t, func() {
		insSpec := getCommonInsSpec()
		event := &etcd3.Event{
			Key:       getCommonEventKey(),
			Value:     getInsSpecBytes(insSpec),
			PrevValue: nil,
			Rev:       0,
			ETCDType:  "",
		}

		processAppInfoUpdateTrigger := false
		defer gomonkey.ApplyFunc(ProcessAppInfoUpdate, func(event2 *etcd3.Event) {
			processAppInfoUpdateTrigger = true
		}).Reset()

		ProcessInstanceUpdate(event)
		convey.So(processAppInfoUpdateTrigger, convey.ShouldBeTrue)
		processAppInfoUpdateTrigger = false

		event.Key = "/sn/instance/business/yrk/tenant/12345678901234561234567890123456/function/0-system-faasExecutorGo1.x/version/$latest/defaultaz/787b900780b2d80600"
		ProcessInstanceUpdate(event)
		convey.So(processAppInfoUpdateTrigger, convey.ShouldBeFalse)

		event.Key = getCommonEventKey()

		insSpec = getCommonInsSpec()
		insSpec.Function = "0-0-faasmanager"
		event.Value = getInsSpecBytes(insSpec)
		convey.So(IsFaaSManager(insSpec.Function), convey.ShouldBeTrue)

		key := "/sn/instance/business/yrk/tenant/0/function/0-system-faasscheduler/version/$latest/defaultaz//scheduler-efunctionschedulerservicecn-perf-gy-scheduler-green-ytvj7-svkpr"
		convey.So(isFaaSScheduler(key), convey.ShouldBeTrue)

		delFuncKey := ""
		delInstanceTrigger := false
		delInstanceId := ""
		defer gomonkey.ApplyPrivateMethod(reflect.TypeOf(GetGlobalInstanceScheduler()), "delInstance", func(_ *FunctionInstancesMap, funcKey string, insSpec *types.InstanceSpecification) {
			delFuncKey = funcKey
			delInstanceTrigger = true
			delInstanceId = insSpec.InstanceID
		}).Reset()
		addFuncKey := ""
		addInstanceTrigger := false
		addInsntaceId := ""
		defer gomonkey.ApplyPrivateMethod(reflect.TypeOf(GetGlobalInstanceScheduler()), "addInstance", func(_ *FunctionInstancesMap, funcKey string, insSpec *types.InstanceSpecification) {
			addFuncKey = funcKey
			addInstanceTrigger = true
			addInsntaceId = insSpec.InstanceID
		}).Reset()
		event.Value = getInsSpecBytes(getCommonInsSpec())
		ProcessInstanceUpdate(event)
		convey.So(addFuncKey, convey.ShouldEqual, "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest")
		convey.So(addInstanceTrigger, convey.ShouldBeTrue)
		convey.So(addInsntaceId, convey.ShouldEqual, "5f000000-0000-4000-824c-75b4b7dae0a3")
		convey.So(delInstanceTrigger, convey.ShouldBeFalse)
		addInstanceTrigger = false

		insSpec = getCommonInsSpec()
		insSpec.InstanceStatus.Code = 2
		event.Value = getInsSpecBytes(insSpec)
		ProcessInstanceUpdate(event)
		convey.So(delFuncKey, convey.ShouldEqual, "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest")
		convey.So(delInstanceTrigger, convey.ShouldBeTrue)
		convey.So(delInstanceId, convey.ShouldEqual, "5f000000-0000-4000-824c-75b4b7dae0a3")
		convey.So(addInstanceTrigger, convey.ShouldBeFalse)
	})
}

func Test_ProcessInstanceDelete(t *testing.T) {
	convey.Convey("Test_ProcessInstanceUpdate simple", t, func() {
		insSpec := getCommonInsSpec()
		event := &etcd3.Event{
			Key:       getCommonEventKey(),
			PrevValue: getInsSpecBytes(insSpec),
			Rev:       0,
			ETCDType:  "",
		}

		processAppInfoDeleteTrigger := false
		defer gomonkey.ApplyFunc(ProcessAppInfoDelete, func(event2 *etcd3.Event) {
			processAppInfoDeleteTrigger = true
		}).Reset()

		ProcessInstanceDelete(event)
		convey.So(processAppInfoDeleteTrigger, convey.ShouldBeTrue)
		processAppInfoDeleteTrigger = false

		event.Key = "/sn/instance/business/yrk/tenant/12345678901234561234567890123456/function/0-system-faasExecutorGo1.x/version/$latest/defaultaz/787b900780b2d80600"
		ProcessInstanceDelete(event)
		convey.So(processAppInfoDeleteTrigger, convey.ShouldBeFalse)

		delFuncKey := ""
		delInstanceTrigger := false
		delInsntaceId := ""
		defer gomonkey.ApplyPrivateMethod(reflect.TypeOf(GetGlobalInstanceScheduler()), "delInstance", func(_ *FunctionInstancesMap, funcKey string, insSpec *types.InstanceSpecification) {
			delFuncKey = funcKey
			delInstanceTrigger = true
			delInsntaceId = insSpec.InstanceID
		}).Reset()

		event.Key = getCommonEventKey()
		event.PrevValue = getInsSpecBytes(getCommonInsSpec())
		ProcessInstanceDelete(event)
		convey.So(delFuncKey, convey.ShouldEqual, "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest")
		convey.So(delInstanceTrigger, convey.ShouldBeTrue)
		convey.So(delInsntaceId, convey.ShouldEqual, "5f000000-0000-4000-824c-75b4b7dae0a3")
		delInstanceTrigger = false

		insSpec.CreateOptions[functionKeyNote] = ""
		event.PrevValue = getInsSpecBytes(insSpec)
		ProcessInstanceDelete(event)
		convey.So(delInstanceTrigger, convey.ShouldBeFalse)
	})
}
