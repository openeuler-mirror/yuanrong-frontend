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

// Package agent provides HTTP handlers for agent instance lifecycle (create/kill).
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/resspeckey"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/urnutils"
	"frontend/pkg/frontend/api/app"
	"frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/functionmeta"
	"frontend/pkg/frontend/instancemanager"
	"frontend/pkg/frontend/schedulerproxy"
)

// agentUserPlaceholder is the workspace mount target placeholder. It is replaced
// with funcMeta.rootfs.user (from the watched funcSpecMap) in applyAgentFuncMeta.
const agentUserPlaceholder = "__AGENT_USER__"

// agentExecutorFormat is the system executor function funcKey pattern. agent reuses the
// faas system executor function (loaded into function_proxy's funcMetaMap_ at startup from
// executor-meta/*_meta.json) as the CreateInstance FuncID so proxy's GetFuncMeta hits and
// does not return "invalid function (1015)". agent's real config (imageurl/user/ports/
// sandboxType) is passed via createOptions — proxy sinks them as-is, docker executor reads.
const agentExecutorFormat = "default/0-system-faasExecutor%s/$latest"

const (
	defaultAgentCPU              = 1000
	defaultAgentMemory           = 2048
	agentCreateTimeoutSeconds    = 60
	agentInitTimeoutSeconds      = 305
	agentGracefulShutdownSeconds = 900
	agentDirectoryQuotaMB        = 512
	agentInstanceType            = "reserved"
	agentDelegateDirectory       = "/tmp"
	agentConcurrency             = "1"
	agentKillInstanceSignal      = constant.KillSignalVal
	agentRunningPollTimeout      = 5 * time.Second
	agentRunningPollInterval     = 200 * time.Millisecond
	agentCreateTimeoutCode       = 3002
)

// getAgentExecutorFuncKey maps the user function runtime to the faas system executor function
// funcKey (mirrors functionscaler.instancepool.getExecutorFuncKey). agent reuses the faas
// executor function so function_proxy's funcMetaMap_ has it (loaded at startup) — avoids
// "invalid function (1015)" since agent does not register its user funcKey into proxy's
// /yr/functions watch. Falls back to PosixCustom for unknown runtimes.
func getAgentExecutorFuncKey(runtime string) string {
	r := strings.ToLower(runtime)
	switch {
	case strings.Contains(r, "python3.6"):
		return fmt.Sprintf(agentExecutorFormat, "Python3.6")
	case strings.Contains(r, "python3.7"):
		return fmt.Sprintf(agentExecutorFormat, "Python3.7")
	case strings.Contains(r, "python3.8"):
		return fmt.Sprintf(agentExecutorFormat, "Python3.8")
	case strings.Contains(r, "python3.9"):
		return fmt.Sprintf(agentExecutorFormat, "Python3.9")
	case strings.Contains(r, "python3.10"):
		return fmt.Sprintf(agentExecutorFormat, "Python3.10")
	case strings.Contains(r, "python3.11"):
		return fmt.Sprintf(agentExecutorFormat, "Python3.11")
	case strings.Contains(r, "go"), strings.Contains(r, "http"),
		strings.Contains(r, "custom image"):
		return fmt.Sprintf(agentExecutorFormat, "Go1.x")
	case strings.Contains(r, "java8"):
		return fmt.Sprintf(agentExecutorFormat, "Java8")
	case strings.Contains(r, "java11"):
		return fmt.Sprintf(agentExecutorFormat, "Java11")
	case strings.Contains(r, "java17"):
		return fmt.Sprintf(agentExecutorFormat, "Java17")
	case strings.Contains(r, "java21"):
		return fmt.Sprintf(agentExecutorFormat, "Java21")
	case strings.Contains(r, constant.PosixCustomRuntimeType):
		return fmt.Sprintf(agentExecutorFormat, "PosixCustom")
	default:
		return fmt.Sprintf(agentExecutorFormat, "PosixCustom")
	}
}

var selectAgentSchedulerID = func(funcKey string) (string, error) {
	schedulerInfo, err := schedulerproxy.Proxy.Get(funcKey, log.GetLogger())
	if err != nil {
		return "", err
	}
	if schedulerInfo == nil || schedulerInfo.InstanceInfo == nil || schedulerInfo.InstanceInfo.InstanceID == "" {
		return "", fmt.Errorf("failed to get valid scheduler for funcKey %s", funcKey)
	}
	return schedulerInfo.InstanceInfo.InstanceID, nil
}

