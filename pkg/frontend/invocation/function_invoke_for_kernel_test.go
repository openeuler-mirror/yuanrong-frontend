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

package invocation

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/resspeckey"
	"frontend/pkg/common/faas_common/snerror"
	"frontend/pkg/common/faas_common/statuscode"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/instancemanager"
	"frontend/pkg/frontend/leaseadaptor"
	"frontend/pkg/frontend/responsehandler"
	"frontend/pkg/frontend/schedulerproxy"
	types2 "frontend/pkg/frontend/types"
	"frontend/pkg/frontend/upgradecompatible"
	"frontend/pkg/frontend/wisecloud"
)

func Test_getAcquireReqCPUAndMemory(t *testing.T) {
	convey.Convey("Test_getAcquireReqCPUAndMemory", t, func() {
		funcSpec := &types.FuncSpec{
			ResourceMetaData: types.ResourceMetaData{
				CPU:    100,
				Memory: 100,
			},
		}

		ctx := &types2.InvokeProcessContext{}

		cpu, memory := getAcquireReqCPUAndMemory(ctx, funcSpec)
		convey.So(cpu, convey.ShouldEqual, 100)
		convey.So(memory, convey.ShouldEqual, 100)

		ctx.ReqHeader = make(map[string]string)
		ctx.ReqHeader[constant.HeaderCPUSize] = "200"
		ctx.ReqHeader[constant.HeaderMemorySize] = "200"

		cpu, memory = getAcquireReqCPUAndMemory(ctx, funcSpec)
		convey.So(cpu, convey.ShouldEqual, 200)
		convey.So(memory, convey.ShouldEqual, 200)

		ctx.ReqHeader[constant.HeaderCPUSize] = "200dfa"
		ctx.ReqHeader[constant.HeaderMemorySize] = "200dafadsf"
		cpu, memory = getAcquireReqCPUAndMemory(ctx, funcSpec)
		convey.So(cpu, convey.ShouldEqual, 100)
		convey.So(memory, convey.ShouldEqual, 100)
	})
}

func Test_convertResSpecKey(t *testing.T) {
	convey.Convey("Test_convertResSpecKey", t, func() {
		funcSpec := &types.FuncSpec{
			ResourceMetaData: types.ResourceMetaData{
				CPU:                 100,
				Memory:              100,
				CustomResources:     "{}",
				CustomResourcesSpec: "{}",
			},
		}

		ctx := &types2.InvokeProcessContext{
			ReqHeader: make(map[string]string),
		}

		resKey := convertResSpecKey(ctx, funcSpec)
		convey.So(resKey.String(), convey.ShouldEqual, "cpu-100-mem-100-storage-0-cstRes--cstResSpec--invokeLabel-")

		ctx.ReqHeader[constant.HeaderCPUSize] = "200"
		ctx.ReqHeader[constant.HeaderMemorySize] = "200"

		resKey = convertResSpecKey(ctx, funcSpec)
		convey.So(resKey.String(), convey.ShouldEqual, "cpu-200-mem-200-storage-0-cstRes--cstResSpec--invokeLabel-")

		ctx.ReqHeader[httpconstant.HeaderInstanceLabel] = "labeltest"
		funcSpec.ResourceMetaData.CustomResourcesSpec = "crspec000"
		funcSpec.ResourceMetaData.CustomResources = "cr000"
		funcSpec.ResourceMetaData.EphemeralStorage = 321
		resKey = convertResSpecKey(ctx, funcSpec)
		convey.So(resKey.String(), convey.ShouldEqual, "cpu-200-mem-200-storage-321-cstRes--cstResSpec--invokeLabel-labeltest")
	})
}

func clearSchedulerProxy() {
	for {
		schedulerInfo, err := schedulerproxy.Proxy.Get("0/0/0", log.GetLogger())
		if err != nil {
			return
		}
		schedulerproxy.Proxy.Remove(schedulerInfo.InstanceInfo, log.GetLogger())
	}
}

func mockSchedulerProxyAdd(id string) {
	schedulerInfo := &schedulerproxy.SchedulerNodeInfo{
		InstanceInfo: &types.InstanceInfo{
			TenantID:     id,
			FunctionName: id,
			Version:      id,
			InstanceName: id,
			InstanceID:   id,
			Address:      id,
		},
		UpdateTime: time.Now(),
	}
	schedulerproxy.Proxy.Add(schedulerInfo, log.GetLogger())
}

func mockSchedulerProxyRemove(id string) {
	schedulerproxy.Proxy.Remove(&types.InstanceInfo{
		TenantID:     id,
		FunctionName: id,
		Version:      id,
		InstanceName: id,
		InstanceID:   id,
		Address:      id,
	}, log.GetLogger())
}

