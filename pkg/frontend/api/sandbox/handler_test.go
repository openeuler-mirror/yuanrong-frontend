package sandbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/ugorji/go/codec"
	"yuanrong.org/kernel/runtime/libruntime/api"

	faasconstant "frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/resspeckey"
	"frontend/pkg/common/job"
	"frontend/pkg/frontend/common/jwtauth"
	"frontend/pkg/frontend/common/util"
)

type runtimeStub struct {
	createInstance func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error)
	invokeInstance func(funcMeta api.FunctionMeta, instanceID string, args []api.Arg, invokeOpt api.InvokeOptions) (string, error)
	getAsync       func(objectID string, cb api.GetAsyncCallback)
	kill           func(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error
}

func (r *runtimeStub) CreateInstance(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
	if r.createInstance != nil {
		return r.createInstance(funcMeta, args, invokeOpt)
	}
	return "", nil
}

func (r *runtimeStub) InvokeByInstanceId(funcMeta api.FunctionMeta, instanceID string, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
	if r.invokeInstance != nil {
		return r.invokeInstance(funcMeta, instanceID, args, invokeOpt)
	}
	return "", nil
}

func (r *runtimeStub) InvokeByFunctionName(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
	return "", nil
}

func (r *runtimeStub) AcquireInstance(state string, funcMeta api.FunctionMeta, acquireOpt api.InvokeOptions) (api.InstanceAllocation, error) {
	return api.InstanceAllocation{}, nil
}

