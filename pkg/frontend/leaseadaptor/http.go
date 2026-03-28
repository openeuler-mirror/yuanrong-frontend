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

// Package leaseadaptor -
package leaseadaptor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/tls"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/common/httpconstant"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/schedulerproxy"
)

const (
	releaseAction     = "release"
	batchRetainAction = "batchRetain"
	defaultTimeout    = 3
)

func createAcquireArgs(option *types.AcquireOption, funcKey string) ([]*api.Arg, error) {
	var invokeArgs []*api.Arg
	var acquireOps []byte
	instanceRequirement := make(map[string][]byte, 3) // magic number

	acquireOps = []byte(fmt.Sprintf("acquire#%s", funcKey))

	resourcesData, err := json.Marshal(option.ResourceSpecs)
	instanceRequirement[constant.InstanceRequirementResourcesKey] = resourcesData
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource when acquire %s instance, error %s",
			funcKey, err.Error())
	}

	instanceRequirement[constant.InstanceRequirementPoolLabel] = []byte(option.PoolLabel)

	if option.InstanceLabel != "" {
		m := map[string]string{
			httpconstant.HeaderInstanceLabel: option.InstanceLabel,
		}
		bytes, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("marshal instanlabel failed, err: %s", err.Error())
		} else {
			instanceRequirement[constant.InstanceRequirementInvokeLabel] = bytes
		}
	}

	callerPodName := getPodName()
	if callerPodName != "" {
		instanceRequirement[constant.InstanceCallerPodName] = []byte(callerPodName)
	}

	if option.TrafficLimited {
		instanceRequirement[constant.InstanceTrafficLimited] = []byte("true")
	}

	if option.InstanceSession != nil {
		bytes, err := json.Marshal(option.InstanceSession)
		if err != nil {
			return nil, fmt.Errorf("marshal instanceSession header failed: %s", err.Error())
		} else {
			instanceRequirement[constant.InstanceSessionConfig] = bytes
		}
	}

	insRequirementBytes, err := json.Marshal(instanceRequirement)
	if err != nil {
		log.GetLogger().Errorf("failed to marshal resource when acquire %s instance, error %s",
			funcKey, err.Error())
	}
	acquireArg := &api.Arg{Type: api.Value, Data: acquireOps}
	instanceArg := &api.Arg{Type: api.Value, Data: insRequirementBytes}
	traceID := &api.Arg{Type: api.Value, Data: []byte(option.TraceID)}
	invokeArgs = []*api.Arg{acquireArg, instanceArg, traceID}
	return invokeArgs, nil
}

func createReleaseArgs(leaseId string, option *types.AcquireOption, report *InstanceReport) ([]*api.Arg, error) {
	reportData, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	actionArg := &api.Arg{
		Type: api.Value,
		Data: []byte(fmt.Sprintf("%s#%s", releaseAction, leaseId)),
	}
	reportArg := &api.Arg{
		Type: api.Value,
		Data: reportData,
	}
	traceIdArg := &api.Arg{Type: api.Value, Data: []byte(option.TraceID)}
	return []*api.Arg{actionArg, reportArg, traceIdArg}, nil
}

func createBatchRetainArgs(batch *BatchRetainLeaseInfos, traceId string) ([]*api.Arg, error) {
	args := make([]*api.Arg, 0, 3)
	args = append(args, &api.Arg{
		Type: api.Value,
		Data: []byte(fmt.Sprintf("%s#%s", batchRetainAction, batch.targetName)),
	})
	bytes, err := json.Marshal(batch.infos)
	if err != nil {
		return nil, err
	}
	args = append(args, &api.Arg{
		Type: api.Value,
		Data: bytes,
	})
	args = append(args, &api.Arg{Type: api.Value, Data: []byte(traceId)})
	return args, nil
}

func doAcquireInvoke(option *types.AcquireOption, ip string, funcKey string, timeout int64) (
	*types.InstanceResponse, error,
) {
	args, err := createAcquireArgs(option, funcKey)
	if err != nil {
		return nil, err
	}
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	err = prepareSchedulerRequest(req, ip, args, option.TraceID, option.TraceParent)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = defaultAcquireLeaseTimeout
	}
	err = requestScheduler(req, resp, timeout)
	if err != nil {
		return nil, err
	}
	instanceResponse := &types.InstanceResponse{}
	err = json.Unmarshal(resp.Body(), instanceResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal instance response error %s", err.Error())
	}
	return instanceResponse, nil
}

// 不用关心是否成功
func doReleaseInvoke(funcKey string, leaseId string, option *types.AcquireOption, report *InstanceReport) {
	logger := log.GetLogger().With(zap.Any("leaseId", leaseId))
	args, err := createReleaseArgs(leaseId, option, report)
	if err != nil {
		logger.Warnf("create release args failed, abort release, err: %s", err.Error())
		return
	}
	schedulerInfo, err := schedulerproxy.Proxy.Get(funcKey, logger)
	if err != nil {
		logger.Errorf("can not get scheduler, err: %s", err.Error())
		return
	}
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	err = prepareSchedulerRequest(req, schedulerInfo.InstanceInfo.Address, args, option.TraceID, option.TraceParent)
	if err != nil {
		logger.Warnf("prepare scheduler request failed,, abort release err: %s", err.Error())
		return
	}
	if err = requestScheduler(req, resp, defaultTimeout); err != nil {
		logger.Warnf("release failed, err: %s, no need retry", err.Error())
	}
}

func doBatchRetainInvoke(batch *BatchRetainLeaseInfos, traceId string) (*types.BatchInstanceResponse, error) {
	logger := log.GetLogger().With(zap.Any("traceId", traceId))
	args, err := createBatchRetainArgs(batch, traceId)
	if err != nil {
		logger.Errorf("create batchratain args failed, err: %s", err.Error())
		return nil, err
	}
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	err = prepareSchedulerRequest(req, batch.SchedulerAddress, args, traceId, "")
	if err != nil {
		logger.Errorf("prepare scheduler request failed, err: %s", err.Error())
		return nil, err
	}
	err = requestScheduler(req, resp, defaultTimeout)
	if err != nil {
		return nil, err
	}
	batchResp := &types.BatchInstanceResponse{}
	err = json.Unmarshal(resp.Body(), batchResp)
	if err != nil {
		return nil, err
	}
	return batchResp, nil
}

func prepareSchedulerRequest(schedulerReq *fasthttp.Request, dstHost string,
	args []*api.Arg, traceID string, traceParent string,
) error {
	schedulerReq.SetRequestURI(callSchedulerPath)
	schedulerReq.Header.SetMethod(http.MethodPost)
	schedulerReq.Header.ResetConnectionClose()
	schedulerReq.SetHost(dstHost)
	schedulerReq.URI().SetScheme(tls.GetURLScheme(false))
	schedulerReq.Header.Set(constant.HeaderTraceID, traceID)
	if traceParent != "" {
		schedulerReq.Header.Set(constant.HeaderTraceParent, traceParent)
	}
	argsData, err := json.Marshal(args)
	if err != nil {
		return err
	}
	schedulerReq.SetBody(argsData)
	return nil
}

func requestScheduler(req *fasthttp.Request, resp *fasthttp.Response, timeout int64) error {
	err := httputil.GetSchedulerClient().DoTimeout(req, resp, time.Duration(timeout)*time.Second)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("call scheduler failed,http code %d", resp.StatusCode())
	}
	return nil
}

func getPodName() string {
	podName := os.Getenv(constant.HostNameEnvKey)
	if os.Getenv(constant.PodNameEnvKey) != "" {
		podName = os.Getenv(constant.PodNameEnvKey)
	}
	return podName
}
