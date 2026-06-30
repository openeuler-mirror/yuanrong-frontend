package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/resspeckey"
	"frontend/pkg/common/job"
	"frontend/pkg/frontend/common/util"
)

type runtimeStub struct {
	createInstance func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error)
	kill           func(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error
}

func (r *runtimeStub) CreateInstance(
	funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions,
) (string, error) {
	if r.createInstance != nil {
		return r.createInstance(funcMeta, args, invokeOpt)
	}
	return "", nil
}

func (r *runtimeStub) InvokeByInstanceId(
	funcMeta api.FunctionMeta, instanceID string, args []api.Arg, invokeOpt api.InvokeOptions,
) (string, error) {
	return "", nil
}

func (r *runtimeStub) InvokeByFunctionName(
	funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions,
) (string, error) {
	return "", nil
}

func (r *runtimeStub) AcquireInstance(
	state string, funcMeta api.FunctionMeta, acquireOpt api.InvokeOptions,
) (api.InstanceAllocation, error) {
	return api.InstanceAllocation{}, nil
}

func (r *runtimeStub) ReleaseInstance(
	allocation api.InstanceAllocation, stateID string, abnormal bool, option api.InvokeOptions,
) {
}

func (r *runtimeStub) Kill(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error {
	if r.kill != nil {
		return r.kill(instanceID, signal, payload, invokeOpt)
	}
	return nil
}

func (r *runtimeStub) CreateInstanceRaw(createReqRaw []byte, option api.RawRequestOption) ([]byte, error) {
	return nil, nil
}

func (r *runtimeStub) InvokeByInstanceIdRaw(invokeReqRaw []byte, option api.RawRequestOption) ([]byte, error) {
	return nil, nil
}

func (r *runtimeStub) KillRaw(killReqRaw []byte, option api.RawRequestOption) ([]byte, error) {
	return nil, nil
}

func (r *runtimeStub) SaveState(state []byte) (string, error) {
	return "", nil
}

func (r *runtimeStub) LoadState(checkpointID string) ([]byte, error) {
	return nil, nil
}

func (r *runtimeStub) Exit(code int, message string) {}

func (r *runtimeStub) KVSet(key string, value []byte, param api.SetParam) error {
	return nil
}

func (r *runtimeStub) KVSetWithoutKey(value []byte, param api.SetParam) (string, error) {
	return "", nil
}

func (r *runtimeStub) KVGet(key string, timeoutms uint) ([]byte, error) {
	return nil, nil
}

func (r *runtimeStub) KVGetMulti(keys []string, timeoutms uint) ([][]byte, error) {
	return nil, nil
}

func (r *runtimeStub) KVDel(key string) error {
	return nil
}

func (r *runtimeStub) KVDelMulti(keys []string) ([]string, error) {
	return nil, nil
}

func (r *runtimeStub) SetTraceID(traceID string) {}

func (r *runtimeStub) Put(objectID string, value []byte, param api.PutParam, nestedObjectIDs ...string) error {
	return nil
}

func (r *runtimeStub) Get(objectIDs []string, timeoutMs int) ([][]byte, error) {
	return nil, nil
}

func (r *runtimeStub) GIncreaseRef(objectIDs []string, remoteClientID ...string) ([]string, error) {
	return nil, nil
}

func (r *runtimeStub) GDecreaseRef(objectIDs []string, remoteClientID ...string) ([]string, error) {
	return nil, nil
}

func (r *runtimeStub) GetAsync(objectID string, cb api.GetAsyncCallback) {}

func (r *runtimeStub) GetEvent(objectID string, cb api.GetEventCallback) {}

func (r *runtimeStub) DeleteGetEventCallback(objectID string) {}

func (r *runtimeStub) GetFormatLogger() api.FormatLogger {
	return nil
}

func (r *runtimeStub) GetCredential() api.Credential {
	return api.Credential{}
}

func (r *runtimeStub) SetTenantID(tenantID string) error {
	return nil
}

func (r *runtimeStub) IsHealth() bool {
	return true
}

func (r *runtimeStub) IsDsHealth() bool {
	return true
}

func (r *runtimeStub) GetActiveMasterAddr() string {
	return ""
}

func TestCreateHandlerPropagatesHeaderTenantID(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-from-header-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	var capturedFuncMeta api.FunctionMeta
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedFuncMeta = funcMeta
			capturedInvokeOpt = invokeOpt
			return "instance-from-header", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-a",
		Namespace: "sandbox",
		Tenant:    "body-tenant",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)
	ctx.Request.Header.Set(constant.HeaderTenantID, "header-tenant")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.Equal(t, sandboxCreateTimeoutSeconds, capturedInvokeOpt.Timeout)
	require.Equal(t, defaultSandboxFunctionID, capturedInvokeOpt.CreateOpt[constant.FunctionKeyNote])
	require.Equal(t, "header-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
	require.Empty(t, capturedInvokeOpt.CreateOpt[constant.SchedulerIDNote])
	require.Empty(t, capturedInvokeOpt.SchedulerInstanceIDs)

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)
}