func (r *runtimeStub) ReleaseInstance(allocation api.InstanceAllocation, stateID string, abnormal bool, option api.InvokeOptions) {
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

func (r *runtimeStub) GetAsync(objectID string, cb api.GetAsyncCallback) {
	if r.getAsync != nil {
		r.getAsync(objectID, cb)
	}
}

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
	ctx.Request.Header.Set(faasconstant.HeaderTenantID, "header-tenant")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.Equal(t, sandboxCreateTimeoutSeconds, capturedInvokeOpt.Timeout)
	require.Equal(t, defaultSandboxFunctionID, capturedInvokeOpt.CreateOpt[faasconstant.FunctionKeyNote])
	require.Equal(t, "header-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
	require.Empty(t, capturedInvokeOpt.CreateOpt[faasconstant.SchedulerIDNote])
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.Equal(t, sandboxCreateTimeoutSeconds, capturedInvokeOpt.Timeout)
	require.Equal(t, defaultSandboxFunctionID, capturedInvokeOpt.CreateOpt[faasconstant.FunctionKeyNote])
	require.Equal(t, "body-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
	require.Empty(t, capturedInvokeOpt.CreateOpt[faasconstant.SchedulerIDNote])
	require.Empty(t, capturedInvokeOpt.SchedulerInstanceIDs)
}

func TestCreateHandlerAttributesTenantFromTokenClaim(t *testing.T) {
	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(_ api.FunctionMeta, _ []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "instance-from-token", nil
		},
	})

	encode := func(value interface{}) string {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(data)
	}
	token := encode(jwtauth.JWTHeader{Alg: "none", Typ: "JWT"}) + "." +
		encode(jwtauth.JWTPayload{Sub: "token-tenant"}) + ".sig"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateRequest{Name: "sandbox-token", Namespace: "sandbox"})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)
	ctx.Request.Header.Set(jwtauth.HeaderXAuth, token)

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "token-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
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
				Code: 3002,
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)

	var data map[string]string
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Equal(t, "instance-created-late", data["instance_id"])
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
			name:     "rust",
			runtime:  "rust",
			wantFunc: "default/0-defaultservice-rrt/$latest",
		},
		{
			name:     "rrt",
			runtime:  "rrt",
			wantFunc: "default/0-defaultservice-rrt/$latest",
		},
		{
			name:     "empty uses default (rust)",
			runtime:  "",
			wantFunc: "default/0-defaultservice-rrt/$latest",
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

func TestDefaultSandboxFunctionIDUsesRustService(t *testing.T) {
	// Default sandbox backend is the dedicated Rust (rrt) slot.
	require.Equal(t, "default/0-defaultservice-rrt/$latest", defaultSandboxFunctionID)
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "default/0-defaultservice-py39/$latest", capturedFuncMeta.FuncID)
	require.Equal(t, "default/0-defaultservice-py39/$latest", capturedInvokeOpt.CreateOpt[faasconstant.FunctionKeyNote])
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "detached", capturedInvokeOpt.CustomExtensions["lifecycle"])
	require.Equal(t, sandboxConcurrency, capturedInvokeOpt.CustomExtensions["Concurrency"])
	require.Equal(t, "reserved", capturedInvokeOpt.CreateOpt[faasconstant.InstanceTypeNote])
	_, hasStaticOwner := capturedInvokeOpt.CreateOpt["resource.owner"]
	require.False(t, hasStaticOwner)
	require.Equal(t, fmt.Sprintf("%d", sandboxCreateTimeoutSeconds), capturedInvokeOpt.CreateOpt["call_timeout"])
	require.Equal(t, "305", capturedInvokeOpt.CreateOpt["init_call_timeout"])
	require.Equal(t, "5", capturedInvokeOpt.CreateOpt["GRACEFUL_SHUTDOWN_TIME"])
	require.Equal(t, "/tmp", capturedInvokeOpt.CreateOpt["DELEGATE_DIRECTORY_INFO"])
	require.Equal(t, "512", capturedInvokeOpt.CreateOpt["DELEGATE_DIRECTORY_QUOTA"])
	require.Equal(t, "1", capturedInvokeOpt.CreateOpt["ConcurrentNum"])

	var resSpec resspeckey.ResourceSpecification
	require.NoError(t, json.Unmarshal([]byte(capturedInvokeOpt.CreateOpt[faasconstant.ResourceSpecNote]), &resSpec))
	require.EqualValues(t, 1000, resSpec.CPU)
	require.EqualValues(t, 2048, resSpec.Memory)
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

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
		Ports:     []string{"8080", "https:9090"},
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/create", bytes.NewReader(body))
	require.NoError(t, err)
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(
		t,
		`{"portForwardings":[{"port":8080,"protocol":"http"},{"port":9090,"protocol":"https"}]}`,
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "port scheme must be http or https")
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
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-create")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01")

	CreateHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.Equal(t, "yr.sandbox.sandbox", capturedFuncMeta.ModuleName)
	require.Equal(t, "SandboxInstance", capturedFuncMeta.ClassName)
	require.Equal(t, api.Python, capturedFuncMeta.Language)
	require.Equal(t, api.ActorApi, capturedFuncMeta.Api)
	require.NotNil(t, capturedFuncMeta.Name)
	require.NotNil(t, capturedFuncMeta.Namespace)
	require.Equal(t, "sandbox-f", *capturedFuncMeta.Name)
	require.Equal(t, "sandbox", *capturedFuncMeta.Namespace)

	require.Equal(t, "detached", capturedInvokeOpt.CustomExtensions["lifecycle"])
	require.Equal(t, sandboxConcurrency, capturedInvokeOpt.CustomExtensions["Concurrency"])
	require.Equal(t, "trace-create", capturedInvokeOpt.TraceID)
	require.Equal(t, "00-123e4567e89b12d3a456426614174000-0123456789abcdef-01", capturedInvokeOpt.CustomExtensions["traceparent"])
	require.Equal(t, "trace-create", recorder.Header().Get(faasconstant.HeaderTraceID))
	require.Equal(t, "contract-tenant", capturedInvokeOpt.CreateOpt["tenantId"])
	require.Equal(t, defaultSandboxFunctionID, capturedInvokeOpt.CreateOpt[faasconstant.FunctionKeyNote])
	_, hasStaticOwner := capturedInvokeOpt.CreateOpt["resource.owner"]
	require.False(t, hasStaticOwner)
	require.Equal(t, sandboxInstanceType, capturedInvokeOpt.CreateOpt[faasconstant.InstanceTypeNote])
	require.Empty(t, capturedInvokeOpt.CreateOpt[faasconstant.SchedulerIDNote])
}