func mockFunctionInstanceAdd(instanceId string) {
	event := &etcd3.Event{}
	event.Key = "/sn/instance/business/yrk/tenant/12345678901234561234567890123456/function/0-system-faasExecutorGo1.x/version/$latest/defaultaz/787b900780b2d80600/" + instanceId
	event.Value = []byte("{\"instanceID\":\"" + instanceId + "\",\"requestID\":\"787b900780b2d80600\",\"runtimeID\":\"runtime-5f000000-0000-4000-824c-75b4b7dae0a3-0000000074dd\",\"runtimeAddress\":\"127.0.0.1:32568\",\"functionAgentID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"functionProxyID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"function\":\"default/0-system-faasExecutorGo1.x/$latest\",\"resources\":{\"resources\":{\"CPU\":{\"name\":\"CPU\",\"scalar\":{\"value\":500}},\"Memory\":{\"name\":\"Memory\",\"scalar\":{\"value\":500}}}},\"scheduleOption\":{\"schedPolicyName\":\"monopoly\",\"affinity\":{\"instanceAffinity\":{},\"resource\":{},\"instance\":{\"scope\":\"NODE\"}},\"initCallTimeOut\":305,\"resourceSelector\":{\"resource.owner\":\"1c50bc05-0000-4000-8000-00ed778a549c\"},\"extension\":{\"schedule_policy\":\"monopoly\"},\"range\":{},\"scheduleTimeoutMs\":\"5000\"},\"createOptions\":{\"INSTANCE_LABEL_NOTE\":\"\",\"DELEGATE_DECRYPT\":\"{\\\"accessKey\\\":\\\"\\\",\\\"authToken\\\":\\\"\\\",\\\"cryptoAlgorithm\\\":\\\"\\\",\\\"encrypted_user_data\\\":\\\"\\\",\\\"envKey\\\":\\\"\\\",\\\"environment\\\":\\\"\\\",\\\"secretKey\\\":\\\"\\\",\\\"securityAk\\\":\\\"\\\",\\\"securitySk\\\":\\\"\\\",\\\"securityToken\\\":\\\"\\\"}\",\"lifecycle\":\"detached\",\"resource.owner\":\"static_function\",\"FUNCTION_KEY_NOTE\":\"8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest\",\"ConcurrentNum\":\"1000\",\"tenantId\":\"8d86c63b22e24d9ab650878b75408ea6\",\"INSTANCE_TYPE_NOTE\":\"reserved\",\"init_call_timeout\":\"305\",\"call_timeout\":\"60\",\"RESOURCE_SPEC_NOTE\":\"{\\\"cpu\\\":500,\\\"invokeLabels\\\":\\\"\\\",\\\"memory\\\":500}\",\"DELEGATE_DIRECTORY_QUOTA\":\"512\",\"GRACEFUL_SHUTDOWN_TIME\":\"900\",\"DELEGATE_DIRECTORY_INFO\":\"/tmp\"},\"instanceStatus\":{\"code\":3,\"msg\":\"running\"},\"schedulerChain\":[\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"],\"parentID\":\"static_function\",\"parentFunctionProxyAID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt-LocalSchedInstanceCtrlActor@10.158.98.238:22423\",\"storageType\":\"local\",\"scheduleTimes\":1,\"deployTimes\":1,\"args\":[{\"value\":\"EkdAAVpDMTIzNDU2Nzg5MDEyMzQ1NjEyMzQ1Njc4OTAxMjM0NTYvMC1zeXN0ZW0tZmFhc0V4ZWN1dG9yR28xLngvJGxhdGVzdBplEgASBy9pbnZva2UYAiD///////////8BKGQwAUJHCAMSQzEyMzQ1Njc4OTAxMjM0NTYxMjM0NTY3ODkwMTIzNDU2LzAtc3lzdGVtLWZhYXNFeGVjdXRvckdvMS54LyRsYXRlc3Q=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiZnVuY01ldGFEYXRhIjp7Im5hbWUiOiIwQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5IiwiZnVuY3Rpb25Vcm4iOiJzbjpjbjp5cms6OGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTY6ZnVuY3Rpb246MEBkZWZhdWx0QGZ1bmM2YWM2NzQxYTAxMzM0MzIwODA5ZGZiN2RjMWU5ODA0OSIsImZ1bmN0aW9uVmVyc2lvblVybiI6InNuOmNuOnlyazo4ZDg2YzYzYjIyZTI0ZDlhYjY1MDg3OGI3NTQwOGVhNjpmdW5jdGlvbjowQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5OmxhdGVzdCIsInZlcnNpb24iOiJsYXRlc3QiLCJmdW5jdGlvblVwZGF0ZVRpbWUiOiIyMDI1LTA2LTIzIDIzOjQ0OjIyLjAwMCIsInJ1bnRpbWUiOiJjdXN0b20gaW1hZ2UiLCJoYW5kbGVyIjoiL2ludm9rZSIsInRpbWVvdXQiOjYwLCJzZXJ2aWNlIjoiZGVmYXVsdCIsInRlbmFudElkIjoiOGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTYiLCJidXNpbmVzc0lkIjoieXJrIiwicmV2aXNpb25JZCI6IjIwMjUwNjIzMTU0NDIyMDEyIiwiZnVuY19uYW1lIjoiZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5In0sImVudk1ldGFEYXRhIjp7ImVudmlyb25tZW50IjoiIn0sImluc3RhbmNlTWV0YURhdGEiOnsibWF4SW5zdGFuY2UiOjEwMCwibWluSW5zdGFuY2UiOjEsImNvbmN1cnJlbnROdW0iOjEwMDAsInNjYWxlUG9saWN5Ijoic3RhdGljRnVuY3Rpb24ifSwicmVzb3VyY2VNZXRhRGF0YSI6eyJjcHUiOjUwMCwibWVtb3J5Ijo1MDB9LCJjb2RlTWV0YURhdGEiOnsic3RvcmFnZV90eXBlIjoiIn0sImV4dGVuZGVkTWV0YURhdGEiOnsiaW5pdGlhbGl6ZXIiOnsiaW5pdGlhbGl6ZXJfdGltZW91dCI6MzAwLCJpbml0aWFsaXplcl9oYW5kbGVyIjoiIn0sImN1c3RvbV9jb250YWluZXJfY29uZmlnIjp7ImltYWdlIjoic3dyLmNuLXNvdXRod2VzdC0yLm15aHVhd2VpY2xvdWQuY29tL3dpc2VmdW5jdGlvbi9jdXN0b20taW1hZ2U6MS4xLjEzLjIwMjUwNTA2MTczNDEyIn19fQ==\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiY2FsbFJvdXRlIjoiaW52b2tlIiwicG9ydCI6ODAwMH0=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsic2NoZWR1bGVyRnVuY0tleSI6IiIsInNjaGVkdWxlcklETGlzdCI6W119\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"}],\"version\":\"3\",\"dataSystemHost\":\"10.158.97.96\",\"gracefulShutdownTime\":\"600\",\"tenantID\":\"8d86c63b22e24d9ab650878b75408ea6\",\"extensions\":{\"receivedTimestamp\":\"1750782213307\",\"podDeploymentName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz\",\"podName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"pid\":\"71\",\"podNamespace\":\"wisefunctionservice-495f57a3-09ee-44d2-87e5-a109cda4dc40\",\"createTimestamp\":\"1750782213\",\"updateTimestamp\":\"1750782231\"},\"unitID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"}")

	instancemanager.ProcessInstanceUpdate(event)
}
func mockFunctionInstanceRemove(instanceId string) {
	event := &etcd3.Event{}
	event.Key = "/sn/instance/business/yrk/tenant/12345678901234561234567890123456/function/0-system-faasExecutorGo1.x/version/$latest/defaultaz/787b900780b2d80600/" + instanceId
	event.PrevValue = []byte("{\"instanceID\":\"" + instanceId + "\",\"requestID\":\"787b900780b2d80600\",\"runtimeID\":\"runtime-5f000000-0000-4000-824c-75b4b7dae0a3-0000000074dd\",\"runtimeAddress\":\"127.0.0.1:32568\",\"functionAgentID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"functionProxyID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"function\":\"default/0-system-faasExecutorGo1.x/$latest\",\"resources\":{\"resources\":{\"CPU\":{\"name\":\"CPU\",\"scalar\":{\"value\":500}},\"Memory\":{\"name\":\"Memory\",\"scalar\":{\"value\":500}}}},\"scheduleOption\":{\"schedPolicyName\":\"monopoly\",\"affinity\":{\"instanceAffinity\":{},\"resource\":{},\"instance\":{\"scope\":\"NODE\"}},\"initCallTimeOut\":305,\"resourceSelector\":{\"resource.owner\":\"1c50bc05-0000-4000-8000-00ed778a549c\"},\"extension\":{\"schedule_policy\":\"monopoly\"},\"range\":{},\"scheduleTimeoutMs\":\"5000\"},\"createOptions\":{\"INSTANCE_LABEL_NOTE\":\"\",\"DELEGATE_DECRYPT\":\"{\\\"accessKey\\\":\\\"\\\",\\\"authToken\\\":\\\"\\\",\\\"cryptoAlgorithm\\\":\\\"\\\",\\\"encrypted_user_data\\\":\\\"\\\",\\\"envKey\\\":\\\"\\\",\\\"environment\\\":\\\"\\\",\\\"secretKey\\\":\\\"\\\",\\\"securityAk\\\":\\\"\\\",\\\"securitySk\\\":\\\"\\\",\\\"securityToken\\\":\\\"\\\"}\",\"lifecycle\":\"detached\",\"resource.owner\":\"static_function\",\"FUNCTION_KEY_NOTE\":\"8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest\",\"ConcurrentNum\":\"1000\",\"tenantId\":\"8d86c63b22e24d9ab650878b75408ea6\",\"INSTANCE_TYPE_NOTE\":\"reserved\",\"init_call_timeout\":\"305\",\"call_timeout\":\"60\",\"RESOURCE_SPEC_NOTE\":\"{\\\"cpu\\\":500,\\\"invokeLabels\\\":\\\"\\\",\\\"memory\\\":500}\",\"DELEGATE_DIRECTORY_QUOTA\":\"512\",\"GRACEFUL_SHUTDOWN_TIME\":\"900\",\"DELEGATE_DIRECTORY_INFO\":\"/tmp\"},\"instanceStatus\":{\"code\":3,\"msg\":\"running\"},\"schedulerChain\":[\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"],\"parentID\":\"static_function\",\"parentFunctionProxyAID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt-LocalSchedInstanceCtrlActor@10.158.98.238:22423\",\"storageType\":\"local\",\"scheduleTimes\":1,\"deployTimes\":1,\"args\":[{\"value\":\"EkdAAVpDMTIzNDU2Nzg5MDEyMzQ1NjEyMzQ1Njc4OTAxMjM0NTYvMC1zeXN0ZW0tZmFhc0V4ZWN1dG9yR28xLngvJGxhdGVzdBplEgASBy9pbnZva2UYAiD///////////8BKGQwAUJHCAMSQzEyMzQ1Njc4OTAxMjM0NTYxMjM0NTY3ODkwMTIzNDU2LzAtc3lzdGVtLWZhYXNFeGVjdXRvckdvMS54LyRsYXRlc3Q=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiZnVuY01ldGFEYXRhIjp7Im5hbWUiOiIwQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5IiwiZnVuY3Rpb25Vcm4iOiJzbjpjbjp5cms6OGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTY6ZnVuY3Rpb246MEBkZWZhdWx0QGZ1bmM2YWM2NzQxYTAxMzM0MzIwODA5ZGZiN2RjMWU5ODA0OSIsImZ1bmN0aW9uVmVyc2lvblVybiI6InNuOmNuOnlyazo4ZDg2YzYzYjIyZTI0ZDlhYjY1MDg3OGI3NTQwOGVhNjpmdW5jdGlvbjowQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5OmxhdGVzdCIsInZlcnNpb24iOiJsYXRlc3QiLCJmdW5jdGlvblVwZGF0ZVRpbWUiOiIyMDI1LTA2LTIzIDIzOjQ0OjIyLjAwMCIsInJ1bnRpbWUiOiJjdXN0b20gaW1hZ2UiLCJoYW5kbGVyIjoiL2ludm9rZSIsInRpbWVvdXQiOjYwLCJzZXJ2aWNlIjoiZGVmYXVsdCIsInRlbmFudElkIjoiOGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTYiLCJidXNpbmVzc0lkIjoieXJrIiwicmV2aXNpb25JZCI6IjIwMjUwNjIzMTU0NDIyMDEyIiwiZnVuY19uYW1lIjoiZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5In0sImVudk1ldGFEYXRhIjp7ImVudmlyb25tZW50IjoiIn0sImluc3RhbmNlTWV0YURhdGEiOnsibWF4SW5zdGFuY2UiOjEwMCwibWluSW5zdGFuY2UiOjEsImNvbmN1cnJlbnROdW0iOjEwMDAsInNjYWxlUG9saWN5Ijoic3RhdGljRnVuY3Rpb24ifSwicmVzb3VyY2VNZXRhRGF0YSI6eyJjcHUiOjUwMCwibWVtb3J5Ijo1MDB9LCJjb2RlTWV0YURhdGEiOnsic3RvcmFnZV90eXBlIjoiIn0sImV4dGVuZGVkTWV0YURhdGEiOnsiaW5pdGlhbGl6ZXIiOnsiaW5pdGlhbGl6ZXJfdGltZW91dCI6MzAwLCJpbml0aWFsaXplcl9oYW5kbGVyIjoiIn0sImN1c3RvbV9jb250YWluZXJfY29uZmlnIjp7ImltYWdlIjoic3dyLmNuLXNvdXRod2VzdC0yLm15aHVhd2VpY2xvdWQuY29tL3dpc2VmdW5jdGlvbi9jdXN0b20taW1hZ2U6MS4xLjEzLjIwMjUwNTA2MTczNDEyIn19fQ==\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiY2FsbFJvdXRlIjoiaW52b2tlIiwicG9ydCI6ODAwMH0=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsic2NoZWR1bGVyRnVuY0tleSI6IiIsInNjaGVkdWxlcklETGlzdCI6W119\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"}],\"version\":\"3\",\"dataSystemHost\":\"10.158.97.96\",\"gracefulShutdownTime\":\"600\",\"tenantID\":\"8d86c63b22e24d9ab650878b75408ea6\",\"extensions\":{\"receivedTimestamp\":\"1750782213307\",\"podDeploymentName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz\",\"podName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"pid\":\"71\",\"podNamespace\":\"wisefunctionservice-495f57a3-09ee-44d2-87e5-a109cda4dc40\",\"createTimestamp\":\"1750782213\",\"updateTimestamp\":\"1750782231\"},\"unitID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"}")

	instancemanager.ProcessInstanceDelete(event)
}

