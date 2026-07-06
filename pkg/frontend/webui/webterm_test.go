package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		if apiPath != "/instance-manager/query-tenant-instances" {
			t.Fatalf("apiPath = %q, want /instance-manager/query-tenant-instances", apiPath)
		}
		if params["tenant_id"] != "default" {
			t.Fatalf("tenant_id param = %q, want default", params["tenant_id"])
		}
		if params["instance_id"] != "inst-2" {
			t.Fatalf("instance_id param = %q, want inst-2", params["instance_id"])
		}
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
		Image:      "registry.example.com/ns/image:tag",
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
	if body[0]["limit_cpu"] != float64(1000) ||
		body[0]["limit_mem"] != float64(2048) ||
		body[0]["limit_gpu"] != float64(2) ||
		body[0]["limit_npu"] != float64(4) {
		t.Fatalf("expected resource limit fields, got %+v", body[0])
	}
	if body[0]["image"] != "registry.example.com/ns/image:tag" {
		t.Fatalf("expected image field, got %+v", body[0])
	}
	if body[0]["image_endpoint"] != "" {
		t.Fatalf("expected empty image_endpoint for registry image, got %+v", body[0])
	}
	if runtimeSeconds, ok := body[0]["runtime_seconds"].(float64); !ok || runtimeSeconds <= 0 {
		t.Fatalf("expected positive runtime_seconds, got %+v", body[0]["runtime_seconds"])
	}
}

func TestSummarizeInstancesExtractsImageFromRootfs(t *testing.T) {
	body := summarizeInstances(InstanceListResponse{Instances: []InstanceInfo{{
		InstanceID: "instance-1",
		CreateOptions: map[string]string{
			"rootfs": `{"runtime":"runsc","type":"image","readonly":false,"imageurl":"registry.example.com/ns/image:tag"}`,
		},
	}}})

	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	if body[0]["image"] != "registry.example.com/ns/image:tag" {
		t.Fatalf("image = %q, want registry.example.com/ns/image:tag", body[0]["image"])
	}
}

func TestSummarizeInstancesFormatsS3RootfsAsImage(t *testing.T) {
	body := summarizeInstances(InstanceListResponse{Instances: []InstanceInfo{{
		InstanceID: "instance-1",
		ScheduleOption: struct {
			Extension map[string]string `json:"extension"`
		}{
			Extension: map[string]string{
				"rootfs": `{"runtime":"runsc","type":"s3","imageurl":"registry.example.com","storageInfo":{"endpoint":"cn-hangzhou.example.com","bucket":"crfs-dev","object":"rootfs.img","accessKey":"secret-ak","secretKey":"secret-sk"}}`,
			},
		},
	}}})

	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	if body[0]["image"] != "s3://crfs-dev/rootfs.img" {
		t.Fatalf("image = %q, want s3://crfs-dev/rootfs.img", body[0]["image"])
	}
	if body[0]["image_endpoint"] != "cn-hangzhou.example.com" {
		t.Fatalf("image_endpoint = %q, want cn-hangzhou.example.com", body[0]["image_endpoint"])
	}
}

func TestSummarizeInstancesFormatsOSSRootfsAsS3Image(t *testing.T) {
	body := summarizeInstances(InstanceListResponse{Instances: []InstanceInfo{{
		InstanceID: "instance-1",
		ScheduleOption: struct {
			Extension map[string]string `json:"extension"`
		}{
			Extension: map[string]string{
				"rootfs": `{"runtime":"runsc","type":"s3","imageurl":"oss-cn-hangzhou.aliyuncs.com","storageInfo":{"endpoint":"https://oss-cn-hangzhou.aliyuncs.com","bucket":"yr-rootfs-prod","object":"/images/python310/rootfs.img","accessKey":"secret-ak","secretKey":"secret-sk"}}`,
			},
		},
	}}})

	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	if body[0]["image"] != "s3://yr-rootfs-prod/images/python310/rootfs.img" {
		t.Fatalf("image = %q, want s3://yr-rootfs-prod/images/python310/rootfs.img", body[0]["image"])
	}
	if body[0]["image_endpoint"] != "https://oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("image_endpoint = %q, want https://oss-cn-hangzhou.aliyuncs.com", body[0]["image_endpoint"])
	}
}

func TestSummarizeInstancesMatchesConcreteGPUAndNPUResourceKeys(t *testing.T) {
	body := summarizeInstances(InstanceListResponse{Instances: []InstanceInfo{{
		InstanceID: "instance-1",
		Resources: Resources{Resources: map[string]Resource{
			"GPU/NVIDIA-A10/count": {
				Scalar: ValueScalar{Value: 1, Limit: 2},
			},
			"NPU/Ascend910B4/count": {
				Scalar: ValueScalar{Value: 3, Limit: 4},
			},
		}},
	}}})

	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	if body[0]["required_gpu"] != float64(1) || body[0]["limit_gpu"] != float64(2) {
		t.Fatalf("expected concrete GPU resource to match, got %+v", body[0])
	}
	if body[0]["required_npu"] != float64(3) || body[0]["limit_npu"] != float64(4) {
		t.Fatalf("expected concrete NPU resource to match, got %+v", body[0])
	}
}

func TestSummarizeInstancesUsesPlainRootfsAsImage(t *testing.T) {
	body := summarizeInstances(InstanceListResponse{Instances: []InstanceInfo{{
		InstanceID: "instance-1",
		CreateOptions: map[string]string{
			"rootfs": "python:3.12-slim",
		},
	}}})

	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	if body[0]["image"] != "python:3.12-slim" {
		t.Fatalf("image = %q, want python:3.12-slim", body[0]["image"])
	}
}

func TestSummarizeLocalInstanceSummariesUsesObservedRunningAtFallback(t *testing.T) {
	summaries := []execendpoint.Summary{{
		InstanceID:        "instance-1",
		TenantID:          "tenant-1",
		StatusCode:        int32(constant.KernelInstanceStatusRunning),
		StatusMsg:         "running",
		ObservedRunningAt: time.Now().Add(-2 * time.Minute),
	}}

	body := summarizeLocalInstanceSummaries(summaries)
	if len(body) != 1 {
		t.Fatalf("expected one instance, got %+v", body)
	}
	runtimeSeconds, ok := body[0]["runtime_seconds"].(int64)
	if !ok {
		t.Fatalf("runtime_seconds type = %T, want int64", body[0]["runtime_seconds"])
	}
	if runtimeSeconds < 110 || runtimeSeconds > 130 {
		t.Fatalf("runtime_seconds = %d, want approximately 120", runtimeSeconds)
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
	resource.Scalar.Limit = value * 2
	return resource
}

func TestHandleWebSocketResolveFailureReturnsHTTPError(t *testing.T) {
	oldLookup := lookupLocalExecEndpoint
	oldQueryMaster := queryMasterFunc
	defer func() {
		lookupLocalExecEndpoint = oldLookup
		queryMasterFunc = oldQueryMaster
	}()

	lookupLocalExecEndpoint = func(string) (execendpoint.Endpoint, bool) {
		return execendpoint.Endpoint{}, false
	}
	queryMasterFunc = func(string, map[string]string, interface{}) error {
		return errors.New("master query timeout")
	}

	req := httptest.NewRequest(http.MethodGet, "/terminal/ws?instance=inst-miss&tenant_id=default", nil)
	w := httptest.NewRecorder()

	HandleWebSocket(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body=%q", w.Code, http.StatusBadGateway, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to resolve executor address") {
		t.Fatalf("expected resolver error in body, got %q", w.Body.String())
	}
}
