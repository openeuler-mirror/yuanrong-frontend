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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/ugorji/go/codec"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/resspeckey"
	appapi "frontend/pkg/frontend/api/app"
	frontendhttpconstant "frontend/pkg/frontend/common/httpconstant"
	httputil "frontend/pkg/frontend/common/httputil"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/common/tenantauth"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/instancemanager"
	"frontend/pkg/frontend/schedulerproxy"
	"frontend/pkg/sandboxrouter/execendpoint"
	routerroute "frontend/pkg/sandboxrouter/route"
	api "yuanrong.org/kernel/runtime/libruntime/api"
)

const (
	// Default sandbox backend is the dedicated Rust slot (rrt): the akernel SDK
	// sends no runtime, so create/invoke resolve to the native rrt-runtime
	// function. python3.x runtimes remain explicitly selectable.
	defaultSandboxRuntime          = "rrt"
	defaultSandboxFunctionID       = "default/0-defaultservice-rrt/$latest"
	sandboxCreateTimeoutSeconds    = 60
	sandboxScheduleBufferSeconds   = 30
	sandboxDefaultCPU              = 1000
	sandboxDefaultMemory           = 2048
	sandboxInitTimeoutSeconds      = 305
	sandboxGracefulShutdownSeconds = 5
	sandboxDirectoryQuotaMB        = 512
	sandboxInstanceType            = "reserved"
	sandboxDelegateDirectory       = "/tmp"
	sandboxConcurrency             = "1"
	sandboxTemporarySchedulerNote  = "-temporary"
	sandboxKillInstanceSignal      = constant.KillSignalVal
	sandboxRunningPollTimeout      = 5 * time.Second
	sandboxRunningPollInterval     = 200 * time.Millisecond
	sandboxDefaultRRTHTTPPort      = 50090
	sandboxDefaultTunnelWSPort     = 8765
	sandboxDefaultTunnelHTTPPort   = 8766
	sandboxCreateHeartbeatInterval = 2 * time.Second
	sandboxCreateStatusCreating    = "creating"
	sandboxCreateStatusRunning     = "running"
	sandboxCreateStatusTimeout     = "timeout"
	sandboxCreateStatusFailed      = "failed"
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

var waitForSandboxInstanceRunning = func(instanceID, functionID, resourceSpecNote string) bool {
	deadline := time.Now().Add(sandboxRunningPollTimeout)
	for time.Now().Before(deadline) {
		if isSandboxInstanceRunning(instanceID, functionID, resourceSpecNote) {
			return true
		}
		time.Sleep(sandboxRunningPollInterval)
	}
	return isSandboxInstanceRunning(instanceID, functionID, resourceSpecNote)
}

// CreateRequest holds the parameters for sandbox creation.
type CreateRequest struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Tenant    string   `json:"tenant"`
	Runtime   string   `json:"runtime"`
	Rootfs    string   `json:"rootfs"`
	Image     string   `json:"image"`
	Ports     []string `json:"ports"`
	// portRouteKinds is populated only by the frontend for runtime-owned ports.
	// It is deliberately not part of the public SDK request contract.
	portRouteKinds map[int]string
	// Resource and runtime extras (honored by v1 create; 0/nil = use default).
	Cpu         int                      `json:"cpu"`
	Memory      int                      `json:"memory"`
	CpuLimit    int                      `json:"cpu_limit"`
	MemLimit    int                      `json:"mem_limit"`
	Env         map[string]string        `json:"env"`
	Mounts      []map[string]interface{} `json:"mounts"`
	ExtraConfig map[string]interface{}   `json:"extra_config"`
	// Optional logical create budget. A positive request value overrides the
	// environment and default without changing the legacy request shape.
	CreateTimeoutSeconds int `json:"createTimeoutSeconds"`
	// Optional scheduling budget. When only one timeout is supplied, the other
	// is derived using sandboxScheduleBufferSeconds.
	ScheduleTimeoutSeconds int `json:"scheduleTimeoutSeconds"`
}

// RootfsSpec describes a structured sandbox rootfs request for the v1 API.
type RootfsSpec struct {
	Runtime  string `json:"runtime,omitempty"`
	Type     string `json:"type,omitempty"`
	Image    string `json:"image,omitempty"`
	ImageURL string `json:"imageurl,omitempty"`
	Path     string `json:"path,omitempty"`
	ReadOnly *bool  `json:"readonly,omitempty"`
}

// TunnelSpec asks the frontend to prepare the sandbox-side reverse tunnel.
//
// The client intentionally does not expose RRT_TUNNEL_* envs or the tunnel
// control port. Frontend owns those runtime details and returns the stable
// /tunnel/{safeID} gateway URL path after create.
type TunnelSpec struct {
	Enabled   bool `json:"enabled,omitempty"`
	ProxyPort int  `json:"proxyPort,omitempty"`
}