func TestCreateHandlerFallsBackToBodyTenant(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-from-body-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	var capturedFuncMeta api.FunctionMeta
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedFuncMeta = funcMeta
			capturedInvokeOpt = invokeOpt
			return "instance-from-body", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-b",
		Namespace: "sandbox",
		Tenant:    "body-tenant",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.Equal(t, sandboxCreateTimeoutSeconds, capturedInvokeOpt.Timeout)
	require.Equal(t, defaultSandboxFunctionID, capturedInvokeOpt.CreateOpt[constant.FunctionKeyNote])
	require.Equal(t, "body-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
	require.Empty(t, capturedInvokeOpt.CreateOpt[constant.SchedulerIDNote])
	require.Empty(t, capturedInvokeOpt.SchedulerInstanceIDs)
}

func TestCreateHandlerReturnsInstanceIDWhenCreateTimesOutAfterScheduling(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-timeout-test", nil
	}
	oldWaitForSandboxInstanceRunning := waitForSandboxInstanceRunning
	waitCalled := false
	waitForSandboxInstanceRunning = func(instanceID, functionID, resourceSpecNote string) bool {
		waitCalled = true
		require.Equal(t, "instance-created-late", instanceID)
		require.Equal(t, defaultSandboxFunctionID, functionID)
		require.NotEmpty(t, resourceSpecNote)
		return true
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
		waitForSandboxInstanceRunning = oldWaitForSandboxInstanceRunning
	}()

	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			return "instance-created-late", api.ErrorInfo{
				Code: createInstanceTimeoutCode,
				Err:  fmt.Errorf("create instance timeout"),
			}
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-c",
		Namespace: "sandbox",
		Tenant:    "body-tenant",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)

	var data map[string]string
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	instanceID, ok := data["instance_id"]
	require.True(t, ok)
	require.Equal(t, "instance-created-late", instanceID)
	require.True(t, waitCalled)
}

func TestSandboxFunctionIDUsesRuntime(t *testing.T) {
	tests := []struct {
		name      string
		runtime   string
		wantFunc  string
		wantError string
	}{
		{
			name:     "python310",
			runtime:  "python3.10",
			wantFunc: "default/0-defaultservice-py310/$latest",
		},
		{
			name:     "python39",
			runtime:  "python3.9",
			wantFunc: "default/0-defaultservice-py39/$latest",
		},
		{
			name:     "py310",
			runtime:  "py310",
			wantFunc: "default/0-defaultservice-py310/$latest",
		},
		{
			name:     "empty uses default",
			runtime:  "",
			wantFunc: "default/0-defaultservice-py310/$latest",
		},
		{
			name:      "unsupported",
			runtime:   "python3.11",
			wantError: "unsupported sandbox runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sandboxFunctionIDForRuntime(tt.runtime)
			if tt.wantError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantFunc, got)
		})
	}
}

func TestDefaultSandboxFunctionIDUsesBuiltInPy310Service(t *testing.T) {
	require.Equal(t, "default/0-defaultservice-py310/$latest", defaultSandboxFunctionID)
}