func mockSchedulerInstanceAdd(key string) {
	event := &etcd3.Event{}
	event.Key = fmt.Sprintf("/sn/instance/business/yrk/tenant/0/function/0-system-faasscheduler/version/$latest/defaultaz//%s", key)
	event.Value = []byte("{\"instanceID\":\"5f000000-0000-4000-824c-75b4b7dae0a3\",\"requestID\":\"787b900780b2d80600\",\"runtimeID\":\"runtime-5f000000-0000-4000-824c-75b4b7dae0a3-0000000074dd\",\"runtimeAddress\":\"127.0.0.1:32568\",\"functionAgentID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"functionProxyID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"function\":\"default/0-system-faasExecutorGo1.x/$latest\",\"resources\":{\"resources\":{\"CPU\":{\"name\":\"CPU\",\"scalar\":{\"value\":500}},\"Memory\":{\"name\":\"Memory\",\"scalar\":{\"value\":500}}}},\"scheduleOption\":{\"schedPolicyName\":\"monopoly\",\"affinity\":{\"instanceAffinity\":{},\"resource\":{},\"instance\":{\"scope\":\"NODE\"}},\"initCallTimeOut\":305,\"resourceSelector\":{\"resource.owner\":\"1c50bc05-0000-4000-8000-00ed778a549c\"},\"extension\":{\"schedule_policy\":\"monopoly\"},\"range\":{},\"scheduleTimeoutMs\":\"5000\"},\"createOptions\":{\"INSTANCE_LABEL_NOTE\":\"\",\"DELEGATE_DECRYPT\":\"{\\\"accessKey\\\":\\\"\\\",\\\"authToken\\\":\\\"\\\",\\\"cryptoAlgorithm\\\":\\\"\\\",\\\"encrypted_user_data\\\":\\\"\\\",\\\"envKey\\\":\\\"\\\",\\\"environment\\\":\\\"\\\",\\\"secretKey\\\":\\\"\\\",\\\"securityAk\\\":\\\"\\\",\\\"securitySk\\\":\\\"\\\",\\\"securityToken\\\":\\\"\\\"}\",\"lifecycle\":\"detached\",\"resource.owner\":\"static_function\",\"FUNCTION_KEY_NOTE\":\"8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest\",\"ConcurrentNum\":\"1000\",\"tenantId\":\"8d86c63b22e24d9ab650878b75408ea6\",\"INSTANCE_TYPE_NOTE\":\"reserved\",\"init_call_timeout\":\"305\",\"call_timeout\":\"60\",\"RESOURCE_SPEC_NOTE\":\"{\\\"cpu\\\":500,\\\"invokeLabels\\\":\\\"\\\",\\\"memory\\\":500}\",\"DELEGATE_DIRECTORY_QUOTA\":\"512\",\"GRACEFUL_SHUTDOWN_TIME\":\"900\",\"DELEGATE_DIRECTORY_INFO\":\"/tmp\"},\"instanceStatus\":{\"code\":3,\"msg\":\"running\"},\"schedulerChain\":[\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"],\"parentID\":\"static_function\",\"parentFunctionProxyAID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt-LocalSchedInstanceCtrlActor@10.158.98.238:22423\",\"storageType\":\"local\",\"scheduleTimes\":1,\"deployTimes\":1,\"args\":[{\"value\":\"EkdAAVpDMTIzNDU2Nzg5MDEyMzQ1NjEyMzQ1Njc4OTAxMjM0NTYvMC1zeXN0ZW0tZmFhc0V4ZWN1dG9yR28xLngvJGxhdGVzdBplEgASBy9pbnZva2UYAiD///////////8BKGQwAUJHCAMSQzEyMzQ1Njc4OTAxMjM0NTYxMjM0NTY3ODkwMTIzNDU2LzAtc3lzdGVtLWZhYXNFeGVjdXRvckdvMS54LyRsYXRlc3Q=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiZnVuY01ldGFEYXRhIjp7Im5hbWUiOiIwQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5IiwiZnVuY3Rpb25Vcm4iOiJzbjpjbjp5cms6OGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTY6ZnVuY3Rpb246MEBkZWZhdWx0QGZ1bmM2YWM2NzQxYTAxMzM0MzIwODA5ZGZiN2RjMWU5ODA0OSIsImZ1bmN0aW9uVmVyc2lvblVybiI6InNuOmNuOnlyazo4ZDg2YzYzYjIyZTI0ZDlhYjY1MDg3OGI3NTQwOGVhNjpmdW5jdGlvbjowQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5OmxhdGVzdCIsInZlcnNpb24iOiJsYXRlc3QiLCJmdW5jdGlvblVwZGF0ZVRpbWUiOiIyMDI1LTA2LTIzIDIzOjQ0OjIyLjAwMCIsInJ1bnRpbWUiOiJjdXN0b20gaW1hZ2UiLCJoYW5kbGVyIjoiL2ludm9rZSIsInRpbWVvdXQiOjYwLCJzZXJ2aWNlIjoiZGVmYXVsdCIsInRlbmFudElkIjoiOGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTYiLCJidXNpbmVzc0lkIjoieXJrIiwicmV2aXNpb25JZCI6IjIwMjUwNjIzMTU0NDIyMDEyIiwiZnVuY19uYW1lIjoiZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5In0sImVudk1ldGFEYXRhIjp7ImVudmlyb25tZW50IjoiIn0sImluc3RhbmNlTWV0YURhdGEiOnsibWF4SW5zdGFuY2UiOjEwMCwibWluSW5zdGFuY2UiOjEsImNvbmN1cnJlbnROdW0iOjEwMDAsInNjYWxlUG9saWN5Ijoic3RhdGljRnVuY3Rpb24ifSwicmVzb3VyY2VNZXRhRGF0YSI6eyJjcHUiOjUwMCwibWVtb3J5Ijo1MDB9LCJjb2RlTWV0YURhdGEiOnsic3RvcmFnZV90eXBlIjoiIn0sImV4dGVuZGVkTWV0YURhdGEiOnsiaW5pdGlhbGl6ZXIiOnsiaW5pdGlhbGl6ZXJfdGltZW91dCI6MzAwLCJpbml0aWFsaXplcl9oYW5kbGVyIjoiIn0sImN1c3RvbV9jb250YWluZXJfY29uZmlnIjp7ImltYWdlIjoic3dyLmNuLXNvdXRod2VzdC0yLm15aHVhd2VpY2xvdWQuY29tL3dpc2VmdW5jdGlvbi9jdXN0b20taW1hZ2U6MS4xLjEzLjIwMjUwNTA2MTczNDEyIn19fQ==\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiY2FsbFJvdXRlIjoiaW52b2tlIiwicG9ydCI6ODAwMH0=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsic2NoZWR1bGVyRnVuY0tleSI6IiIsInNjaGVkdWxlcklETGlzdCI6W119\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"}],\"version\":\"3\",\"dataSystemHost\":\"10.158.97.96\",\"gracefulShutdownTime\":\"600\",\"tenantID\":\"8d86c63b22e24d9ab650878b75408ea6\",\"extensions\":{\"receivedTimestamp\":\"1750782213307\",\"podDeploymentName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz\",\"podName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"pid\":\"71\",\"podNamespace\":\"wisefunctionservice-495f57a3-09ee-44d2-87e5-a109cda4dc40\",\"createTimestamp\":\"1750782213\",\"updateTimestamp\":\"1750782231\"},\"unitID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"}")

	instancemanager.ProcessInstanceUpdate(event)
}