// CreateV1Request holds POST /api/sandbox/v1/sandboxes parameters.
type CreateV1Request struct {
	Name                   string                   `json:"name"`
	Namespace              string                   `json:"namespace"`
	Tenant                 string                   `json:"tenant"`
	Runtime                string                   `json:"runtime"`
	Image                  string                   `json:"image"`
	Rootfs                 RootfsSpec               `json:"rootfs"`
	Ports                  []string                 `json:"ports"`
	IdleTimeoutSeconds     int                      `json:"idleTimeoutSeconds"`
	Cpu                    int                      `json:"cpu"`
	Memory                 int                      `json:"memory"`
	CpuLimit               int                      `json:"cpu_limit"`
	MemLimit               int                      `json:"mem_limit"`
	Env                    map[string]string        `json:"env"`
	Mounts                 []map[string]interface{} `json:"mounts"`
	ExtraConfig            map[string]interface{}   `json:"extra_config"`
	Tunnel                 TunnelSpec               `json:"tunnel,omitempty"`
	CreateTimeoutSeconds   int                      `json:"createTimeoutSeconds"`
	ScheduleTimeoutSeconds int                      `json:"scheduleTimeoutSeconds"`
	portRouteKinds         map[int]string
}

type TunnelInfo struct {
	URL       string `json:"url"`
	Path      string `json:"path"`
	ProxyURL  string `json:"proxyUrl"`
	WSPath    string `json:"wsPath"`
	ProxyPort int    `json:"proxyPort"`
}

// InvokeV1Request is the single data-plane envelope for file/process/shell actions.
type InvokeV1Request struct {
	Action string                 `json:"action"`
	Args   map[string]interface{} `json:"args"`
}

// CreateHandler handles POST /api/sandbox/create.
// It calls CreateInstanceByLibRt directly to create the sandbox yrlib actor
// with skip_serialize semantics: the worker loads yr.sandbox.sandbox.SandboxInstance
// from its local Python environment (no code upload required).
func CreateHandler(ctx *gin.Context) {
	traceID := ensureSandboxTrace(ctx)
	var req CreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("invalid request body: %v", err))
		return
	}
	instanceID, _, err := createSandbox(ctx, req, 0, traceID, true)
	if err != nil {
		return
	}
	appapi.SetCtxResponse(ctx, map[string]string{"instance_id": instanceID}, http.StatusOK, nil)
}

// CreateV1Handler handles POST /api/sandbox/v1/sandboxes.
func CreateV1Handler(ctx *gin.Context) {
	traceID := ensureSandboxTrace(ctx)
	var req CreateV1Request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("invalid request body: %v", err))
		return
	}
	if req.Name == "" {
		req.Name = fmt.Sprintf("sandbox-%d", time.Now().UnixNano())
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	rootfs, err := buildRootfsOption(req.Rootfs, req.Image)
	if err != nil {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, err)
		return
	}
	if usesSandboxRRTRuntime(req.Runtime) {
		prepareSandboxRRTHTTP(&req)
	}
	tunnelInfo := prepareSandboxTunnel(&req)
	createReq := CreateRequest{
		Name:                   req.Name,
		Namespace:              req.Namespace,
		Tenant:                 req.Tenant,
		Runtime:                req.Runtime,
		Rootfs:                 rootfs,
		Ports:                  req.Ports,
		portRouteKinds:         req.portRouteKinds,
		Cpu:                    req.Cpu,
		Memory:                 req.Memory,
		CpuLimit:               req.CpuLimit,
		MemLimit:               req.MemLimit,
		Env:                    req.Env,
		Mounts:                 req.Mounts,
		ExtraConfig:            req.ExtraConfig,
		CreateTimeoutSeconds:   req.CreateTimeoutSeconds,
		ScheduleTimeoutSeconds: req.ScheduleTimeoutSeconds,
	}

	if acceptsSandboxCreateEventStream(ctx) {
		writeSandboxCreateSSEHeader(ctx)
		_ = writeSandboxCreateSSEEvent(ctx, "accepted", map[string]interface{}{
			"status": sandboxCreateStatusCreating,
		})

		stopHeartbeat := make(chan struct{})
		heartbeatDone := make(chan struct{})
		go serveSandboxCreateHeartbeats(ctx, stopHeartbeat, heartbeatDone)

		// Keep the libruntime call on the request goroutine. The C libruntime
		// call path is not safe when moved to an extra goroutine; only heartbeat
		// writes are delegated, and they stop before the final event is written.
		instanceID, status, createErr := createSandbox(
			ctx, createReq, req.IdleTimeoutSeconds, traceID, false,
		)
		close(stopHeartbeat)
		<-heartbeatDone

		data := createV1Response(instanceID, status, tunnelInfo)
		if createErr != nil {
			data["errorCode"] = sandboxCreateErrorCode(createErr)
			data["message"] = createErr.Error()
		}
		_ = writeSandboxCreateSSEEvent(ctx, "final", data)
		return
	}

	instanceID, _, err := createSandbox(ctx, createReq, req.IdleTimeoutSeconds, traceID, true)
	if err != nil {
		return
	}
	appapi.SetCtxResponse(ctx, createV1Response(instanceID, sandboxCreateStatusRunning, tunnelInfo), http.StatusOK, nil)
}