var waitForAgentInstanceRunning = func(instanceID, functionID, resourceSpecNote string) bool {
	deadline := time.Now().Add(agentRunningPollTimeout)
	for time.Now().Before(deadline) {
		if isAgentInstanceRunning(instanceID, functionID, resourceSpecNote) {
			return true
		}
		time.Sleep(agentRunningPollInterval)
	}
	return isAgentInstanceRunning(instanceID, functionID, resourceSpecNote)
}

// CreateRequest holds the parameters for POST /api/agent.
// Run-as user and container ports come from the function meta (rootfs.user /
// rootfs.ports), so they are NOT create params.
type CreateRequest struct {
	Namespace string            `json:"namespace" binding:"required"`
	Name      string            `json:"name" binding:"required"`
	Urn       string            `json:"urn" binding:"required"`
	Workspace string            `json:"workspace" binding:"required"`
	EnvVars   map[string]string `json:"env_vars,omitempty"`
	Mounts    []Mount           `json:"mounts,omitempty"`
}

// Mount defines a custom bind mount.
type Mount struct {
	Source   string `json:"source" binding:"required"`
	Target   string `json:"target" binding:"required"`
	ReadOnly bool   `json:"readonly,omitempty"`
}

// CreateHandler handles POST /api/agent.
// It calls CreateInstanceByLibRt with the user function URN (FuncID=funcKey, no fallback),
// mounts workspace to /home/${rootfs.user} (placeholder replaced by function_proxy), and
// sinks the dynamic env vars (incl. userid) via createOptions["DELEGATE_ENV_VAR"]. Static env,
// run-as user, and container ports travel with the function meta, not here.
func CreateHandler(ctx *gin.Context) {
	var req CreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		app.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("invalid request body: %v", err))
		return
	}

	// libruntime routes by funcKey (tenant/funcName/version), not the URN string.
	// Parse the URN and combine into a funcKey before setting FuncID.
	funcUrn := &urnutils.FunctionURN{}
	if err := funcUrn.ParseFrom(req.Urn); err != nil {
		app.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("invalid urn: %v", err))
		return
	}
	funcKey := urnutils.CombineFunctionKey(funcUrn.TenantID, funcUrn.FuncName, funcUrn.FuncVersion)
	// agent reuses the faas system executor function as the CreateInstance FuncID (see
	// getAgentExecutorFuncKey). function_proxy's funcMetaMap_ has these system functions
	// preloaded at startup (from executor-meta/*_meta.json), so GetFuncMeta(funcKey) hits —
	// avoids "invalid function (1015)" since agent's user funcKey is never registered into
	// proxy's /yr/functions watch. The user function's real config (imageurl/user/ports/
	// sandboxType) is looked up from the frontend-watched funcSpecMap by funcKey and sunk
	// via createOptions; proxy does not need the user funcMeta.
	runtime := ""
	if spec, ok := functionmeta.LoadFuncSpec(funcKey); ok && spec != nil {
		runtime = spec.FuncMetaData.Runtime
	}
	executorFuncKey := getAgentExecutorFuncKey(runtime)
	// Name intentionally left empty: when FunctionMeta.Name is empty, libruntime SDK
	// does not set designatedInstanceID, and function_proxy's instance_control_view
	// generates a random UUID as instance_id — avoiding namespace-name collisions.
	funcMeta := api.FunctionMeta{
		FuncID:    executorFuncKey,
		Language:  api.Python,
		Api:       api.ActorApi,
		Namespace: &req.Namespace,
	}
	invokeOpts, err := buildAgentInvokeOptions(ctx, req, funcKey)
	if err != nil {
		app.SetCtxResponse(ctx, nil, http.StatusBadRequest, err)
		return
	}
	createAgentInstance(ctx, req, funcMeta, invokeOpts)
}

// buildAgentInvokeOptions builds the invoke options for agent create.
// funcKey is used to look up the watched funcMeta (rootfs.user/ports/sandboxType/imageurl)
// and transparently pass them into createOptions — proxy does not need to merge funcMeta.
func buildAgentInvokeOptions(ctx *gin.Context, req CreateRequest, funcKey string) (api.InvokeOptions, error) {
	invokeOpts := api.InvokeOptions{
		Cpu:              defaultAgentCPU,
		Memory:           defaultAgentMemory,
		Timeout:          agentCreateTimeoutSeconds,
		CreateOpt:        map[string]string{},
		CustomExtensions: map[string]string{"lifecycle": "detached", "Concurrency": agentConcurrency},
	}

	if err := applyAgentRootfsMounts(&invokeOpts, req); err != nil {
		return api.InvokeOptions{}, err
	}
	applyAgentDynamicEnv(&invokeOpts, req)
	applyAgentFuncMeta(&invokeOpts, funcKey)
	applyAgentCreateOpts(&invokeOpts, ctx, req)
	return invokeOpts, nil
}

