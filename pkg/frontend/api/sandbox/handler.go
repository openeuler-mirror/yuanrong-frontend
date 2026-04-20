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

// Package sandbox provides HTTP handlers for sandbox lifecycle management.
package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/resspeckey"
	appapi "frontend/pkg/frontend/api/app"
	httputil "frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/instancemanager"
	"frontend/pkg/frontend/schedulerproxy"
	api "yuanrong.org/kernel/runtime/libruntime/api"
)

const (
	defaultSandboxFunctionID       = "default/0-defaultservice-py39/$latest"
	sandboxCreateTimeoutSeconds    = 60
	sandboxInitTimeoutSeconds      = 305
	sandboxGracefulShutdownSeconds = 900
	sandboxDirectoryQuotaMB        = 512
	sandboxInstanceType            = "reserved"
	sandboxDelegateDirectory       = "/tmp"
	sandboxConcurrency             = "1"
	sandboxTemporarySchedulerNote  = "-temporary"
	sandboxRunningPollTimeout      = 5 * time.Second
	sandboxRunningPollInterval     = 200 * time.Millisecond
)

var selectSandboxSchedulerID = func(funcKey string) (string, error) {
	schedulerInfo, err := schedulerproxy.Proxy.Get(funcKey, log.GetLogger())
	if err != nil {
		return "", err
	}
	if schedulerInfo == nil || schedulerInfo.InstanceInfo == nil || schedulerInfo.InstanceInfo.InstanceID == "" {
		return "", fmt.Errorf("failed to get valid scheduler for funcKey %s", funcKey)
	}
	return schedulerInfo.InstanceInfo.InstanceID, nil
}

var waitForSandboxInstanceRunning = func(instanceID, resourceSpecNote string) bool {
	deadline := time.Now().Add(sandboxRunningPollTimeout)
	for time.Now().Before(deadline) {
		if isSandboxInstanceRunning(instanceID, resourceSpecNote) {
			return true
		}
		time.Sleep(sandboxRunningPollInterval)
	}
	return isSandboxInstanceRunning(instanceID, resourceSpecNote)
}

// CreateRequest holds the parameters for sandbox creation.
type CreateRequest struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Tenant    string `json:"tenant"`
}