func createV1Response(instanceID, status string, tunnelInfo *TunnelInfo) map[string]interface{} {
	resp := map[string]interface{}{
		"sandboxId":  instanceID,
		"instanceId": instanceID,
		"status":     status,
	}
	if tunnelInfo != nil && status == sandboxCreateStatusRunning {
		tunnelInfo.URL = fmt.Sprintf("/tunnel/%s", routerroute.SanitizeID(instanceID))
		tunnelInfo.Path = tunnelInfo.URL
		tunnelInfo.WSPath = tunnelInfo.URL
		resp["tunnel"] = tunnelInfo
	}
	return resp
}

func prepareSandboxRRTHTTP(req *CreateV1Request) {
	req.Ports = appendUniquePorts(req.Ports, strconv.Itoa(sandboxDefaultRRTHTTPPort))
	setSandboxPortRouteKind(req, sandboxDefaultRRTHTTPPort, sandboxRouteDirect)
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	// Frontend owns the direct-invoke RRT HTTP server port. SDK callers use
	// /direct/{safeID}/invoke and never need to know or set RRT_HTTP_PORT.
	req.Env["RRT_HTTP_PORT"] = strconv.Itoa(sandboxDefaultRRTHTTPPort)
}

func usesSandboxRRTRuntime(runtime string) bool {
	selectedRuntime := strings.ToLower(strings.TrimSpace(runtime))
	if selectedRuntime == "" {
		selectedRuntime = defaultSandboxRuntime
	}
	return selectedRuntime == "rust" || selectedRuntime == "rrt" || selectedRuntime == "rrt-runtime"
}

func prepareSandboxTunnel(req *CreateV1Request) *TunnelInfo {
	if !req.Tunnel.Enabled {
		return nil
	}
	proxyPort := req.Tunnel.ProxyPort
	if proxyPort <= 0 {
		proxyPort = sandboxDefaultTunnelHTTPPort
	}
	wsPort := proxyPort - 1
	if wsPort <= 0 {
		wsPort = sandboxDefaultTunnelWSPort
		proxyPort = sandboxDefaultTunnelHTTPPort
	}
	req.Ports = appendUniquePorts(req.Ports, strconv.Itoa(wsPort), strconv.Itoa(proxyPort))
	setSandboxPortRouteKind(req, wsPort, sandboxRouteTunnel)
	setSandboxPortRouteKind(req, proxyPort, sandboxRouteDirect)
	if req.Env == nil {
		req.Env = map[string]string{}
	}
	// Frontend deliberately owns these RRT runtime envs. SDK callers request a
	// tunnel declaratively and never need to know or override the control ports.
	req.Env["RRT_TUNNEL_WS_PORT"] = strconv.Itoa(wsPort)
	req.Env["RRT_TUNNEL_HTTP_PORT"] = strconv.Itoa(proxyPort)
	return &TunnelInfo{
		ProxyURL:  fmt.Sprintf("http://127.0.0.1:%d", proxyPort),
		ProxyPort: proxyPort,
	}
}

func setSandboxPortRouteKind(req *CreateV1Request, port int, routeKind string) {
	if req.portRouteKinds == nil {
		req.portRouteKinds = make(map[int]string)
	}
	req.portRouteKinds[port] = routeKind
}

func appendUniquePorts(ports []string, values ...string) []string {
	seen := make(map[string]bool, len(ports)+len(values))
	for _, port := range ports {
		seen[port] = true
	}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		ports = append(ports, value)
		seen[value] = true
	}
	return ports
}