// applyAgentFuncMeta looks up the watched funcMeta by funcKey and transparently passes
// rootfs (imageurl/user/ports) and sandboxType into createOptions. This way proxy does not
// need to merge funcMeta — frontend gives proxy everything it needs to create the container.
func applyAgentFuncMeta(invokeOpts *api.InvokeOptions, funcKey string) {
	spec, ok := functionmeta.LoadFuncSpec(funcKey)
	if !ok || spec == nil {
		log.GetLogger().Warnf("agent funcMeta not found in cache for funcKey %s, skip funcMeta sinking", funcKey)
		return
	}
	if spec.SandboxType != "" {
		invokeOpts.CreateOpt["sandbox_type"] = spec.SandboxType
	}
	if spec.RootfsSpecMeta.User != "" {
		invokeOpts.CreateOpt["host_user"] = spec.RootfsSpecMeta.User
		// Replace workspace target placeholder with real user.
		if rootfsStr, exists := invokeOpts.CreateOpt["rootfs"]; exists && rootfsStr != "" {
			rootfsStr = strings.ReplaceAll(rootfsStr, agentUserPlaceholder, spec.RootfsSpecMeta.User)
			invokeOpts.CreateOpt["rootfs"] = rootfsStr
		}
	}
	if spec.RootfsSpecMeta.ImageURL != "" {
		rootfsJSON, err := json.Marshal(map[string]interface{}{
			"type": "image", "imageurl": spec.RootfsSpecMeta.ImageURL, "mounts": []interface{}{},
		})
		if err != nil {
			log.GetLogger().Warnf("failed to marshal agent rootfs imageurl: %v", err)
		} else if existing, exists := invokeOpts.CreateOpt["rootfs"]; exists && existing != "" {
			invokeOpts.CreateOpt["rootfs"] = mergeRootfsJSON(existing, string(rootfsJSON))
		} else {
			invokeOpts.CreateOpt["rootfs"] = string(rootfsJSON)
		}
	}
	if len(spec.RootfsSpecMeta.Ports) > 0 {
		portForwardings := make([]map[string]interface{}, 0, len(spec.RootfsSpecMeta.Ports))
		for _, p := range spec.RootfsSpecMeta.Ports {
			protocol := "TCP"
			portStr := p
			if idx := strings.Index(p, ":"); idx >= 0 {
				protocol = strings.ToUpper(p[:idx])
				portStr = p[idx+1:]
			}
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}
			portForwardings = append(portForwardings, map[string]interface{}{"port": port, "protocol": protocol})
		}
		networkJSON, err := json.Marshal(map[string]interface{}{"portForwardings": portForwardings})
		if err != nil {
			log.GetLogger().Warnf("failed to marshal agent network portForwardings: %v", err)
		} else {
			invokeOpts.CreateOpt["network"] = string(networkJSON)
		}
	}
	mergeAgentStaticEnv(invokeOpts, spec)
}

// mergeAgentStaticEnv merges the function's static environment (funcSpec.EnvMetaData.Environment,
// a JSON map registered with the function) into createOptions["DELEGATE_ENV_VAR"]. Since agent
// reuses the faasExecutor system function as FuncID, function_proxy sinks the (empty) faasExecutor
// envMetaData — the user function's static env never reaches the container unless frontend passes it.
// Dynamic env_vars (set earlier by applyAgentDynamicEnv) take precedence; static env only fills keys
// not already present. ParseDelegateEnv on the C++ side also inserts (non-overwriting), so the
// merged JSON yields: dynamic values win, static values fill the rest.
func mergeAgentStaticEnv(invokeOpts *api.InvokeOptions, spec *types.FuncSpec) {
	if spec == nil || spec.EnvMetaData.Environment == "" {
		return
	}
	var staticEnv map[string]string
	if err := json.Unmarshal([]byte(spec.EnvMetaData.Environment), &staticEnv); err != nil {
		log.GetLogger().Warnf("failed to unmarshal agent static environment: %v", err)
		return
	}
	if len(staticEnv) == 0 {
		return
	}
	merged := make(map[string]string, len(staticEnv))
	for k, v := range staticEnv {
		merged[k] = v
	}
	if existing, ok := invokeOpts.CreateOpt["DELEGATE_ENV_VAR"]; ok && existing != "" {
		var dynamicEnv map[string]string
		if err := json.Unmarshal([]byte(existing), &dynamicEnv); err == nil {
			for k, v := range dynamicEnv {
				merged[k] = v // dynamic wins
			}
		}
	}
	envJSON, err := json.Marshal(merged)
	if err != nil {
		log.GetLogger().Warnf("failed to marshal merged agent env: %v", err)
		return
	}
	invokeOpts.CreateOpt["DELEGATE_ENV_VAR"] = string(envJSON)
}