func TestCreateV1HandlerDefaultsAndReturnsSandboxID(t *testing.T) {
	var capturedInvokeOpt api.InvokeOptions
	var capturedFuncMeta api.FunctionMeta
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedFuncMeta = funcMeta
			capturedInvokeOpt = invokeOpt
			return "sandbox-v1", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateV1Request{
		Image:              "ubuntu:22.04",
		IdleTimeoutSeconds: 123,
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes", bytes.NewReader(body))
	require.NoError(t, err)

	CreateV1Handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, defaultSandboxFunctionID, capturedFuncMeta.FuncID)
	require.NotNil(t, capturedFuncMeta.Name)
	require.NotEmpty(t, *capturedFuncMeta.Name)
	require.NotNil(t, capturedFuncMeta.Namespace)
	require.Equal(t, "default", *capturedFuncMeta.Namespace)
	require.Equal(t, "123", capturedInvokeOpt.CustomExtensions["idle_timeout"])
	require.JSONEq(t, `{"runtime":"runsc","type":"image","imageurl":"ubuntu:22.04"}`, capturedInvokeOpt.CustomExtensions["rootfs"])

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	var data map[string]string
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Equal(t, "sandbox-v1", data["sandboxId"])
	require.Equal(t, "running", data["status"])
}

func TestCreateV1HandlerSSEUsesRequestedTimeoutAndReturnsFinal(t *testing.T) {
	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "sandbox-sse", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateV1Request{
		Name:                 "sandbox-sse",
		Namespace:            "default",
		Image:                "ubuntu:22.04",
		CreateTimeoutSeconds: 37,
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes", bytes.NewReader(body))
	require.NoError(t, err)
	ctx.Request.Header.Set("Accept", "text/event-stream")

	CreateV1Handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, recorder.Body.String(), "event: accepted")
	require.Contains(t, recorder.Body.String(), `"status":"creating"`)
	require.Contains(t, recorder.Body.String(), "event: final")
	require.Contains(t, recorder.Body.String(), `"sandboxId":"sandbox-sse"`)
	require.Contains(t, recorder.Body.String(), `"status":"running"`)
	require.Equal(t, 37, capturedInvokeOpt.Timeout)
	require.Equal(t, int64(7000), capturedInvokeOpt.ScheduleTimeoutMs)
	require.Equal(t, "37", capturedInvokeOpt.CreateOpt["call_timeout"])
}

func TestResolveSandboxCreateTimeouts(t *testing.T) {
	t.Setenv("YR_SANDBOX_CREATE_TIMEOUT", "")

	tests := []struct {
		name             string
		createTimeout    int
		scheduleTimeout  int
		wantCreate       int
		wantSchedule     int
		wantErrorMessage string
	}{
		{
			name:         "default create derives schedule",
			wantCreate:   60,
			wantSchedule: 30,
		},
		{
			name:          "create derives schedule",
			createTimeout: 120,
			wantCreate:    120,
			wantSchedule:  90,
		},
		{
			name:            "schedule derives create",
			scheduleTimeout: 90,
			wantCreate:      120,
			wantSchedule:    90,
		},
		{
			name:             "create must exceed buffer",
			createTimeout:    30,
			wantErrorMessage: "createTimeoutSeconds must be greater than 30",
		},
		{
			name:             "create must be positive",
			createTimeout:    -1,
			wantErrorMessage: "createTimeoutSeconds must be a positive integer",
		},
		{
			name:             "schedule must be positive",
			scheduleTimeout:  -1,
			wantErrorMessage: "scheduleTimeoutSeconds must be a positive integer",
		},
		{
			name:             "schedule must not exceed create",
			createTimeout:    60,
			scheduleTimeout:  70,
			wantErrorMessage: "scheduleTimeoutSeconds must be less than or equal to createTimeoutSeconds",
		},
		{
			name:             "explicit timeouts must reserve buffer",
			createTimeout:    60,
			scheduleTimeout:  45,
			wantErrorMessage: "createTimeoutSeconds - scheduleTimeoutSeconds must be at least 30",
		},
		{
			name:            "explicit timeouts preserve caller budgets",
			createTimeout:   120,
			scheduleTimeout: 80,
			wantCreate:      120,
			wantSchedule:    80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createTimeout, scheduleTimeout, err := resolveSandboxCreateTimeouts(
				tt.createTimeout, tt.scheduleTimeout,
			)
			if tt.wantErrorMessage != "" {
				require.EqualError(t, err, tt.wantErrorMessage)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantCreate, createTimeout)
			require.Equal(t, tt.wantSchedule, scheduleTimeout)
		})
	}
}

