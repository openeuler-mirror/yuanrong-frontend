package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/sandboxrouter/execendpoint"
)

func TestGetExecAddrUsesLocalCache(t *testing.T) {
	oldLookup := lookupLocalExecEndpoint
	oldQueryMaster := queryMasterFunc
	defer func() {
		lookupLocalExecEndpoint = oldLookup
		queryMasterFunc = oldQueryMaster
	}()

	lookupLocalExecEndpoint = func(instanceID string) (execendpoint.Endpoint, bool) {
		if instanceID != "inst-1" {
			t.Fatalf("unexpected instanceID %q", instanceID)
		}
		return execendpoint.Endpoint{
			InstanceID:       "inst-1",
			ProxyGrpcAddress: "10.0.0.5:22774",
			ContainerID:      "sbox-xyz",
		}, true
	}
	queryMasterFunc = func(string, map[string]string, interface{}) error {
		t.Fatal("master must not be queried on local cache hit")
		return nil
	}

	info, err := getExecAddr("inst-1", "default")
	if err != nil {
		t.Fatalf("getExecAddr returned error: %v", err)
	}
	if info.ProxyGrpcAddress != "10.0.0.5:22774" {
		t.Errorf("ProxyGrpcAddress = %q, want 10.0.0.5:22774", info.ProxyGrpcAddress)
	}
	if info.ContainerID != "sbox-xyz" {
		t.Errorf("ContainerID = %q, want sbox-xyz", info.ContainerID)
	}
	if info.InstanceID != "inst-1" {
		t.Errorf("InstanceID = %q, want inst-1", info.InstanceID)
	}
}

func TestGetExecAddrFallsBackToMaster(t *testing.T) {
	oldLookup := lookupLocalExecEndpoint
	oldQueryMaster := queryMasterFunc
	defer func() {
		lookupLocalExecEndpoint = oldLookup
		queryMasterFunc = oldQueryMaster
	}()

	// Local cache miss.
	lookupLocalExecEndpoint = func(string) (execendpoint.Endpoint, bool) {
		return execendpoint.Endpoint{}, false
	}
	called := false
	queryMasterFunc = func(apiPath string, params map[string]string, result interface{}) error {
		called = true
		resp := result.(*InstanceListResponse)
		resp.Instances = []InstanceInfo{{
			InstanceID:       "inst-2",
			ProxyGrpcAddress: "10.0.0.6:22774",
			ContainerID:      "sbox-from-master",
		}}
		return nil
	}

	info, err := getExecAddr("inst-2", "default")
	if err != nil {
		t.Fatalf("getExecAddr returned error: %v", err)
	}
	if !called {
		t.Error("master query should be invoked on local cache miss")
	}
	if info.ProxyGrpcAddress != "10.0.0.6:22774" || info.ContainerID != "sbox-from-master" {
		t.Errorf("unexpected fallback result: %+v", info)
	}
}

func TestGetExecAddrLocalHitWithEmptyProxyFallsBack(t *testing.T) {
	oldLookup := lookupLocalExecEndpoint
	oldQueryMaster := queryMasterFunc
	defer func() {
		lookupLocalExecEndpoint = oldLookup
		queryMasterFunc = oldQueryMaster
	}()

	// Cache hit but with no usable proxy address must not short-circuit; it
	// should fall through to the master query.
	lookupLocalExecEndpoint = func(string) (execendpoint.Endpoint, bool) {
		return execendpoint.Endpoint{InstanceID: "inst-3", ProxyGrpcAddress: ""}, true
	}
	called := false
	queryMasterFunc = func(string, map[string]string, interface{}) error {
		called = true
		return nil
	}

	_, _ = getExecAddr("inst-3", "default")
	if !called {
		t.Error("empty-proxy local hit should fall back to master query")
	}
}