// mergeRootfsJSON merges two rootfs JSON strings (image into existing mounts).
func mergeRootfsJSON(existing, imageJSON string) string {
	var existingMap, imageMap map[string]interface{}
	if err := json.Unmarshal([]byte(existing), &existingMap); err != nil {
		return imageJSON
	}
	if err := json.Unmarshal([]byte(imageJSON), &imageMap); err != nil {
		return existing
	}
	for k, v := range imageMap {
		if k == "mounts" {
			if _, ok := existingMap[k]; !ok {
				existingMap[k] = v
			}
			continue
		}
		existingMap[k] = v
	}
	merged, err := json.Marshal(existingMap)
	if err != nil {
		log.GetLogger().Warnf("failed to marshal merged agent rootfs: %v", err)
		return existing
	}
	return string(merged)
}

// applyAgentRootfsMounts builds rootfs.mounts from the workspace and custom mounts,
// writes it into CreateOpt["rootfs"]. rootfs goes into CreateOpt (not CustomExtensions)
// so docker executor's BuildBindMounts (which reads deployOptions["rootfs"]) can parse it.
func applyAgentRootfsMounts(invokeOpts *api.InvokeOptions, req CreateRequest) error {
	var rootfsMounts []map[string]interface{}
	if req.Workspace != "" {
		if err := validateBindSource(req.Workspace, "workspace"); err != nil {
			return err
		}
		rootfsMounts = append(rootfsMounts, map[string]interface{}{
			"source": req.Workspace, "target": "/home/" + agentUserPlaceholder, "readonly": false,
		})
	}
	for _, m := range req.Mounts {
		if err := validateBindSource(m.Source, "mount source"); err != nil {
			return err
		}
		rootfsMounts = append(rootfsMounts, map[string]interface{}{
			"source": m.Source, "target": m.Target, "readonly": m.ReadOnly,
		})
	}
	if len(rootfsMounts) == 0 {
		return nil
	}
	rootfsJSON, err := json.Marshal(map[string]interface{}{"mounts": rootfsMounts})
	if err != nil {
		return fmt.Errorf("failed to marshal rootfs mounts: %v", err)
	}
	invokeOpts.CreateOpt["rootfs"] = string(rootfsJSON)
	return nil
}

// applyAgentDynamicEnv sinks dynamic env vars (incl. userid) via createOptions["DELEGATE_ENV_VAR"]
// (JSON), which the runtime merges into sandbox userenvs. Static env travels with the
// function meta (EnvMetaData), not here.
func applyAgentDynamicEnv(invokeOpts *api.InvokeOptions, req CreateRequest) {
	if len(req.EnvVars) == 0 {
		return
	}
	envJSON, err := json.Marshal(req.EnvVars)
	if err != nil {
		log.GetLogger().Warnf("failed to marshal agent env_vars: %v", err)
		return
	}
	invokeOpts.CreateOpt["DELEGATE_ENV_VAR"] = string(envJSON)
}

// applyAgentCreateOpts sets the common createOptions shared across agent instances.
func applyAgentCreateOpts(invokeOpts *api.InvokeOptions, ctx *gin.Context, req CreateRequest) {
	tenantID := httputil.GetCompatibleGinHeader(ctx.Request, constant.HeaderTenantID, "tenantId")
	if tenantID != "" {
		invokeOpts.CreateOpt["tenantId"] = tenantID
	}
	invokeOpts.CreateOpt[constant.FunctionKeyNote] = req.Urn
	invokeOpts.CreateOpt[constant.InstanceTypeNote] = agentInstanceType
	invokeOpts.CreateOpt["call_timeout"] = strconv.Itoa(agentCreateTimeoutSeconds)
	invokeOpts.CreateOpt["init_call_timeout"] = strconv.Itoa(agentInitTimeoutSeconds)
	invokeOpts.CreateOpt["GRACEFUL_SHUTDOWN_TIME"] = strconv.Itoa(agentGracefulShutdownSeconds)
	invokeOpts.CreateOpt["DELEGATE_DIRECTORY_INFO"] = agentDelegateDirectory
	invokeOpts.CreateOpt["DELEGATE_DIRECTORY_QUOTA"] = strconv.Itoa(agentDirectoryQuotaMB)
	invokeOpts.CreateOpt["ConcurrentNum"] = agentConcurrency
	if resSpecJSON, err := buildAgentResourceSpecJSON(invokeOpts.Cpu, invokeOpts.Memory); err == nil {
		invokeOpts.CreateOpt[constant.ResourceSpecNote] = resSpecJSON
	} else {
		log.GetLogger().Warnf("failed to marshal agent resource spec: %v", err)
	}
}