func mockSchedulerInstanceRemove(key string) {
	event := &etcd3.Event{}
	event.Key = fmt.Sprintf("/sn/instance/business/yrk/tenant/0/function/0-system-faasscheduler/version/$latest/defaultaz//%s", key)
	event.PrevValue = []byte("{\"instanceID\":\"5f000000-0000-4000-824c-75b4b7dae0a3\",\"requestID\":\"787b900780b2d80600\",\"runtimeID\":\"runtime-5f000000-0000-4000-824c-75b4b7dae0a3-0000000074dd\",\"runtimeAddress\":\"127.0.0.1:32568\",\"functionAgentID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"functionProxyID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"function\":\"default/0-system-faasExecutorGo1.x/$latest\",\"resources\":{\"resources\":{\"CPU\":{\"name\":\"CPU\",\"scalar\":{\"value\":500}},\"Memory\":{\"name\":\"Memory\",\"scalar\":{\"value\":500}}}},\"scheduleOption\":{\"schedPolicyName\":\"monopoly\",\"affinity\":{\"instanceAffinity\":{},\"resource\":{},\"instance\":{\"scope\":\"NODE\"}},\"initCallTimeOut\":305,\"resourceSelector\":{\"resource.owner\":\"1c50bc05-0000-4000-8000-00ed778a549c\"},\"extension\":{\"schedule_policy\":\"monopoly\"},\"range\":{},\"scheduleTimeoutMs\":\"5000\"},\"createOptions\":{\"INSTANCE_LABEL_NOTE\":\"\",\"DELEGATE_DECRYPT\":\"{\\\"accessKey\\\":\\\"\\\",\\\"authToken\\\":\\\"\\\",\\\"cryptoAlgorithm\\\":\\\"\\\",\\\"encrypted_user_data\\\":\\\"\\\",\\\"envKey\\\":\\\"\\\",\\\"environment\\\":\\\"\\\",\\\"secretKey\\\":\\\"\\\",\\\"securityAk\\\":\\\"\\\",\\\"securitySk\\\":\\\"\\\",\\\"securityToken\\\":\\\"\\\"}\",\"lifecycle\":\"detached\",\"resource.owner\":\"static_function\",\"FUNCTION_KEY_NOTE\":\"8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest\",\"ConcurrentNum\":\"1000\",\"tenantId\":\"8d86c63b22e24d9ab650878b75408ea6\",\"INSTANCE_TYPE_NOTE\":\"reserved\",\"init_call_timeout\":\"305\",\"call_timeout\":\"60\",\"RESOURCE_SPEC_NOTE\":\"{\\\"cpu\\\":500,\\\"invokeLabels\\\":\\\"\\\",\\\"memory\\\":500}\",\"DELEGATE_DIRECTORY_QUOTA\":\"512\",\"GRACEFUL_SHUTDOWN_TIME\":\"900\",\"DELEGATE_DIRECTORY_INFO\":\"/tmp\"},\"instanceStatus\":{\"code\":3,\"msg\":\"running\"},\"schedulerChain\":[\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"],\"parentID\":\"static_function\",\"parentFunctionProxyAID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt-LocalSchedInstanceCtrlActor@10.158.98.238:22423\",\"storageType\":\"local\",\"scheduleTimes\":1,\"deployTimes\":1,\"args\":[{\"value\":\"EkdAAVpDMTIzNDU2Nzg5MDEyMzQ1NjEyMzQ1Njc4OTAxMjM0NTYvMC1zeXN0ZW0tZmFhc0V4ZWN1dG9yR28xLngvJGxhdGVzdBplEgASBy9pbnZva2UYAiD///////////8BKGQwAUJHCAMSQzEyMzQ1Njc4OTAxMjM0NTYxMjM0NTY3ODkwMTIzNDU2LzAtc3lzdGVtLWZhYXNFeGVjdXRvckdvMS54LyRsYXRlc3Q=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiZnVuY01ldGFEYXRhIjp7Im5hbWUiOiIwQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5IiwiZnVuY3Rpb25Vcm4iOiJzbjpjbjp5cms6OGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTY6ZnVuY3Rpb246MEBkZWZhdWx0QGZ1bmM2YWM2NzQxYTAxMzM0MzIwODA5ZGZiN2RjMWU5ODA0OSIsImZ1bmN0aW9uVmVyc2lvblVybiI6InNuOmNuOnlyazo4ZDg2YzYzYjIyZTI0ZDlhYjY1MDg3OGI3NTQwOGVhNjpmdW5jdGlvbjowQGRlZmF1bHRAZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5OmxhdGVzdCIsInZlcnNpb24iOiJsYXRlc3QiLCJmdW5jdGlvblVwZGF0ZVRpbWUiOiIyMDI1LTA2LTIzIDIzOjQ0OjIyLjAwMCIsInJ1bnRpbWUiOiJjdXN0b20gaW1hZ2UiLCJoYW5kbGVyIjoiL2ludm9rZSIsInRpbWVvdXQiOjYwLCJzZXJ2aWNlIjoiZGVmYXVsdCIsInRlbmFudElkIjoiOGQ4NmM2M2IyMmUyNGQ5YWI2NTA4NzhiNzU0MDhlYTYiLCJidXNpbmVzc0lkIjoieXJrIiwicmV2aXNpb25JZCI6IjIwMjUwNjIzMTU0NDIyMDEyIiwiZnVuY19uYW1lIjoiZnVuYzZhYzY3NDFhMDEzMzQzMjA4MDlkZmI3ZGMxZTk4MDQ5In0sImVudk1ldGFEYXRhIjp7ImVudmlyb25tZW50IjoiIn0sImluc3RhbmNlTWV0YURhdGEiOnsibWF4SW5zdGFuY2UiOjEwMCwibWluSW5zdGFuY2UiOjEsImNvbmN1cnJlbnROdW0iOjEwMDAsInNjYWxlUG9saWN5Ijoic3RhdGljRnVuY3Rpb24ifSwicmVzb3VyY2VNZXRhRGF0YSI6eyJjcHUiOjUwMCwibWVtb3J5Ijo1MDB9LCJjb2RlTWV0YURhdGEiOnsic3RvcmFnZV90eXBlIjoiIn0sImV4dGVuZGVkTWV0YURhdGEiOnsiaW5pdGlhbGl6ZXIiOnsiaW5pdGlhbGl6ZXJfdGltZW91dCI6MzAwLCJpbml0aWFsaXplcl9oYW5kbGVyIjoiIn0sImN1c3RvbV9jb250YWluZXJfY29uZmlnIjp7ImltYWdlIjoic3dyLmNuLXNvdXRod2VzdC0yLm15aHVhd2VpY2xvdWQuY29tL3dpc2VmdW5jdGlvbi9jdXN0b20taW1hZ2U6MS4xLjEzLjIwMjUwNTA2MTczNDEyIn19fQ==\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsiY2FsbFJvdXRlIjoiaW52b2tlIiwicG9ydCI6ODAwMH0=\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHsic2NoZWR1bGVyRnVuY0tleSI6IiIsInNjaGVkdWxlcklETGlzdCI6W119\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"},{\"value\":\"AAAAAAAAAAAAAAAAAAAAAHt9\"}],\"version\":\"3\",\"dataSystemHost\":\"10.158.97.96\",\"gracefulShutdownTime\":\"600\",\"tenantID\":\"8d86c63b22e24d9ab650878b75408ea6\",\"extensions\":{\"receivedTimestamp\":\"1750782213307\",\"podDeploymentName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz\",\"podName\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\",\"pid\":\"71\",\"podNamespace\":\"wisefunctionservice-495f57a3-09ee-44d2-87e5-a109cda4dc40\",\"createTimestamp\":\"1750782213\",\"updateTimestamp\":\"1750782231\"},\"unitID\":\"func6ac6741a01334320809dfb7dc1e98049-latest-yrhjz-pqjvt\"}")

	instancemanager.ProcessInstanceDelete(event)
}