func TestParseCommandSplitsArguments(t *testing.T) {
	got := parseCommand("python3 -m yr.cli --version")
	want := []string{"python3", "-m", "yr.cli", "--version"}
	if len(got) != len(want) {
		t.Fatalf("len(parseCommand()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseCommand()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHandleInstancesIncludesTenantID(t *testing.T) {
	info := execendpoint.Summary{
		InstanceID: "instance-1",
		TenantID:   "tenant-1",
		StatusCode: int32(constant.KernelInstanceStatusRunning),
		StatusMsg:  "running",
		StartTime:  "1700000000",
		Resources: map[string]execendpoint.Resource{
			"CPU":          localResource(500),
			"Memory":       localResource(1024),
			"GPU":          localResource(1),
			"NPU/.+/count": localResource(2),
		},
	}
	oldLookupSummaries := lookupLocalInstanceSummaries
	oldQueryMasterFunc := queryMasterFunc
	lookupLocalInstanceSummaries = func(tenantID, instanceID string) []execendpoint.Summary {
		if tenantID != "tenant-1" || instanceID != "" {
			t.Fatalf("unexpected local filters tenant=%q instance=%q", tenantID, instanceID)
		}
		return []execendpoint.Summary{info}
	}
	queryMasterFunc = func(string, map[string]string, interface{}) error {
		t.Fatal("master must not be queried for local running instance list")
		return nil
	}
	defer func() {
		lookupLocalInstanceSummaries = oldLookupSummaries
		queryMasterFunc = oldQueryMasterFunc
	}()

	req := httptest.NewRequest(http.MethodGet, "/api/instances?tenant_id=tenant-1", nil)
	recorder := httptest.NewRecorder()

	HandleInstances(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HandleInstances status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"tenantID":"tenant-1"`) {
		t.Fatalf("expected tenantID in response body, got %s", recorder.Body.String())
	}

	var body []map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	if body[0]["required_cpu"] != float64(500) ||
		body[0]["required_mem"] != float64(1024) ||
		body[0]["required_gpu"] != float64(1) ||
		body[0]["required_npu"] != float64(2) {
		t.Fatalf("expected resource quota fields, got %+v", body[0])
	}
	if runtimeSeconds, ok := body[0]["runtime_seconds"].(float64); !ok || runtimeSeconds <= 0 {
		t.Fatalf("expected positive runtime_seconds, got %+v", body[0]["runtime_seconds"])
	}
}

func TestHandleIndexIncludesSandboxCreateResponseParser(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/terminal/", nil)
	req.Header.Set("X-Forwarded-Prefix", "/frontend")
	recorder := httptest.NewRecorder()

	HandleIndex(recorder, req)

	body := recorder.Body.String()
	if !strings.Contains(body, "function extractSandboxCreateInstanceId(result)") {
		t.Fatalf("expected sandbox create response parser helper in page")
	}
	if !strings.Contains(body, "const effectiveTenant = parseTenantFromJWT(token) || tenant;") {
		t.Fatalf("expected sandbox create flow to prefer JWT tenant when redirecting")
	}
}

func TestHandleIndexDoesNotPrefixSandboxAPIWithTerminalPath(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/terminal/", nil)
	req.Header.Set("X-Forwarded-Prefix", "/frontend/terminal")
	recorder := httptest.NewRecorder()

	HandleIndex(recorder, req)

	body := recorder.Body.String()
	if strings.Contains(body, "/frontend/terminal/api/sandbox/create") {
		t.Fatalf("expected sandbox create API URL to strip trailing /terminal from forwarded prefix")
	}
	if !strings.Contains(body, "/frontend/api/sandbox/create") {
		t.Fatalf("expected sandbox create API URL to target sibling /api path under forwarded prefix")
	}
	if !strings.Contains(body, "const jobsApiUrl = '/frontend/api/jobs';") {
		t.Fatalf("expected jobs API URL to target sibling /api path under forwarded prefix")
	}
	if !strings.Contains(body, "fetch('/frontend/api/sandbox/' + encodeURIComponent(instanceId), fetchOptions);") {
		t.Fatalf("expected sandbox delete API URL to target sibling /api path under forwarded prefix")
	}
	if !strings.Contains(body, "const response = await fetch('/frontend/api/instances?' + apiParams.toString(), fetchOptions);") {
		t.Fatalf("expected instances API URL to target sibling /api path under forwarded prefix")
	}
}

func TestHandleInstancesUsesLocalCacheWithPagination(t *testing.T) {
	originalLookupSummaries := lookupLocalInstanceSummaries
	originalQueryMaster := queryMasterFunc
	defer func() {
		lookupLocalInstanceSummaries = originalLookupSummaries
		queryMasterFunc = originalQueryMaster
	}()

	lookupLocalInstanceSummaries = func(tenantID, instanceID string) []execendpoint.Summary {
		if tenantID != "tenant-a" || instanceID != "" {
			t.Fatalf("unexpected local filters tenant=%q instance=%q", tenantID, instanceID)
		}
		return []execendpoint.Summary{
			{InstanceID: "instance-1", TenantID: "tenant-a", StatusCode: int32(constant.KernelInstanceStatusRunning)},
			{InstanceID: "instance-2", TenantID: "tenant-a", Function: "func-a", StatusCode: int32(constant.KernelInstanceStatusRunning)},
			{InstanceID: "instance-3", TenantID: "tenant-a", StatusCode: int32(constant.KernelInstanceStatusRunning)},
		}
	}
	queryMasterFunc = func(string, map[string]string, interface{}) error {
		t.Fatal("master must not be queried for local running instance list")
		return nil
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/instances?tenant_id=tenant-a&page=2&page_size=1",
		nil,
	)
	recorder := httptest.NewRecorder()

	HandleInstances(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Instances []map[string]interface{} `json:"instances"`
		Count     int                      `json:"count"`
		Page      int                      `json:"page"`
		PageSize  int                      `json:"pageSize"`
		TenantID  string                   `json:"tenantID"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Count != 3 || body.Page != 2 || body.PageSize != 1 || body.TenantID != "tenant-a" {
		t.Fatalf("unexpected pagination metadata: %+v", body)
	}
	if len(body.Instances) != 1 || body.Instances[0]["id"] != "instance-2" {
		t.Fatalf("unexpected instances: %+v", body.Instances)
	}
}

func TestHandleInstancesRejectsInvalidPaginationBeforeLocalLookup(t *testing.T) {
	originalLookupSummaries := lookupLocalInstanceSummaries
	defer func() {
		lookupLocalInstanceSummaries = originalLookupSummaries
	}()
	lookupLocalInstanceSummaries = func(string, string) []execendpoint.Summary {
		t.Fatal("local cache must not be queried for invalid pagination")
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances?tenant_id=tenant-a&page=0&page_size=10", nil)
	recorder := httptest.NewRecorder()

	HandleInstances(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "page must be a positive integer") {
		t.Fatalf("expected pagination error body, got %q", recorder.Body.String())
	}
}

func localResource(value float64) execendpoint.Resource {
	var resource execendpoint.Resource
	resource.Scalar.Value = value
	return resource
}