func TestCreateV1HandlerSSEDoesNotReportUnconfirmedTimeoutAsRunning(t *testing.T) {
	oldWaitForSandboxInstanceRunning := waitForSandboxInstanceRunning
	waitForSandboxInstanceRunning = func(instanceID, functionID, resourceSpecNote string) bool {
		return false
	}
	defer func() { waitForSandboxInstanceRunning = oldWaitForSandboxInstanceRunning }()

	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			return "sandbox-timeout", api.ErrorInfo{Code: 3002, Err: fmt.Errorf("create instance timeout")}
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateV1Request{Name: "sandbox-timeout", Namespace: "default"})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes", bytes.NewReader(body))
	require.NoError(t, err)
	ctx.Request.Header.Set("Accept", "text/event-stream")

	CreateV1Handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "event: final")
	require.Contains(t, recorder.Body.String(), `"sandboxId":"sandbox-timeout"`)
	require.Contains(t, recorder.Body.String(), `"status":"timeout"`)
	require.Contains(t, recorder.Body.String(), `"errorCode":3002`)
	require.NotContains(t, recorder.Body.String(), `"status":"running"`)
}

func TestCreateV1HandlerFrontendOwnsTunnelSetup(t *testing.T) {
	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "default/sandbox_demo.1", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateV1Request{
		Image:  "ubuntu:22.04",
		Env:    map[string]string{"RRT_HTTP_PORT": "19000", "RRT_TUNNEL_WS_PORT": "19001", "USER_ENV": "ok"},
		Tunnel: TunnelSpec{Enabled: true},
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes", bytes.NewReader(body))
	require.NoError(t, err)

	CreateV1Handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(
		t,
		`{"portForwardings":[{"port":50090,"protocol":"http"},{"port":8765,"protocol":"http"},{"port":8766,"protocol":"http"}]}`,
		capturedInvokeOpt.CreateOpt["network"],
	)
	require.JSONEq(
		t,
		`{"RRT_HTTP_PORT":"50090","RRT_TUNNEL_WS_PORT":"8765","RRT_TUNNEL_HTTP_PORT":"8766","USER_ENV":"ok"}`,
		capturedInvokeOpt.CreateOpt[faasconstant.DelegateEnvVar],
	)

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	tunnel, ok := data["tunnel"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "/tunnel/default-sandbox-demo-1", tunnel["url"])
	require.Equal(t, "/tunnel/default-sandbox-demo-1", tunnel["path"])
	require.Equal(t, "http://127.0.0.1:8766", tunnel["proxyUrl"])
}

func TestCreateV1HandlerDoesNotInjectRRTPortForPythonRuntime(t *testing.T) {
	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		createInstance: func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedInvokeOpt = invokeOpt
			return "default/sandbox_python.1", nil
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body, err := json.Marshal(CreateV1Request{
		Runtime: "python3.9",
		Image:   "aio-yr-runtime:latest",
		Env:     map[string]string{"USER_ENV": "ok"},
	})
	require.NoError(t, err)
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes", bytes.NewReader(body))
	require.NoError(t, err)

	CreateV1Handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, capturedInvokeOpt.CreateOpt, "network")
	require.JSONEq(
		t,
		`{"USER_ENV":"ok"}`,
		capturedInvokeOpt.CreateOpt[faasconstant.DelegateEnvVar],
	)
}

func TestNormalizeJSONValuePreservesFractionalAndConvertsIntegers(t *testing.T) {
	got := normalizeJSONValue(map[string]interface{}{
		"pid":     float64(123),
		"timeout": float64(0.5),
		"nested":  []interface{}{float64(7)},
	}).(map[string]interface{})
	require.Equal(t, int64(123), got["pid"])
	require.Equal(t, float64(0.5), got["timeout"])
	require.Equal(t, int64(7), got["nested"].([]interface{})[0])
}