func TestCreateHandlerUsesRequestedRuntime(t *testing.T) {
	var capturedInvokeOpt api.InvokeOptions
	var capturedFuncMeta api.FunctionMeta
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedFuncMeta = funcMeta
			capturedInvokeOpt = invokeOpt
			return "instance-runtime", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-runtime",
		Namespace: "sandbox",
		Tenant:    "body-tenant",
		Runtime:   "python3.9",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "default/0-defaultservice-py39/$latest", capturedFuncMeta.FuncID)
	require.Equal(t, "default/0-defaultservice-py39/$latest", capturedInvokeOpt.CreateOpt[constant.FunctionKeyNote])
}

func TestCreateHandlerRejectsUnsupportedRuntime(t *testing.T) {
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			t.Fatalf("createInstance should not be called for unsupported runtime")
			return "", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-runtime",
		Namespace: "sandbox",
		Tenant:    "body-tenant",
		Runtime:   "python3.11",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "unsupported sandbox runtime")
}

func TestCreateHandlerAddsSchedulerCreateOptions(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-options-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "instance-with-options", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-d",
		Namespace: "sandbox",
		Tenant:    "body-tenant",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "detached", capturedInvokeOpt.CustomExtensions["lifecycle"])
	require.Equal(t, sandboxConcurrency, capturedInvokeOpt.CustomExtensions["Concurrency"])
	require.Equal(t, "reserved", capturedInvokeOpt.CreateOpt[constant.InstanceTypeNote])
	_, hasStaticOwner := capturedInvokeOpt.CreateOpt["resource.owner"]
	require.False(t, hasStaticOwner)
	require.Equal(t, fmt.Sprintf("%d", sandboxCreateTimeoutSeconds), capturedInvokeOpt.CreateOpt["call_timeout"])
	require.Equal(t, "305", capturedInvokeOpt.CreateOpt["init_call_timeout"])
	require.Equal(t, "900", capturedInvokeOpt.CreateOpt["GRACEFUL_SHUTDOWN_TIME"])
	require.Equal(t, "/tmp", capturedInvokeOpt.CreateOpt["DELEGATE_DIRECTORY_INFO"])
	require.Equal(t, "512", capturedInvokeOpt.CreateOpt["DELEGATE_DIRECTORY_QUOTA"])
	require.Equal(t, "1", capturedInvokeOpt.CreateOpt["ConcurrentNum"])

	var resSpec resspeckey.ResourceSpecification
	require.NoError(t, json.Unmarshal([]byte(capturedInvokeOpt.CreateOpt[constant.ResourceSpecNote]), &resSpec))
	require.EqualValues(t, defaultSandboxCPU, resSpec.CPU)
	require.EqualValues(t, defaultSandboxMemory, resSpec.Memory)
	require.Equal(t, "", resSpec.InvokeLabel)
	require.Empty(t, capturedInvokeOpt.SchedulerInstanceIDs)
}

func TestCreateHandlerPassesRootfsToSandboxCustomExtensions(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-rootfs-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "instance-with-rootfs", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-rootfs",
		Namespace: "sandbox",
		Rootfs:    "python:3.12-slim",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "python:3.12-slim", capturedInvokeOpt.CustomExtensions["rootfs"])
}

func TestCreateHandlerAcceptsImageAliasForRootfs(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-image-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "instance-with-image", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-image",
		Namespace: "sandbox",
		Image:     "ubuntu:22.04",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "ubuntu:22.04", capturedInvokeOpt.CustomExtensions["rootfs"])
}

func TestCreateHandlerPassesPortForwardingsToNetworkCreateOption(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-ports-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "instance-with-ports", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-ports",
		Namespace: "sandbox",
		Ports:     []string{"8080", "udp:9090"},
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(
		t,
		`{"portForwardings":[{"port":8080,"protocol":"TCP"},{"port":9090,"protocol":"UDP"}]}`,
		capturedInvokeOpt.CreateOpt["network"],
	)
}

