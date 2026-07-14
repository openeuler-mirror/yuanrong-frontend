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

package route

import (
	"errors"
	"testing"
)

const applyKey = "/sn/instance/business/yrk/tenant/default/function/0-svc/version/$latest/defaultaz/req0/inst-abc"

const applyRunningJSON = `{"instanceID":"inst-abc","tenantID":"tenant-a","proxyGrpcAddress":"10.0.0.1:22772",` +
	`"instanceStatus":{"code":3},"extensions":{"portForward":"[\"tcp:31080:8765\"]"}}`

const applyFatalJSON = `{"instanceID":"inst-abc","proxyGrpcAddress":"10.0.0.1:22772",` +
	`"instanceStatus":{"code":6},"extensions":{}}`

func getRoute(c *RouteCache) (*RouteTarget, error) {
	return c.Get(RouteKey{SafeInstanceID: "inst-abc", Port: 8765})
}

func TestApplyPutRunningAdds(t *testing.T) {
	c := NewRouteCache()
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(applyRunningJSON))
	tgt, err := getRoute(c)
	if err != nil {
		t.Fatalf("route should exist after RUNNING put: %v", err)
	}
	if tgt.TargetURL.String() != "http://10.0.0.1:31080" {
		t.Errorf("target = %q, want http://10.0.0.1:31080", tgt.TargetURL)
	}
}

func TestApplyNonRunningRemoves(t *testing.T) {
	c := NewRouteCache()
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(applyRunningJSON))
	if _, err := getRoute(c); err != nil {
		t.Fatalf("precondition: route should exist: %v", err)
	}
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(applyFatalJSON))
	if _, err := getRoute(c); !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("route should be gone after Fatal, got %v", err)
	}
}

// DELETE carries no value; the instanceID is recovered from the key's last segment.
func TestApplyDeleteRemoves(t *testing.T) {
	c := NewRouteCache()
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(applyRunningJSON))
	if _, err := getRoute(c); err != nil {
		t.Fatalf("precondition: route should exist: %v", err)
	}
	ApplyInstanceEvent(c, EventDelete, applyKey, nil)
	if _, err := getRoute(c); !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("route should be gone after DELETE, got %v", err)
	}
}

func TestApplyMalformedJSONIgnored(t *testing.T) {
	c := NewRouteCache()
	ApplyInstanceEvent(c, EventPut, applyKey, []byte("not-json")) // must not panic
	if _, err := getRoute(c); !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("malformed put should add no route, got %v", err)
	}
}

func TestApplyRunningWithoutPortForwardAddsNothing(t *testing.T) {
	c := NewRouteCache()
	body := `{"instanceID":"inst-abc","tenantID":"tenant-a","proxyGrpcAddress":"10.0.0.1:22772","instanceStatus":{"code":3},"extensions":{}}`
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(body))
	if _, err := getRoute(c); !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("no portForward should yield no route, got %v", err)
	}
}

func TestInstanceIDFromKey(t *testing.T) {
	if got := instanceIDFromKey(applyKey); got != "inst-abc" {
		t.Errorf("instanceIDFromKey = %q, want inst-abc", got)
	}
	if got := instanceIDFromKey("noslash"); got != "noslash" {
		t.Errorf("instanceIDFromKey(noslash) = %q, want noslash", got)
	}
}

// A route without an instance owner cannot be authorized safely. The function
// tenant embedded in the key is not a substitute because shared system
// functions use a different tenant from the sandbox owner.
func TestApplyMissingTenantAddsNoRoute(t *testing.T) {
	c := NewRouteCache()
	body := `{"instanceID":"inst-abc","proxyGrpcAddress":"10.0.0.1:22772",` +
		`"instanceStatus":{"code":3},"extensions":{"portForward":"[\"tcp:31080:8765\"]"}}`
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(body))
	if _, err := getRoute(c); !errors.Is(err, ErrRouteNotFound) {
		t.Errorf("route without tenantID should not be registered, got %v", err)
	}
}

// Shared system functions use the function tenant (for example "default") in
// their etcd key, while tenantID in the instance payload identifies the actual
// sandbox owner. The instance owner must take precedence or legitimate SDK
// requests are rejected and the shared function tenant can be over-authorized.
func TestApplyPrefersInstanceTenantOverKeyTenant(t *testing.T) {
	c := NewRouteCache()
	body := `{"instanceID":"inst-abc","tenantID":"tenant-e2e-a",` +
		`"proxyGrpcAddress":"10.0.0.1:22772","instanceStatus":{"code":3},` +
		`"extensions":{"portForward":"[\"tcp:31080:8765\"]"}}`
	ApplyInstanceEvent(c, EventPut, applyKey, []byte(body))
	tgt, err := getRoute(c)
	if err != nil {
		t.Fatalf("route should exist: %v", err)
	}
	if tgt.Tenant != "tenant-e2e-a" {
		t.Errorf("target Tenant = %q, want tenant-e2e-a", tgt.Tenant)
	}
}