func Test_needDownGrade(t *testing.T) {
	convey.Convey("Test_needDownGrade", t, func() {
		clearSchedulerProxy()
		instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer clearSchedulerProxy()
		defer instancemanager.GetFaaSSchedulerInstanceManager().Reset()

		schedulerInfo := &types.InstanceInfo{}
		convey.So(needDownGrade(schedulerInfo), convey.ShouldBeTrue)

		mockSchedulerProxyAdd("0")
		mockSchedulerInstanceAdd("1")
		convey.So(needDownGrade(schedulerInfo), convey.ShouldBeTrue)

		convey.So(needDownGrade(nil), convey.ShouldBeTrue)

		mockSchedulerInstanceAdd("0")
		convey.So(needDownGrade(schedulerInfo), convey.ShouldBeFalse)
		mockSchedulerProxyRemove("0")
		mockSchedulerInstanceRemove("0")
		mockSchedulerInstanceRemove("1")
	})
}

type fakeClient struct {
}

func (f *fakeClient) AcquireInstance(functionKey string, req types.AcquireOption) (*types.InstanceAllocationInfo, error) {
	// TODO implement me
	panic("implement me")
}

func (f *fakeClient) ReleaseInstance(allocation *types.InstanceAllocationInfo, abnormal bool) {
	// TODO implement me
	panic("implement me")
}

func (f *fakeClient) Invoke(req util.InvokeRequest) ([]byte, error) {
	// TODO implement me
	panic("implement me")
}

func (f *fakeClient) CreateInstanceRaw(createReq []byte, option api.RawRequestOption) ([]byte, error) {
	return nil, nil
}
func (f *fakeClient) InvokeInstanceRaw(invokeReq []byte, option api.RawRequestOption) ([]byte, error) {
	return nil, nil
}
func (f *fakeClient) KillRaw(killReq []byte, option api.RawRequestOption) ([]byte, error) {
	return nil, nil
}
func (c *fakeClient) CreateInstanceByLibRt(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (instanceID string, err error) {
	InstanceID := ""
	return InstanceID, nil
}
func (c *fakeClient) KillByLibRt(instanceID string, signal int, payload []byte) error {
	return nil
}

// InvokeByName copy from faasinvoker_test.go
func (f *fakeClient) InvokeByName(request util.InvokeRequest) ([]byte, error) {
	return nil, nil
}

func (f *fakeClient) IsHealth() bool {
	return true
}

func (f *fakeClient) IsDsHealth() bool {
	return true
}

func Test_invokeByClient(t *testing.T) {
	convey.Convey("Test_invokeByClient", t, func() {
		c := &fakeClient{}
		defer gomonkey.ApplyFunc(util.NewClient, func() util.Client {
			return c
		}).Reset()

		invokeTrigger := false
		invoekInstance := ""
		defer gomonkey.ApplyMethod(reflect.TypeOf(c), "Invoke", func(_ *fakeClient, req util.InvokeRequest) ([]byte, error) {
			invokeTrigger = true
			invoekInstance = req.InstanceID
			return nil, fmt.Errorf("")
		}).Reset()

		invokeByNameTrigger := false
		defer gomonkey.ApplyMethod(reflect.TypeOf(c), "InvokeByName", func(_ *fakeClient, req util.InvokeRequest) ([]byte, error) {
			invokeByNameTrigger = true
			return nil, fmt.Errorf("")
		}).Reset()

		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
		}
		req := util.InvokeRequest{
			InstanceID: "0",
		}

		invokeFunctionWithLibRuntime(ctx, req, log.GetLogger())
		convey.So(invokeTrigger, convey.ShouldBeTrue)
		convey.So(invoekInstance, convey.ShouldEqual, "0")
		convey.So(invokeByNameTrigger, convey.ShouldBeFalse)

		invokeTrigger = false
		req.InstanceID = ""
		invokeFunctionWithLibRuntime(ctx, req, log.GetLogger())
		convey.So(invokeTrigger, convey.ShouldBeFalse)
		convey.So(invokeByNameTrigger, convey.ShouldBeTrue)
	})
}