func TestCreateHandlerRejectsInvalidPortForwarding(t *testing.T) {
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			t.Fatalf("createInstance should not be called for invalid port forwarding")
			return "", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-ports",
		Namespace: "sandbox",
		Ports:     []string{"sctp:8080"},
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "protocol must be TCP or UDP")
}

func TestCreateHandlerBuildsBuiltinDetachedSandboxRequest(t *testing.T) {
	oldSelectScheduler := selectSandboxSchedulerID
	selectSandboxSchedulerID = func(string) (string, error) {
		return "scheduler-contract-test", nil
	}
	defer func() {
		selectSandboxSchedulerID = oldSelectScheduler
	}()

	var capturedInvokeOpt api.InvokeOptions
	var capturedFuncMeta api.FunctionMeta
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedFuncMeta = funcMeta
			capturedInvokeOpt = invokeOpt
			return "instance-contract", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{
		Name:      "sandbox-f",
		Namespace: "sandbox",
		Tenant:    "contract-tenant",
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.Equal(t, api.Python, capturedFuncMeta.Language)
	require.Equal(t, api.ActorApi, capturedFuncMeta.Api)
	require.NotNil(t, capturedFuncMeta.Name)
	require.NotNil(t, capturedFuncMeta.Namespace)
	require.Equal(t, "sandbox-f", *capturedFuncMeta.Name)
	require.Equal(t, "sandbox", *capturedFuncMeta.Namespace)

	require.Equal(t, "detached", capturedInvokeOpt.CustomExtensions["lifecycle"])
	require.Equal(t, sandboxConcurrency, capturedInvokeOpt.CustomExtensions["Concurrency"])
	require.Equal(t, "yr.sandbox.sandbox", capturedInvokeOpt.CreateOpt["moduleName"])
	require.Equal(t, "SandboxInstance", capturedInvokeOpt.CreateOpt["className"])
	require.Equal(t, "contract-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
	require.Equal(t, defaultSandboxFunctionID, capturedInvokeOpt.CreateOpt[constant.FunctionKeyNote])
	_, hasStaticOwner := capturedInvokeOpt.CreateOpt["resource.owner"]
	require.False(t, hasStaticOwner)
	require.Equal(t, sandboxInstanceType, capturedInvokeOpt.CreateOpt[constant.InstanceTypeNote])
	require.Empty(t, capturedInvokeOpt.CreateOpt[constant.SchedulerIDNote])
}

func TestDeleteHandlerDeletesSandboxInstance(t *testing.T) {
	var (
		capturedInstanceID string
		capturedSignal     int
		capturedPayload    []byte
	)
	util.SetAPIClientLibruntime(&runtimeStub{
		kill: func(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error {
			capturedInstanceID = instanceID
			capturedSignal = signal
			capturedPayload = append([]byte(nil), payload...)
			require.Equal(t, api.InvokeOptions{}, invokeOpt)
			return nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "instanceId", Value: "sandbox-delete-ok"}}
	req, err := http.NewRequest(http.MethodDelete, "/api/sandbox/sandbox-delete-ok", nil)
	require.NoError(t, err)
	ctx.Request = req

	DeleteHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "sandbox-delete-ok", capturedInstanceID)
	require.Equal(t, constant.KillSignalVal, capturedSignal)
	require.Equal(t, []byte("sandbox deleted"), capturedPayload)

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)

	var data map[string]string
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	status, ok := data["status"]
	require.True(t, ok)
	require.Equal(t, "deleted", status)
}

func TestDeleteHandlerReturns500WhenKillFails(t *testing.T) {
	util.SetAPIClientLibruntime(&runtimeStub{
		kill: func(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error {
			return fmt.Errorf("kill failed")
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "instanceId", Value: "sandbox-delete-fail"}}
	req, err := http.NewRequest(http.MethodDelete, "/api/sandbox/sandbox-delete-fail", nil)
	require.NoError(t, err)
	ctx.Request = req

	DeleteHandler(ctx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "failed to delete sandbox")
}