func createSandbox(
	ctx *gin.Context,
	req CreateRequest,
	idleTimeoutSeconds int,
	traceID string,
	respondOnError bool,
) (string, string, error) {
	if req.Name == "" || req.Namespace == "" {
		err := fmt.Errorf("name and namespace are required")
		if respondOnError {
			appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, err)
		}
		return "", sandboxCreateStatusFailed, err
	}
	rootfs := req.Rootfs
	if rootfs == "" {
		rootfs = req.Image
	}
	funcID, err := sandboxFunctionIDForRuntime(req.Runtime)
	if err != nil {
		if respondOnError {
			appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, err)
		}
		return "", sandboxCreateStatusFailed, err
	}

	funcMeta := api.FunctionMeta{
		FuncID:     funcID,
		ModuleName: "yr.sandbox.sandbox",
		ClassName:  "SandboxInstance",
		Language:   api.Python,
		Api:        api.ActorApi,
		Name:       &req.Name,
		Namespace:  &req.Namespace,
	}
	cpu := req.Cpu
	if cpu <= 0 {
		cpu = sandboxDefaultCPU
	}
	memory := req.Memory
	if memory <= 0 {
		memory = sandboxDefaultMemory
	}
	traceParent := ctx.Request.Header.Get(constant.HeaderTraceParent)
	createTimeoutSeconds, scheduleTimeoutSeconds, err := resolveSandboxCreateTimeouts(
		req.CreateTimeoutSeconds, req.ScheduleTimeoutSeconds,
	)
	if err != nil {
		if respondOnError {
			appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, err)
		}
		return "", sandboxCreateStatusFailed, err
	}

	invokeOpts := api.InvokeOptions{
		TraceID:           traceID,
		Cpu:               cpu,
		Memory:            memory,
		CpuLimit:          req.CpuLimit,
		MemoryLimit:       req.MemLimit,
		Timeout:           createTimeoutSeconds,
		ScheduleTimeoutMs: int64(scheduleTimeoutSeconds) * 1000,
		CreateOpt:         map[string]string{},
		CustomExtensions: map[string]string{
			"lifecycle":   "detached",
			"Concurrency": sandboxConcurrency,
		},
	}
	if traceParent != "" {
		invokeOpts.CustomExtensions["traceparent"] = traceParent
	}
	if idleTimeoutSeconds > 0 {
		invokeOpts.CustomExtensions["idle_timeout"] = strconv.Itoa(idleTimeoutSeconds)
	}
	if rootfs != "" {
		invokeOpts.CustomExtensions["rootfs"] = rootfs
	}
	// Environment variables → delegate env channel (consumed by the runtime
	// launcher when starting the sandbox container), matching the app path.
	if len(req.Env) > 0 {
		if envJSON, err := json.Marshal(req.Env); err == nil {
			invokeOpts.CreateOpt[constant.DelegateEnvVar] = string(envJSON)
		} else {
			log.GetLogger().Warnf("failed to marshal sandbox env: %v", err)
		}
	}
	// Read-only mounts and sandbox extra config → custom extensions, consumed
	// by node/sandboxd container setup (same keys the yr SDK used).
	if len(req.Mounts) > 0 {
		if mountsJSON, err := json.Marshal(req.Mounts); err == nil {
			invokeOpts.CustomExtensions["mounts"] = string(mountsJSON)
		} else {
			log.GetLogger().Warnf("failed to marshal sandbox mounts: %v", err)
		}
	}
	if len(req.ExtraConfig) > 0 {
		if ecJSON, err := json.Marshal(req.ExtraConfig); err == nil {
			invokeOpts.CustomExtensions["extra_config"] = string(ecJSON)
		} else {
			log.GetLogger().Warnf("failed to marshal sandbox extra_config: %v", err)
		}
	}
	if len(req.Ports) > 0 {
		networkConfig, err := buildSandboxNetworkConfig(req.Ports, req.portRouteKinds)
		if err != nil {
			if respondOnError {
				appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, err)
			}
			return "", sandboxCreateStatusFailed, err
		}
		invokeOpts.CreateOpt["network"] = networkConfig
	}
	tenantID := httputil.GetCompatibleGinHeader(ctx.Request, constant.HeaderTenantID, "tenantId")
	if tenantID == "" {
		tenantID = req.Tenant
	}
	if tenantID == "" {
		tenantID = sandboxTenantClaim(ctx.Request)
	}
	if tenantID != "" {
		invokeOpts.CreateOpt["tenantId"] = tenantID
	}
	invokeOpts.CreateOpt[constant.FunctionKeyNote] = funcID
	invokeOpts.CreateOpt[constant.InstanceTypeNote] = sandboxInstanceType
	invokeOpts.CreateOpt["call_timeout"] = fmt.Sprintf("%d", invokeOpts.Timeout)
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

	instanceID, err := util.NewClient().CreateInstanceByLibRt(funcMeta, []api.Arg{}, invokeOpts)
	if err != nil {
		if shouldTreatCreateTimeoutAsSuccess(instanceID, err) {
			if waitForSandboxInstanceRunning(instanceID, funcID, invokeOpts.CreateOpt[constant.ResourceSpecNote]) {
				log.GetLogger().Infof(
					"sandbox instance reached running state after create timeout instanceID=%s name=%s ns=%s",
					instanceID, req.Name, req.Namespace,
				)
				return instanceID, sandboxCreateStatusRunning, nil
			}
			log.GetLogger().Warnf(
				"sandbox create timed out before running instanceID=%s name=%s ns=%s: %v",
				instanceID, req.Name, req.Namespace, err,
			)
			if respondOnError {
				appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("sandbox create timeout before running: %w", err))
			}
			return instanceID, sandboxCreateStatusTimeout, err
		}
		log.GetLogger().Errorf("failed to create sandbox instance name=%s ns=%s: %v", req.Name, req.Namespace, err)
		if respondOnError {
			appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("failed to create sandbox: %v", err))
		}
		return instanceID, sandboxCreateStatusFailed, err
	}

	log.GetLogger().Infof("sandbox created: instanceID=%s name=%s ns=%s", instanceID, req.Name, req.Namespace)
	return instanceID, sandboxCreateStatusRunning, nil
}