func TestInvokeV1HandlerRoutesEnvelopeToRRTSandboxInvoke(t *testing.T) {
	var capturedFuncMeta api.FunctionMeta
	var capturedInstanceID string
	var capturedArgs []api.Arg
	var capturedInvokeOpt api.InvokeOptions
	util.SetAPIClientLibruntime(&runtimeStub{
		invokeInstance: func(funcMeta api.FunctionMeta, instanceID string, args []api.Arg, invokeOpt api.InvokeOptions) (string, error) {
			capturedFuncMeta = funcMeta
			capturedInstanceID = instanceID
			capturedArgs = append([]api.Arg(nil), args...)
			capturedInvokeOpt = invokeOpt
			return "object-1", nil
		},
		getAsync: func(objectID string, cb api.GetAsyncCallback) {
			require.Equal(t, "object-1", objectID)
			result, err := encodeMsgpack(map[string]interface{}{"ok": true})
			require.NoError(t, err)
			cb(append(make([]byte, 16), result...), nil)
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "sandboxID", Value: "sandbox-123"}}
	body := []byte(`{"action":"process.exec","args":{"cmd":"echo hi","cwd":"/tmp"}}`)
	var err error
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes/sandbox-123/invoke", bytes.NewReader(body))
	require.NoError(t, err)
	ctx.Request.Header.Set(faasconstant.HeaderTraceID, "trace-invoke")
	ctx.Request.Header.Set(faasconstant.HeaderTraceParent, "00-abcdefabcdefabcdefabcdefabcdefab-0123456789abcdef-01")

	InvokeV1Handler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "sandbox-123", capturedInstanceID)
	require.Equal(t, "sandbox_invoke", capturedFuncMeta.FuncName)
	require.Equal(t, api.ActorApi, capturedFuncMeta.Api)
	require.Equal(t, "trace-invoke", capturedInvokeOpt.TraceID)
	require.Equal(t, "00-abcdefabcdefabcdefabcdefabcdefab-0123456789abcdef-01", capturedInvokeOpt.CustomExtensions["traceparent"])
	require.Equal(t, "trace-invoke", recorder.Header().Get(faasconstant.HeaderTraceID))
	require.Equal(t, sandboxCreateTimeoutSeconds, capturedInvokeOpt.Timeout)
	require.True(t, capturedInvokeOpt.BypassDataSystem)
	require.Len(t, capturedArgs, 4)
	decodedArgs := make(map[string]interface{}, len(capturedArgs)/2)
	for i := 0; i+1 < len(capturedArgs); i += 2 {
		key, err := decodePackedArg(capturedArgs[i].Data)
		require.NoError(t, err)
		value, err := decodePackedArg(capturedArgs[i+1].Data)
		require.NoError(t, err)
		decodedArgs[fmt.Sprint(key)] = value
	}
	require.Equal(t, "process.exec", decodedArgs["action"])
	require.Equal(t, map[string]interface{}{
		"cmd": "echo hi",
		"cwd": "/tmp",
	}, decodedArgs["args"])

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.JSONEq(t, `{"ok":true}`, string(resp.Data))
}

func decodePackedArg(data []byte) (interface{}, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("arg too short: %d", len(data))
	}
	var out interface{}
	dec := codec.NewDecoderBytes(data[16:], &msgpackHandle)
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return normalizeMsgpack(out), nil
}

func TestInvokeV1HandlerRejectsMissingAction(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "sandboxID", Value: "sandbox-123"}}
	var err error
	ctx.Request, err = http.NewRequest(http.MethodPost, "/api/sandbox/v1/sandboxes/sandbox-123/invoke", bytes.NewReader([]byte(`{"args":{}}`)))
	require.NoError(t, err)

	InvokeV1Handler(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "action is required")
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
	require.Equal(t, faasconstant.KillSignalVal, capturedSignal)
	require.Equal(t, []byte("sandbox deleted"), capturedPayload)

	var resp job.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, resp.Code)

	var data map[string]string
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Equal(t, "deleted", data["status"])
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
