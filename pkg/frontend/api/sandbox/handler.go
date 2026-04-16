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
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	appapi "frontend/pkg/frontend/api/app"
	"frontend/pkg/frontend/common/util"
	api "yuanrong.org/kernel/runtime/libruntime/api"
)

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
		ModuleName: "yr.sandbox.sandbox",
		ClassName:  "SandboxInstance",
		Language:   api.Python,
		Api:        api.ActorApi,
		Name:       &req.Name,
		Namespace:  &req.Namespace,
	}
	invokeOpts := api.InvokeOptions{
		Cpu:    1000,
		Memory: 2048,
		CustomExtensions: map[string]string{
			"lifecycle":   "detached",
			"Concurrency": "1",
		},
	}

	instanceID, err := util.NewClient().CreateInstanceByLibRt(funcMeta, []api.Arg{}, invokeOpts)
	if err != nil {
		log.GetLogger().Errorf("failed to create sandbox instance name=%s ns=%s: %v", req.Name, req.Namespace, err)
		appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("failed to create sandbox: %v", err))
		return
	}

	log.GetLogger().Infof("sandbox created: instanceID=%s name=%s ns=%s", instanceID, req.Name, req.Namespace)
	appapi.SetCtxResponse(ctx, map[string]string{"instance_id": instanceID}, http.StatusOK, nil)
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