// CreateHandler handles POST /api/sandbox/create.
// It calls CreateInstanceByLibRt directly to create the sandbox yrlib actor
// with skip_serialize semantics: the worker loads yr.sandbox.sandbox.SandboxInstance
// from its local Python environment (no code upload required).
func CreateHandler(ctx *gin.Context) {
	var req CreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("invalid request body: %v", err))
		return
	}
	if req.Name == "" || req.Namespace == "" {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("name and namespace are required"))
		return
	}

	funcMeta := api.FunctionMeta{
		FuncID:     defaultSandboxFunctionID,
		ModuleName: "yr.sandbox.sandbox",
		ClassName:  "SandboxInstance",
		Language:   api.Python,
		Api:        api.ActorApi,
		Name:       &req.Name,
		Namespace:  &req.Namespace,
	}
	invokeOpts := api.InvokeOptions{
		Cpu:       1000,
		Memory:    2048,
		Timeout:   sandboxCreateTimeoutSeconds,
		CreateOpt: map[string]string{},
		CustomExtensions: map[string]string{
			"lifecycle":   "detached",
			"Concurrency": sandboxConcurrency,
		},
	}
	tenantID := httputil.GetCompatibleGinHeader(ctx.Request, constant.HeaderTenantID, "tenantId")
	if tenantID == "" {
		tenantID = req.Tenant
	}
	if tenantID != "" {
		invokeOpts.CreateOpt["tenantId"] = tenantID
	}
	invokeOpts.CreateOpt[constant.FunctionKeyNote] = defaultSandboxFunctionID
	invokeOpts.CreateOpt[constant.InstanceTypeNote] = sandboxInstanceType
	invokeOpts.CreateOpt["call_timeout"] = fmt.Sprintf("%d", sandboxCreateTimeoutSeconds)
	invokeOpts.CreateOpt["init_call_timeout"] = fmt.Sprintf("%d", sandboxInitTimeoutSeconds)
	invokeOpts.CreateOpt["GRACEFUL_SHUTDOWN_TIME"] = fmt.Sprintf("%d", sandboxGracefulShutdownSeconds)
	invokeOpts.CreateOpt["DELEGATE_DIRECTORY_INFO"] = sandboxDelegateDirectory
	invokeOpts.CreateOpt["DELEGATE_DIRECTORY_QUOTA"] = fmt.Sprintf("%d", sandboxDirectoryQuotaMB)
	invokeOpts.CreateOpt["ConcurrentNum"] = sandboxConcurrency
	if resSpecJSON, err := buildSandboxResourceSpecJSON(invokeOpts.Cpu, invokeOpts.Memory); err == nil {
		invokeOpts.CreateOpt[constant.ResourceSpecNote] = resSpecJSON
	} else {
		log.GetLogger().Warnf("failed to marshal sandbox resource spec: %v", err)
	}
	schedulerID, err := selectSandboxSchedulerID(defaultSandboxFunctionID)
	if err != nil {
		log.GetLogger().Errorf("failed to select scheduler for sandbox create name=%s ns=%s: %v",
			req.Name, req.Namespace, err)
		appapi.SetCtxResponse(ctx, nil, http.StatusServiceUnavailable,
			fmt.Errorf("failed to create sandbox: no available scheduler"))
		return
	}
	invokeOpts.SchedulerInstanceIDs = []string{schedulerID}
	invokeOpts.CreateOpt[constant.SchedulerIDNote] = schedulerID + sandboxTemporarySchedulerNote

	instanceID, err := util.NewClient().CreateInstanceByLibRt(funcMeta, []api.Arg{}, invokeOpts)
	if err != nil {
		if shouldTreatCreateTimeoutAsSuccess(instanceID, err) {
			if waitForSandboxInstanceRunning(instanceID, invokeOpts.CreateOpt[constant.ResourceSpecNote]) {
				log.GetLogger().Infof(
					"sandbox instance reached running state after create timeout instanceID=%s name=%s ns=%s",
					instanceID, req.Name, req.Namespace,
				)
			}
			log.GetLogger().Warnf(
				"sandbox create returned timeout after scheduling instanceID=%s name=%s ns=%s: %v",
				instanceID, req.Name, req.Namespace, err,
			)
			appapi.SetCtxResponse(ctx, map[string]string{"instance_id": instanceID}, http.StatusOK, nil)
			return
		}
		log.GetLogger().Errorf("failed to create sandbox instance name=%s ns=%s: %v", req.Name, req.Namespace, err)
		appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("failed to create sandbox: %v", err))
		return
	}

	log.GetLogger().Infof("sandbox created: instanceID=%s name=%s ns=%s", instanceID, req.Name, req.Namespace)
	appapi.SetCtxResponse(ctx, map[string]string{"instance_id": instanceID}, http.StatusOK, nil)
}

func shouldTreatCreateTimeoutAsSuccess(instanceID string, err error) bool {
	if instanceID == "" || err == nil {
		return false
	}

	var errInfo api.ErrorInfo
	if !errors.As(err, &errInfo) {
		return false
	}

	return errInfo.Code == 3002
}

func buildSandboxResourceSpecJSON(cpu, memory int) (string, error) {
	resourceSpec := resspeckey.ResourceSpecification{
		CPU:         int64(cpu),
		Memory:      int64(memory),
		InvokeLabel: "",
	}
	data, err := json.Marshal(resourceSpec)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isSandboxInstanceRunning(instanceID, resourceSpecNote string) bool {
	resKey, err := resspeckey.GetResKeyFromStr(resourceSpecNote)
	if err != nil {
		log.GetLogger().Warnf("failed to parse sandbox resource spec while checking instance status: %v", err)
		return false
	}
	return instancemanager.GetGlobalInstanceScheduler().GetInstance(
		defaultSandboxFunctionID, resKey.String(), instanceID,
	) != nil
}

// DeleteHandler handles DELETE /api/sandbox/:instanceId.
// It sends a kill signal directly to the sandbox instance via the libruntime API.
func DeleteHandler(ctx *gin.Context) {
	instanceID := ctx.Param("instanceId")
	if instanceID == "" {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("instanceId is required"))
		return
	}

	if err := util.NewClient().KillByLibRt(instanceID, constant.KillSignalVal, []byte("sandbox deleted")); err != nil {
		log.GetLogger().Errorf("failed to kill sandbox instance %s: %v", instanceID, err)
		appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("failed to delete sandbox: %v", err))
		return
	}

	appapi.SetCtxResponse(ctx, map[string]string{"status": "deleted"}, http.StatusOK, nil)
}