func getSandboxCreateTimeoutSeconds(requested int) int {
	if requested > 0 {
		return requested
	}
	raw := strings.TrimSpace(os.Getenv("YR_SANDBOX_CREATE_TIMEOUT"))
	if raw == "" {
		return sandboxCreateTimeoutSeconds
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.GetLogger().Warnf(
			"invalid YR_SANDBOX_CREATE_TIMEOUT=%q, using default %d",
			raw, sandboxCreateTimeoutSeconds,
		)
		return sandboxCreateTimeoutSeconds
	}
	return value
}

func resolveSandboxCreateTimeouts(requestedCreate, requestedSchedule int) (int, int, error) {
	if requestedCreate < 0 {
		return 0, 0, fmt.Errorf("createTimeoutSeconds must be a positive integer")
	}
	if requestedSchedule < 0 {
		return 0, 0, fmt.Errorf("scheduleTimeoutSeconds must be a positive integer")
	}

	createTimeout := requestedCreate
	scheduleTimeout := requestedSchedule
	if createTimeout == 0 && scheduleTimeout == 0 {
		createTimeout = getSandboxCreateTimeoutSeconds(0)
	}
	if createTimeout == 0 {
		return scheduleTimeout + sandboxScheduleBufferSeconds, scheduleTimeout, nil
	}
	if scheduleTimeout == 0 {
		if createTimeout <= sandboxScheduleBufferSeconds {
			return 0, 0, fmt.Errorf(
				"createTimeoutSeconds must be greater than %d", sandboxScheduleBufferSeconds,
			)
		}
		return createTimeout, createTimeout - sandboxScheduleBufferSeconds, nil
	}
	if scheduleTimeout > createTimeout {
		return 0, 0, fmt.Errorf(
			"scheduleTimeoutSeconds must be less than or equal to createTimeoutSeconds",
		)
	}
	if createTimeout-scheduleTimeout < sandboxScheduleBufferSeconds {
		return 0, 0, fmt.Errorf(
			"createTimeoutSeconds - scheduleTimeoutSeconds must be at least %d",
			sandboxScheduleBufferSeconds,
		)
	}
	return createTimeout, scheduleTimeout, nil
}

func acceptsSandboxCreateEventStream(ctx *gin.Context) bool {
	for _, part := range strings.Split(ctx.GetHeader("Accept"), ",") {
		mediaType := strings.TrimSpace(strings.SplitN(strings.TrimSpace(part), ";", 2)[0])
		if strings.EqualFold(mediaType, frontendhttpconstant.AcceptEventStream) {
			return true
		}
	}
	return false
}

func writeSandboxCreateSSEHeader(ctx *gin.Context) {
	ctx.Header(frontendhttpconstant.ContentTypeHeaderKey, frontendhttpconstant.AcceptEventStream)
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)
}

func writeSandboxCreateSSEHeartbeat(ctx *gin.Context) error {
	if _, err := ctx.Writer.WriteString(": heartbeat\n\n"); err != nil {
		return err
	}
	ctx.Writer.Flush()
	return nil
}

func serveSandboxCreateHeartbeats(ctx *gin.Context, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(sandboxCreateHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Request.Context().Done():
			return
		case <-ticker.C:
			if err := writeSandboxCreateSSEHeartbeat(ctx); err != nil {
				return
			}
		}
	}
}

func writeSandboxCreateSSEEvent(ctx *gin.Context, event string, data map[string]interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err = ctx.Writer.WriteString(fmt.Sprintf("event: %s\n", event)); err != nil {
		return err
	}
	if _, err = ctx.Writer.WriteString(fmt.Sprintf("data: %s\n\n", payload)); err != nil {
		return err
	}
	ctx.Writer.Flush()
	return nil
}

// sandboxTenantClaim recovers the tenant claim for instance ownership metadata
// when frontend JWT middleware is disabled (for example in local/CI smoke).
// This is attribution, not authentication: /direct still sends the original
// token to sandboxRouter, which validates it according to router policy before
// comparing the claim with this stored owner.
func sandboxTenantClaim(req *http.Request) string {
	if req == nil {
		return ""
	}
	token := req.Header.Get(jwtauth.HeaderXAuth)
	if token == "" {
		token = req.URL.Query().Get("token")
	}
	parsed, err := jwtauth.ParseJWT(token)
	if err != nil || parsed == nil || parsed.Payload == nil {
		return ""
	}
	return parsed.Payload.Sub
}

