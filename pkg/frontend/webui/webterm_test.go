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
	info := InstanceInfo{
		InstanceID: "instance-1",
		TenantID:   "tenant-1",
	}
	oldQueryMasterFunc := queryMasterFunc
	queryMasterFunc = func(apiPath string, queryParams map[string]string, result interface{}) error {
		response := result.(*InstanceListResponse)
		response.Instances = []InstanceInfo{info}
		return nil
	}
	defer func() { queryMasterFunc = oldQueryMasterFunc }()

	req := httptest.NewRequest(http.MethodGet, "/api/instances?tenant_id=tenant-1", nil)
	recorder := httptest.NewRecorder()

	HandleInstances(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HandleInstances status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"tenantID":"tenant-1"`) {
		t.Fatalf("expected tenantID in response body, got %s", recorder.Body.String())
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

func TestHandleInstancesForwardsPaginationToMaster(t *testing.T) {
	originalQueryMaster := queryMasterFunc
	defer func() {
		queryMasterFunc = originalQueryMaster
	}()

	queryMasterFunc = func(apiPath string, queryParams map[string]string, result interface{}) error {
		if apiPath != "/instance-manager/query-tenant-instances" {
			t.Fatalf("unexpected api path: %s", apiPath)
		}
		expectedParams := map[string]string{
			"tenant_id":   "tenant-a",
			"instance_id": "instance-2",
			"page":        "2",
			"page_size":   "1",
		}
		for key, expected := range expectedParams {
			if queryParams[key] != expected {
				t.Fatalf("expected query param %s=%s, got %q", key, expected, queryParams[key])
			}
		}

		response, ok := result.(*InstanceListResponse)
		if !ok {
			t.Fatalf("unexpected result type: %T", result)
		}
		*response = InstanceListResponse{
			Instances: []InstanceInfo{
				{
					InstanceID: "instance-2",
					TenantID:   "tenant-a",
					Function:   "func-a",
					InstanceStatus: InstanceStatus{
						Code: int(constant.KernelInstanceStatusRunning),
						Msg:  "running",
					},
				},
			},
			Count:    3,
			Page:     2,
			PageSize: 1,
			TenantID: "tenant-a",
		}
		return nil
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/instances?tenant_id=tenant-a&instance_id=instance-2&page=2&page_size=1",
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

func TestHandleInstancesPropagatesPaginatedMasterErrors(t *testing.T) {
	originalQueryMaster := queryMasterFunc
	defer func() {
		queryMasterFunc = originalQueryMaster
	}()

	queryMasterFunc = func(apiPath string, queryParams map[string]string, result interface{}) error {
		if apiPath != "/instance-manager/query-tenant-instances" {
			t.Fatalf("unexpected api path: %s", apiPath)
		}
		if queryParams["page_size"] != "1001" {
			t.Fatalf("expected page_size=1001, got %q", queryParams["page_size"])
		}
		return masterQueryError{
			statusCode: http.StatusBadRequest,
			body:       "{\"error\":\"page_size exceeds maximum limit\"}",
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances?tenant_id=tenant-a&page=1&page_size=1001", nil)
	recorder := httptest.NewRecorder()

	HandleInstances(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "page_size exceeds maximum limit") {
		t.Fatalf("expected master error body, got %q", recorder.Body.String())
	}
}