func Test_functionInvokeForKernel(t *testing.T) {
	convey.Convey("Test_functionInvokeForKernel", t, func() {
		responsehandler.Handler = (&FGAdapter{}).MakeResponseHandler()
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
			RespHeader:    make(map[string]string),
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500

		clearSchedulerProxy()
		instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer clearSchedulerProxy()
		defer instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer gomonkey.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(funcKey string, resSpec *resspeckey.ResSpecKey, pendingReq *wisecloud.PendingRequest) {
			log.GetLogger().Infof("debug show addpending request")
			pendingReq.ResultChan <- &wisecloud.PendingResponse{Instance: nil}
		}).Reset()

		err := newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(strings.Contains(err.Error(), "no available instance, no available scheduler"), convey.ShouldBeTrue)

		mockSchedulerProxyAdd("0")
		mockSchedulerInstanceAdd("0")
		var getreq util.InvokeRequest
		defer gomonkey.ApplyFunc(invokeFunctionWithLibRuntime, func(_ *types2.InvokeProcessContext, req util.InvokeRequest) snerror.SNError {
			getreq = req
			return nil
		}).Reset()
		p := gomonkey.ApplyFunc(needDownGrade, func() bool {
			return false
		})
		newKernelRequestHandler(ctx, funcSpec).invoke()
		p.Reset()
		convey.So(getreq.InstanceID, convey.ShouldEqual, "")
		getreq.SchedulerID = ""
		defer gomonkey.ApplyFunc(needDownGrade, func() bool {
			return true
		}).Reset()

		mockFunctionInstanceAdd("111")
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(getreq.InstanceID, convey.ShouldEqual, "111")
	})
}

func Test_functionInvokeForKernel_legacy(t *testing.T) {
	convey.Convey("Test_functionInvokeForKernel_legacy", t, func() {
		responsehandler.Handler = (&FGAdapter{}).MakeResponseHandler()
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
			RespHeader:    make(map[string]string),
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500

		clearSchedulerProxy()
		instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer clearSchedulerProxy()
		defer instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer gomonkey.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(funcKey string, resSpec *resspeckey.ResSpecKey, pendingReq *wisecloud.PendingRequest) {
			log.GetLogger().Infof("debug show addpending request")
			pendingReq.ResultChan <- &wisecloud.PendingResponse{Instance: nil}
		}).Reset()

		err := newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(strings.Contains(err.Error(), "no available instance, no available scheduler"), convey.ShouldBeTrue)

		mockSchedulerProxyAdd("0")
		mockSchedulerInstanceAdd("0")
		var getreq util.InvokeRequest
		defer gomonkey.ApplyFunc(invokeFunctionWithLibRuntime, func(_ *types2.InvokeProcessContext, req util.InvokeRequest) snerror.SNError {
			getreq = req
			return nil
		}).Reset()
		defer gomonkey.ApplyFunc(upgradecompatible.GetAccessFaaSSchedulerType, func() string {
			return "libruntime"
		}).Reset()

		p := gomonkey.ApplyFunc(needDownGrade, func() bool {
			return false
		})
		newKernelRequestHandler(ctx, funcSpec).invoke()
		p.Reset()
		convey.So(getreq.InstanceID, convey.ShouldEqual, "")
		getreq.SchedulerID = ""
		defer gomonkey.ApplyFunc(needDownGrade, func() bool {
			return true
		}).Reset()

		mockFunctionInstanceAdd("111")
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(getreq.InstanceID, convey.ShouldEqual, "111")
		convey.So(getreq.SchedulerID, convey.ShouldEqual, "0") // hash环上有节点，但是，没有scheduler实例
	})
}

func Test_functionInvokeForKernel_retry(t *testing.T) {
	convey.Convey("Test_functionInvokeForKernel_retry", t, func() {
		clearSchedulerProxy()
		instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer clearSchedulerProxy()
		defer instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		ctx.InvokeWithoutScheduler = true
		mockFunctionInstanceAdd("111")

		mockSchedulerProxyAdd("0")
		var getreq util.InvokeRequest
		p := gomonkey.ApplyFunc(invokeFunctionWithLibRuntime, func(_ *types2.InvokeProcessContext, req util.InvokeRequest) snerror.SNError {
			getreq = req
			return nil
		})
		metricsInvokeStartFlag := false
		metricsInvokeStartInstance := ""
		defer gomonkey.ApplyMethodFunc(reflect.TypeOf(wisecloud.GetMetricsManager()), "InvokeStart", func(funcKey string, resSpecKeyStr string, instanceId string) {
			metricsInvokeStartFlag = true
			metricsInvokeStartInstance = instanceId
		}).Reset()
		metricsInvokeEndFlag := false
		metricsInvokeEndInstance := ""
		defer gomonkey.ApplyMethodFunc(reflect.TypeOf(wisecloud.GetMetricsManager()), "InvokeEnd", func(funcKey string, resSpecKeyStr string, instanceId string) {
			metricsInvokeEndFlag = true
			metricsInvokeEndInstance = instanceId
		}).Reset()
		newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(getreq.InstanceID, convey.ShouldEqual, "111")
		convey.So(metricsInvokeStartFlag, convey.ShouldBeTrue)
		convey.So(metricsInvokeStartInstance, convey.ShouldEqual, "111")
		convey.So(metricsInvokeEndFlag, convey.ShouldBeTrue)
		convey.So(metricsInvokeEndInstance, convey.ShouldEqual, "111")
		p.Reset()

		// 比较复杂的用例
		// 构造只有一个scheduler
		// 首先，调用的请求，是走租约体系，则其schedulerId不为空，且instanceId为空。然后我们构造返回9009
		// 然后，重试调用请求，是走降级，则其schedulerId为空，且instanceId不为空。然后我们构造1003
		// 最后，再次重试调用请求，走降级，则其schedulerId为空，且instanceId不为空且不是上一次重试的instanceId。然后我们构造成功。
		times := 0
		var getreq1 util.InvokeRequest
		var getreq2 util.InvokeRequest
		mockSchedulerInstanceAdd("0")
		mockFunctionInstanceAdd("222")
		defer mockFunctionInstanceRemove("222")
		p = gomonkey.ApplyFunc(invokeFunctionWithLibRuntime, func(_ *types2.InvokeProcessContext, req util.InvokeRequest) snerror.SNError {
			times++
			if times == 1 {
				getreq1 = req
				time.Sleep(1*time.Second + 200*time.Millisecond)
				return snerror.New(statuscode.ErrInstanceEvicted, "")
			}
			if times == 2 {
				getreq2 = req
			}

			return nil
		})
		ctx.InvokeWithoutScheduler = false
		newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(times, convey.ShouldEqual, 2)
		convey.So([]string{"111", "222"}, convey.ShouldContain, getreq1.InstanceID)
		convey.So([]string{"111", "222"}, convey.ShouldContain, getreq2.InstanceID)
		convey.So(getreq1.InstanceID, convey.ShouldNotEqual, getreq2.InstanceID)
		convey.So(getreq2.InvokeTimeout, convey.ShouldBeLessThan, 10)

		ctx.InvokeTimeout = 1
		times = 0
		err := newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(strings.Contains(err.Error(), "do invoke failed, timeout"), convey.ShouldBeTrue)
		convey.So(times, convey.ShouldEqual, 1)
		p.Reset()
	})
}