func sandboxCreateErrorCode(err error) int {
	var errInfo api.ErrorInfo
	if errors.As(err, &errInfo) {
		return errInfo.Code
	}
	return 0
}

func ensureSandboxTrace(ctx *gin.Context) string {
	traceID := httputil.InitTraceID(ctx)
	ctx.Header(constant.HeaderTraceID, traceID)
	return traceID
}

func buildRootfsOption(spec RootfsSpec, fallbackImage string) (string, error) {
	image := strings.TrimSpace(spec.ImageURL)
	if image == "" {
		image = strings.TrimSpace(spec.Image)
	}
	if image == "" {
		image = strings.TrimSpace(fallbackImage)
	}
	if spec.Type == "" && image != "" {
		spec.Type = "image"
	}
	if spec.Runtime == "" && (spec.Type != "" || spec.Path != "" || image != "") {
		spec.Runtime = "runsc"
	}
	if spec.Type == "image" {
		spec.ImageURL = image
	}
	if spec.Type == "" && spec.Path == "" && spec.ImageURL == "" {
		return "", nil
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sandboxMethodMeta(method string) api.FunctionMeta {
	return api.FunctionMeta{
		FuncID:     defaultSandboxFunctionID,
		ModuleName: "yr.sandbox.sandbox",
		ClassName:  "SandboxInstance",
		FuncName:   method,
		Language:   api.Python,
		Api:        api.ActorApi,
	}
}

func InvokeV1Handler(ctx *gin.Context) {
	traceID := ensureSandboxTrace(ctx)
	instanceID := ctx.Param("sandboxID")
	if instanceID == "" {
		instanceID = ctx.Param("sandboxId")
	}
	if instanceID == "" {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("sandboxID is required"))
		return
	}
	var req InvokeV1Request
	if err := ctx.ShouldBindJSON(&req); err != nil {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("invalid request body: %v", err))
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	if req.Action == "" {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("action is required"))
		return
	}
	if req.Args == nil {
		req.Args = map[string]interface{}{}
	}
	result, err := invokeSandboxAction(instanceID, req.Action, req.Args, sandboxCreateTimeoutSeconds, traceID, ctx.Request.Header.Get(constant.HeaderTraceParent))
	if err != nil {
		appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, err)
		return
	}
	appapi.SetCtxResponse(ctx, result, http.StatusOK, responseHasError(result))
}

func invokeSandboxAction(instanceID, action string, args map[string]interface{}, timeout int, traceID string, traceParent string) (interface{}, error) {
	envelope := map[string]interface{}{
		"action": action,
		"args":   normalizeJSONValue(args),
	}
	packedArgs, err := packKwargs(envelope)
	if err != nil {
		return nil, err
	}
	invokeOpts := api.InvokeOptions{
		TraceID:          traceID,
		Timeout:          timeout,
		BypassDataSystem: true,
	}
	if traceParent != "" {
		invokeOpts.CustomExtensions = map[string]string{"traceparent": traceParent}
	}
	data, err := util.NewClient().InvokeInstanceByLibRtAndGet(sandboxMethodMeta("sandbox_invoke"), instanceID, packedArgs, invokeOpts)
	if err != nil {
		return nil, err
	}
	return decodeYRValue(data)
}

func normalizeJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		m := make(map[string]interface{}, len(v))
		for key, val := range v {
			m[key] = normalizeJSONValue(val)
		}
		return m
	case []interface{}:
		for i := range v {
			v[i] = normalizeJSONValue(v[i])
		}
		return v
	case float64:
		if math.Trunc(v) == v && v >= math.MinInt64 && v <= math.MaxInt64 {
			return int64(v)
		}
		return v
	default:
		return v
	}
}

func packKwargs(kwargs map[string]interface{}) ([]api.Arg, error) {
	args := make([]api.Arg, 0, len(kwargs)*2)
	for key, value := range kwargs {
		keyArg, err := encodeYRArg(key)
		if err != nil {
			return nil, err
		}
		valueArg, err := encodeYRArg(value)
		if err != nil {
			return nil, err
		}
		args = append(args, keyArg, valueArg)
	}
	return args, nil
}

var msgpackHandle codec.MsgpackHandle

func encodeMsgpack(value interface{}) ([]byte, error) {
	var out []byte
	enc := codec.NewEncoderBytes(&out, &msgpackHandle)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeYRArg(value interface{}) (api.Arg, error) {
	msg, err := encodeMsgpack(value)
	if err != nil {
		return api.Arg{}, err
	}
	// The inline-value header carries the msgpack payload length at buf[8:16]
	// as a FIXED-WIDTH little-endian uint64 (the libruntime DataObject
	// convention), NOT a variable-length msgpack int. Encoding it as msgpack
	// happens to work only while len(msg) < 128 (the fixint byte equals the
	// raw LE byte); at len(msg) >= 128 msgpack prepends a 0xcc marker, so a
	// fixed-width reader gets a corrupt size and the payload is dropped (this
	// is the "data is required" failure on WS uploads of chunks > ~125 bytes).
	buf := make([]byte, 16+len(msg))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(len(msg)))
	copy(buf[16:], msg)
	return api.Arg{Type: api.Value, Data: buf}, nil
}

