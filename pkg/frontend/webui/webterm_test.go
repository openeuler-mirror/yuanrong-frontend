package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	if !strings.Contains(body, "fetch('/frontend/api/instances?tenant_id=' + encodeURIComponent(tenantId), fetchOptions);") {
		t.Fatalf("expected instances API URL to target sibling /api path under forwarded prefix")
	}
}