func Test_functionInvokeForKernel_retry_legacy(t *testing.T) {
	convey.Convey("Test_functionInvokeForKernel_retry_legacy", t, func() {
		clearSchedulerProxy()
		instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer clearSchedulerProxy()
		defer instancemanager.GetFaaSSchedulerInstanceManager().Reset()
		defer gomonkey.ApplyFunc(upgradecompatible.GetAccessFaaSSchedulerType, func() string {
			return "libruntime"
		}).Reset()
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		ctx.InvokeWithoutScheduler = true
		mockFunctionInstanceAdd("111")

		mockSchedulerProxyAdd("0")
		var getreq util.InvokeRequest
		p := gomonkey.ApplyFunc(invokeFunctionWithLibRuntime, func(_ *types2.InvokeProcessContext, req util.InvokeRequest) snerror.SNError {
			getreq = req
			return nil
		})
		metricsInvokeStartFlag := false
		metricsInvokeStartInstance := ""
		defer gomonkey.ApplyMethodFunc(reflect.TypeOf(wisecloud.GetMetricsManager()), "InvokeStart", func(funcKey string, resSpecKeyStr string, instanceId string) {
			metricsInvokeStartFlag = true
			metricsInvokeStartInstance = instanceId
		}).Reset()
		metricsInvokeEndFlag := false
		metricsInvokeEndInstance := ""
		defer gomonkey.ApplyMethodFunc(reflect.TypeOf(wisecloud.GetMetricsManager()), "InvokeEnd", func(funcKey string, resSpecKeyStr string, instanceId string) {
			metricsInvokeEndFlag = true
			metricsInvokeEndInstance = instanceId
		}).Reset()
		newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(getreq.InstanceID, convey.ShouldEqual, "111")
		convey.So(metricsInvokeStartFlag, convey.ShouldBeTrue)
		convey.So(metricsInvokeStartInstance, convey.ShouldEqual, "111")
		convey.So(metricsInvokeEndFlag, convey.ShouldBeTrue)
		convey.So(metricsInvokeEndInstance, convey.ShouldEqual, "111")
		p.Reset()

		// 比较复杂的用例
		// 构造只有一个scheduler
		// 首先，调用的请求，是走租约体系，则其schedulerId不为空，且instanceId为空。然后我们构造返回9009
		// 然后，重试调用请求，是走降级，则其schedulerId为空，且instanceId不为空。然后我们构造1003
		// 最后，再次重试调用请求，走降级，则其schedulerId为空，且instanceId不为空且不是上一次重试的instanceId。然后我们构造成功。
		times := 0
		var getreq1 util.InvokeRequest
		var getreq2 util.InvokeRequest
		var getreq3 util.InvokeRequest
		mockSchedulerInstanceAdd("0")
		mockFunctionInstanceAdd("222")
		defer mockFunctionInstanceRemove("222")
		p = gomonkey.ApplyFunc(invokeFunctionWithLibRuntime, func(_ *types2.InvokeProcessContext, req util.InvokeRequest) snerror.SNError {
			times++
			if times == 1 {
				getreq1 = req
				time.Sleep(1*time.Second + 200*time.Millisecond)
				return snerror.New(statuscode.ErrAllSchedulerUnavailable, "")
			}
			if times == 2 {
				getreq2 = req
				return snerror.New(statuscode.ErrInstanceExitedCode, "")
			}
			if times == 3 {
				getreq3 = req
			}
			return nil
		})
		ctx.InvokeWithoutScheduler = false
		newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(times, convey.ShouldEqual, 3)
		convey.So(getreq1.SchedulerID, convey.ShouldEqual, "0")
		convey.So(getreq2.SchedulerID, convey.ShouldBeEmpty)
		convey.So([]string{"111", "222"}, convey.ShouldContain, getreq2.InstanceID)
		convey.So(getreq3.SchedulerID, convey.ShouldBeEmpty)
		convey.So([]string{"111", "222"}, convey.ShouldContain, getreq3.InstanceID)
		convey.So(getreq2.InstanceID, convey.ShouldNotEqual, getreq3.InstanceID)
		convey.So(getreq3.InvokeTimeout, convey.ShouldBeLessThan, 10)

		ctx.InvokeTimeout = 1
		times = 0
		err := newKernelRequestHandler(ctx, funcSpec).invoke()
		convey.So(strings.Contains(err.Error(), "do invoke failed, timeout"), convey.ShouldBeTrue)
		convey.So(times, convey.ShouldEqual, 1)
		p.Reset()
	})
}

func TestInvokeInstanceNeedRetry(t *testing.T) {
	convey.Convey("Test needRetryCode function", t, func() {
		successStatusCodes := []int{
			statuscode.DsDeleteFailed,
			statuscode.DsDownloadFailed,
			statuscode.DsKeyNotFound,
		}

		retryErrorCodes := []int{
			statuscode.ErrInstanceNotFound,
			statuscode.ErrInstanceExitedCode,
			statuscode.ErrInstanceCircuitCode,
			statuscode.ErrInstanceEvicted,
			statuscode.ErrRequestBetweenRuntimeBusCode,
			statuscode.ErrInnerCommunication,
			statuscode.ErrSharedMemoryLimited,
			statuscode.ErrOperateDiskFailed,
			statuscode.ErrInsufficientDiskSpace,
			statuscode.ErrFinalized,
		}

		convey.Convey("When passing retry-required error codes", func() {
			for _, errCode := range retryErrorCodes {
				convey.Convey("Should return true for error code "+strconv.Itoa(errCode), func() {
					convey.So(needRetryCode(errCode), convey.ShouldBeTrue)
				})
			}
		})

		convey.Convey("When passing non-retry status codes", func() {
			for _, statusCode := range successStatusCodes {
				convey.Convey("Should return false for status code "+strconv.Itoa(statusCode), func() {
					convey.So(needRetryCode(statusCode), convey.ShouldBeFalse)
				})
			}
		})

		convey.Convey("When passing undefined status codes", func() {
			undefinedCodes := []int{9999, -1, 10000}
			for _, unknownCode := range undefinedCodes {
				convey.Convey("Should return false for unknown code "+strconv.Itoa(unknownCode), func() {
					convey.So(needRetryCode(unknownCode), convey.ShouldBeFalse)
				})
			}
		})
	})
}

func TestKernelRequestHandler_legacyMakeReq(t *testing.T) {
	convey.Convey("Test legacyMakeReq method", t, func() {
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		ctx.InvokeWithoutScheduler = true

		defer gomonkey.ApplyFunc(upgradecompatible.GetAccessFaaSSchedulerType, func() string {
			return "libruntime"
		}).Reset()

		handler := newKernelRequestHandler(ctx, funcSpec)

		// Mock补丁集合
		var patches *gomonkey.Patches

		convey.Convey("降级情况 - 无scheduler但能获取实例", func() {
			clearSchedulerProxy()
			mockFunctionInstanceAdd("111")
			defer mockFunctionInstanceRemove("111")
			req, err := handler.makeReq(log.GetLogger())

			convey.So(err, convey.ShouldBeNil)
			convey.So(req.InstanceID, convey.ShouldEqual, "111")
			convey.So(handler.downgrade, convey.ShouldBeTrue)
		})

		convey.Convey("降级情况 - 需要排队获取实例", func() {
			patches = gomonkey.NewPatches()
			defer patches.Reset()

			clearSchedulerProxy()

			// Mock 排队返回结果
			mockResponse := &wisecloud.PendingResponse{
				Instance: &types.InstanceSpecification{InstanceID: "222"},
				Error:    nil,
			}
			patches.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(_ string, _ *resspeckey.ResSpecKey, req *wisecloud.PendingRequest) {
				req.ResultChan <- mockResponse
			})

			// Mock convert 函数
			patches.ApplyFunc(convert, func(_ *types2.InvokeProcessContext, _ *types.FuncSpec, instanceId string, forceInvoke bool, legacySchedulerInfo *types.InstanceInfo) (*util.InvokeRequest, error) {
				return &util.InvokeRequest{InstanceID: instanceId}, nil
			})

			req, err := handler.makeReq(log.GetLogger())

			convey.So(err, convey.ShouldBeNil)
			convey.So(req.InstanceID, convey.ShouldEqual, "222")
			convey.So(handler.downgrade, convey.ShouldBeTrue)
		})

		convey.Convey("异常情况 - 排队获取实例失败", func() {
			patches = gomonkey.NewPatches()
			defer patches.Reset()

			clearSchedulerProxy()

			// Mock 排队返回错误
			patches.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(_ string, _ *resspeckey.ResSpecKey, req *wisecloud.PendingRequest) {
				req.ResultChan <- &wisecloud.PendingResponse{
					Error: fmt.Errorf("queue error"),
				}
			})

			req, err := handler.makeReq(log.GetLogger())

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(req, convey.ShouldBeNil)
		})
	})
}