func decodeYRValue(data []byte) (interface{}, error) {
	candidates := make([][]byte, 0, 2)
	if len(data) >= 16 && isZeroHeader(data[:16]) {
		candidates = append(candidates, data[16:])
	} else {
		candidates = append(candidates, data)
		if len(data) >= 16 {
			candidates = append(candidates, data[16:])
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		var out interface{}
		dec := codec.NewDecoderBytes(candidate, &msgpackHandle)
		if err := dec.Decode(&out); err != nil {
			lastErr = err
			continue
		}
		return normalizeMsgpack(out), nil
	}
	return nil, lastErr
}

func isZeroHeader(header []byte) bool {
	for _, b := range header {
		if b != 0 {
			return false
		}
	}
	return true
}

func normalizeMsgpack(value interface{}) interface{} {
	switch v := value.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(v))
		for key, val := range v {
			m[fmt.Sprint(key)] = normalizeMsgpack(val)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(v))
		for key, val := range v {
			m[key] = normalizeMsgpack(val)
		}
		return m
	case []interface{}:
		for i := range v {
			v[i] = normalizeMsgpack(v[i])
		}
		return v
	case []uint8:
		if utf8.Valid(v) {
			return string(v)
		}
		return v
	default:
		return v
	}
}

func responseHasError(value interface{}) error {
	if m, ok := value.(map[string]interface{}); ok {
		if errVal, exists := m["error"]; exists && errVal != nil && fmt.Sprint(errVal) != "" {
			return fmt.Errorf("%v", errVal)
		}
	}
	return nil
}

