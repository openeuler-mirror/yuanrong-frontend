package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