func TestKernelRequestHandler_makeReq(t *testing.T) {
	convey.Convey("Test makeReq method", t, func() {
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		ctx.InvokeWithoutScheduler = true

		handler := newKernelRequestHandler(ctx, funcSpec)

		// Mock补丁集合
		var patches *gomonkey.Patches

		convey.Convey("降级情况 - 无scheduler但能获取实例", func() {
			clearSchedulerProxy()
			mockFunctionInstanceAdd("111")
			defer mockFunctionInstanceRemove("111")
			req, err := handler.makeReq(log.GetLogger())

			convey.So(err, convey.ShouldBeNil)
			convey.So(req.InstanceID, convey.ShouldEqual, "111")
			convey.So(handler.downgrade, convey.ShouldBeTrue)
		})

		convey.Convey("降级情况 - 需要排队获取实例", func() {
			patches = gomonkey.NewPatches()
			defer patches.Reset()

			clearSchedulerProxy()

			// Mock 排队返回结果
			mockResponse := &wisecloud.PendingResponse{
				Instance: &types.InstanceSpecification{InstanceID: "222"},
				Error:    nil,
			}
			patches.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(_ string, _ *resspeckey.ResSpecKey, req *wisecloud.PendingRequest) {
				req.ResultChan <- mockResponse
			})

			// Mock convert 函数
			patches.ApplyFunc(convert, func(_ *types2.InvokeProcessContext, funcSpec *types.FuncSpec, instanceId string, forceInvoke bool, schedulerInfo *types.InstanceInfo) (*util.InvokeRequest, error) {
				return &util.InvokeRequest{InstanceID: instanceId}, nil
			})

			req, err := handler.makeReq(log.GetLogger())

			convey.So(err, convey.ShouldBeNil)
			convey.So(req.InstanceID, convey.ShouldEqual, "222")
			convey.So(handler.downgrade, convey.ShouldBeTrue)
		})

		convey.Convey("异常情况 - 排队获取实例失败", func() {
			patches = gomonkey.NewPatches()
			defer patches.Reset()

			clearSchedulerProxy()

			// Mock 排队返回错误
			patches.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(_ string, _ *resspeckey.ResSpecKey, req *wisecloud.PendingRequest) {
				req.ResultChan <- &wisecloud.PendingResponse{
					Error: fmt.Errorf("queue error"),
				}
			})

			req, err := handler.makeReq(log.GetLogger())

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(req, convey.ShouldBeNil)
		})
	})

	convey.Convey("Test makeReq with no k.downgrade", t, func() {
		ctx := &types2.InvokeProcessContext{
			InvokeTimeout: 10,
		}
		funcSpec := &types.FuncSpec{}
		funcSpec.ResourceMetaData.CPU = 500
		funcSpec.ResourceMetaData.Memory = 500
		funcSpec.FunctionKey = "8d86c63b22e24d9ab650878b75408ea6/0@default@func6ac6741a01334320809dfb7dc1e98049/latest"
		ctx.InvokeWithoutScheduler = true

		defer gomonkey.ApplyFuncReturn(upgradecompatible.GetAccessFaaSSchedulerType, "libRuntime").Reset()
		// Mock 排队返回结果
		mockResponse := &wisecloud.PendingResponse{
			Instance: &types.InstanceSpecification{InstanceID: "testId"},
			Error:    nil,
		}
		defer gomonkey.ApplyMethodFunc(wisecloud.GetQueueManager(), "AddPendingRequest", func(_ string, _ *resspeckey.ResSpecKey, req *wisecloud.PendingRequest) {
			req.ResultChan <- mockResponse
		}).Reset()

		// Mock convert 函数
		defer gomonkey.ApplyFunc(convert, func(_ *types2.InvokeProcessContext, funcSpec *types.FuncSpec, instanceId string, forceInvoke bool, schedulerInfo *types.InstanceInfo) (*util.InvokeRequest, error) {
			return &util.InvokeRequest{InstanceID: instanceId}, nil
		}).Reset()

		handler := newKernelRequestHandler(ctx, funcSpec)
		handler.downgrade = false
		convey.Convey("case1: all scheduler unavailable, do downgrade", func() {
			defer gomonkey.ApplyMethodReturn(
				leaseadaptor.GetInstanceManager(),
				"AcquireInstance",
				nil, snerror.New(statuscode.ErrAllSchedulerUnavailable, constant.AllSchedulerUnavailableErrorMessage),
			).Reset()

			req, err := handler.makeReq(log.GetLogger())
			convey.So(handler.downgrade, convey.ShouldBeTrue)
			convey.So(err, convey.ShouldBeNil)
			convey.So(req.InstanceID, convey.ShouldEqual, "testId")
		})
		convey.Convey("case2: other error, do not downgrade, return err", func() {
			defer gomonkey.ApplyMethodReturn(
				leaseadaptor.GetInstanceManager(),
				"AcquireInstance",
				nil, snerror.New(statuscode.InstanceSessionInvalidErrCode, "instance session invalid"),
			).Reset()

			req, err := handler.makeReq(log.GetLogger())
			convey.So(handler.downgrade, convey.ShouldBeFalse)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, "instance session invalid")
			convey.So(req, convey.ShouldBeNil)
		})
	})
}

func TestConvertResourceSpecs(t *testing.T) {
	convey.Convey("Test ConvertResourceSpecs", t, func() {
		// 初始化测试用的上下文和请求
		ctx := &types2.InvokeProcessContext{}
		req := &util.InvokeRequest{
			ResourceSpecs: map[string]int64{},
		}
		convey.Convey("When prepareDynamicResource returns an error", func() {
			defer gomonkey.ApplyFunc(prepareDynamicResource, func(ctx *types2.InvokeProcessContext) (map[string]int64, error) {
				return nil, errors.New("prepare dynamic resource error")
			}).Reset()
			defer gomonkey.ApplyFunc(responsehandler.SetErrorInContext, func(ctx *types2.InvokeProcessContext, innerCode int, message interface{}) {
				return
			}).Reset()
			_, err := convertResourceSpecs(ctx, req)
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("When prepareDynamicResource returns valid resource specs", func() {
			// Mock prepareDynamicResource to return valid resource specs
			dynamicResourceSpecs := map[string]int64{
				constant.ResourceCPUName:    1,
				constant.ResourceMemoryName: 2,
			}
			defer gomonkey.ApplyFunc(prepareDynamicResource, func(ctx *types2.InvokeProcessContext) (map[string]int64, error) {
				return dynamicResourceSpecs, nil
			}).Reset()
			_, err := convertResourceSpecs(ctx, req)
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("When prepareDynamicResource returns invalid resource specs", func() {
			// Mock prepareDynamicResource to return valid resource specs
			dynamicResourceSpecs := map[string]int64{
				constant.ResourceCPUName:    0,
				constant.ResourceMemoryName: 0,
			}
			defer gomonkey.ApplyFunc(prepareDynamicResource, func(ctx *types2.InvokeProcessContext) (map[string]int64, error) {
				return dynamicResourceSpecs, nil
			}).Reset()
			_, err := convertResourceSpecs(ctx, req)
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestCheckErrorMsg(t *testing.T) {
	convey.Convey("test checkErrorMsg", t, func() {
		convey.Convey("msg is nil", func() {
			convey.So(checkErrorMsg(""), convey.ShouldBeNil)
		})
		convey.Convey("msg format err", func() {
			convey.So(checkErrorMsg("dddsaas"), convey.ShouldBeNil)
		})
		convey.Convey("msg with correct format", func() {
			msg := `{ "code": 123, "message": "123 msg"}`
			err := checkErrorMsg(msg)
			convey.So(err.Code(), convey.ShouldEqual, 123)
			convey.So(err.Error(), convey.ShouldEqual, "123 msg")
		})
	})
}

func Test_kernelRequestHandler_accessFaaSSchedulerWithLibRuntime(t *testing.T) {
	convey.Convey("Test kernelRequestHandler accessFaaSSchedulerWithLibRuntime", t, func() {
		k := &kernelRequestHandler{
			logger: log.GetLogger().With(zap.Any("traceId", "123456")),
		}
		convey.Convey("test access scheduler type", func() {
			res := k.accessFaaSSchedulerWithLibRuntime()
			convey.So(res, convey.ShouldBeFalse)
		})
	})
}