func validateBindSource(path, label string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be an absolute path: %s", label, path)
	}
	if !isSafeBindSource(path) {
		return fmt.Errorf("unsafe %s: %s", label, path)
	}
	return nil
}

func createAgentInstance(
	ctx *gin.Context, req CreateRequest, funcMeta api.FunctionMeta, invokeOpts api.InvokeOptions,
) {
	instanceID, err := util.NewClient().CreateInstanceByLibRt(funcMeta, []api.Arg{}, invokeOpts)
	if err != nil {
		if shouldTreatCreateTimeoutAsSuccess(instanceID, err) {
			if waitForAgentInstanceRunning(instanceID, funcMeta.FuncID,
				invokeOpts.CreateOpt[constant.ResourceSpecNote]) {
				log.GetLogger().Infof(
					"agent instance reached running state after create timeout instanceID=%s name=%s ns=%s",
					instanceID, req.Name, req.Namespace)
			}
			log.GetLogger().Warnf(
				"agent create returned timeout after scheduling instanceID=%s name=%s ns=%s: %v",
				instanceID, req.Name, req.Namespace, err)
			ctx.JSON(http.StatusOK, gin.H{"code": 200, "instance_id": instanceID})
			return
		}
		log.GetLogger().Errorf("failed to create agent instance name=%s ns=%s: %v", req.Name, req.Namespace, err)
		app.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("failed to create agent: %v", err))
		return
	}

	log.GetLogger().Infof("agent created: instanceID=%s name=%s ns=%s", instanceID, req.Name, req.Namespace)
	ctx.JSON(http.StatusOK, gin.H{"code": 200, "instance_id": instanceID})
}

// DeleteHandler handles DELETE /api/agent/:instanceId.
// Input is the instance_id returned by create; kills the agent instance directly.
func DeleteHandler(ctx *gin.Context) {
	instanceID := ctx.Param("instanceId")
	if instanceID == "" {
		app.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("instanceId is required"))
		return
	}
	if err := util.NewClient().KillByLibRt(instanceID, agentKillInstanceSignal,
		[]byte("agent deleted")); err != nil {
		log.GetLogger().Errorf("failed to kill agent instance %s: %v", instanceID, err)
		ctx.JSON(http.StatusInternalServerError,
			gin.H{"code": 500, "message": fmt.Sprintf("failed to delete agent: %v", err)})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 200, "status": "deleted"})
}

func shouldTreatCreateTimeoutAsSuccess(instanceID string, err error) bool {
	if instanceID == "" || err == nil {
		return false
	}
	var errInfo api.ErrorInfo
	if !errors.As(err, &errInfo) {
		return false
	}
	return errInfo.Code == agentCreateTimeoutCode
}

func buildAgentResourceSpecJSON(cpu, memory int) (string, error) {
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

func isAgentInstanceRunning(instanceID, functionID, resourceSpecNote string) bool {
	resKey, err := resspeckey.GetResKeyFromStr(resourceSpecNote)
	if err != nil {
		log.GetLogger().Warnf("failed to parse agent resource spec while checking instance status: %v", err)
		return false
	}
	return instancemanager.GetGlobalInstanceScheduler().GetInstance(
		functionID, resKey.String(), instanceID) != nil
}

// isSafeBindSource mirrors docker_executor IsSafeBindSource: reject unsafe host paths
// ("/", "/etc", "/proc", "/sys", "/dev", "/boot", docker.sock, "..").
func isSafeBindSource(path string) bool {
	clean := filepath.Clean(path)
	switch clean {
	case "/", "/etc", "/proc", "/sys", "/dev", "/boot":
		return false
	}
	if strings.Contains(clean, "..") {
		return false
	}
	if strings.HasSuffix(clean, "docker.sock") {
		return false
	}
	return true
}