func sandboxFunctionIDForRuntime(runtime string) (string, error) {
	selectedRuntime := strings.TrimSpace(runtime)
	if selectedRuntime == "" {
		selectedRuntime = defaultSandboxRuntime
	}

	switch strings.ToLower(selectedRuntime) {
	case "python3.10", "py3.10", "py310", "3.10":
		return "default/0-defaultservice-py310/$latest", nil
	case "python3.9", "py3.9", "py39", "3.9":
		return "default/0-defaultservice-py39/$latest", nil
	case "rust", "rrt", "rrt-runtime":
		// Dedicated Rust sandbox backend (native rrt-runtime). Selected
		// explicitly via the create API's runtime field, decoupled from the
		// python-version slots. See the "rrt" function in services.yaml.
		return "default/0-defaultservice-rrt/$latest", nil
	default:
		return "", fmt.Errorf("unsupported sandbox runtime %q", selectedRuntime)
	}
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

type sandboxPortForwarding struct {
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	RouteKind string `json:"routeKind"`
}

const (
	sandboxRoutePublic = "public"
	sandboxRouteDirect = "direct"
	sandboxRouteTunnel = "tunnel"
)

func buildSandboxNetworkConfig(ports []string, routeKinds map[int]string) (string, error) {
	// The sandbox data plane is L7-only (sandboxrouter is an HTTP/WS reverse
	// proxy; there is no L4 routing). So a forwarded port declares its L7
	// SCHEME ("http"/"https"), NOT a transport protocol — the scheme flows
	// into portForward so the router knows how to speak to the backend. The
	// L4 host<->container mapping is always TCP (http/https ride TCP); the
	// runtime-launcher maps the scheme to a TCP docker port binding.
	portForwardings := make([]sandboxPortForwarding, 0, len(ports))
	for _, portForward := range ports {
		scheme := "http"
		portString := strings.TrimSpace(portForward)
		parts := strings.Split(portString, ":")
		switch len(parts) {
		case 1:
			portString = strings.TrimSpace(parts[0])
		case 2:
			scheme = strings.ToLower(strings.TrimSpace(parts[0]))
			portString = strings.TrimSpace(parts[1])
		default:
			return "", fmt.Errorf("invalid port forwarding format %q, expected PORT or http|https:PORT", portForward)
		}

		port, err := strconv.Atoi(portString)
		if err != nil {
			return "", fmt.Errorf("invalid port number %q", portString)
		}
		if port < 1 || port > 65535 {
			return "", fmt.Errorf("port must be in [1, 65535], got %d", port)
		}
		if scheme != "http" && scheme != "https" {
			return "", fmt.Errorf("port scheme must be http or https, got %s", scheme)
		}
		portForwardings = append(portForwardings, sandboxPortForwarding{
			Port:      port,
			Protocol:  scheme,
			RouteKind: sandboxPortRouteKind(port, routeKinds),
		})
	}

	networkConfig := map[string][]sandboxPortForwarding{
		"portForwardings": portForwardings,
	}
	data, err := json.Marshal(networkConfig)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sandboxPortRouteKind(port int, routeKinds map[int]string) string {
	if routeKind, ok := routeKinds[port]; ok {
		return routeKind
	}
	return sandboxRoutePublic
}

func isSandboxInstanceRunning(instanceID, functionID, resourceSpecNote string) bool {
	resKey, err := resspeckey.GetResKeyFromStr(resourceSpecNote)
	if err != nil {
		log.GetLogger().Warnf("failed to parse sandbox resource spec while checking instance status: %v", err)
		return false
	}
	return instancemanager.GetGlobalInstanceScheduler().GetInstance(
		functionID, resKey.String(), instanceID,
	) != nil
}

// DeleteHandler handles DELETE /api/sandbox/:instanceId.
// It sends a kill signal directly to the sandbox instance via the libruntime API.
func DeleteHandler(ctx *gin.Context) {
	ensureSandboxTrace(ctx)
	instanceID := ctx.Param("instanceId")
	if instanceID == "" {
		instanceID = ctx.Param("sandboxID")
	}
	if instanceID == "" {
		appapi.SetCtxResponse(ctx, nil, http.StatusBadRequest, fmt.Errorf("instanceId is required"))
		return
	}
	needsAuth, errCode, authErr := ensureDeleteJWTContext(ctx, ctx.GetHeader(constant.HeaderTraceID))
	if authErr != nil {
		log.GetLogger().Warnf("reject sandbox delete instanceID=%s: %v", instanceID, authErr)
		appapi.SetCtxResponse(ctx, nil, errCode, authErr)
		return
	}
	if needsAuth {
		if errCode, err := authorizeSandboxDelete(ctx, instanceID); err != nil {
			log.GetLogger().Warnf("reject sandbox delete instanceID=%s: %v", instanceID, err)
			appapi.SetCtxResponse(ctx, nil, errCode, err)
			return
		}
	}

	if err := util.NewClient().KillByLibRt(instanceID, sandboxKillInstanceSignal, []byte("sandbox deleted")); err != nil {
		log.GetLogger().Errorf("failed to kill sandbox instance %s: %v", instanceID, err)
		appapi.SetCtxResponse(ctx, nil, http.StatusInternalServerError, fmt.Errorf("failed to delete sandbox: %v", err))
		return
	}

	appapi.SetCtxResponse(ctx, map[string]string{"status": "deleted"}, http.StatusOK, nil)
}

func needsDeleteAuthorization(ctx *gin.Context) bool {
	_, hasSub := ctx.Get("jwt_sub")
	_, hasRole := ctx.Get("jwt_role")
	return hasSub || hasRole || ctx.GetHeader(jwtauth.HeaderXAuth) != ""
}

func ensureDeleteJWTContext(ctx *gin.Context, traceID string) (bool, int, error) {
	if !needsDeleteAuthorization(ctx) {
		return false, http.StatusOK, nil
	}
	if _, hasSub := ctx.Get("jwt_sub"); hasSub {
		if _, hasRole := ctx.Get("jwt_role"); hasRole {
			return true, http.StatusOK, nil
		}
	}
	token := ctx.GetHeader(jwtauth.HeaderXAuth)
	identity, errCode, err := tenantauth.AuthenticateDeveloperToken(token, traceID)
	if err != nil {
		return true, errCode, err
	}
	ctx.Set("jwt_sub", identity.TenantID)
	ctx.Set("jwt_role", identity.Role)
	return true, http.StatusOK, nil
}

func authorizeSandboxDelete(ctx *gin.Context, instanceID string) (int, error) {
	callerTenant, _ := ctx.Get("jwt_sub")
	callerRole, _ := ctx.Get("jwt_role")
	callerTenantID, ok := callerTenant.(string)
	if !ok || callerTenantID == "" {
		return http.StatusForbidden, errors.New("missing caller tenant in JWT context")
	}
	callerRoleName, ok := callerRole.(string)
	if !ok || callerRoleName == "" {
		return http.StatusForbidden, errors.New("missing caller role in JWT context")
	}
	if callerRoleName != jwtauth.RoleDeveloper {
		return http.StatusForbidden, errors.New("caller role is not authorized to delete instances")
	}
	summary, ok := execendpoint.Default().GetSummary(instanceID)
	if !ok {
		return http.StatusNotFound, errors.New("instance not found in frontend cache")
	}
	if callerTenantID == tenantauth.SystemTenantID {
		return http.StatusOK, nil
	}
	if callerTenantID == summary.TenantID {
		return http.StatusOK, nil
	}
	return http.StatusForbidden, errors.New("caller tenant is not authorized to delete target instance")
}
