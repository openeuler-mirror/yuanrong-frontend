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

package resolver

import (
	"context"
	"errors"
	"testing"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/sandboxrouter/route"
)

const instanceKey = "/sn/instance/business/yrk/tenant/default/function/0-svc/version/$latest/defaultaz/req0/inst-abc"

const runningJSON = `{"instanceID":"inst-abc","proxyGrpcAddress":"10.0.0.1:22772",` +
	`"instanceStatus":{"code":3},"extensions":{"portForward":"[\"tcp:31080:8765\"]"}}`

func resolve(r *InstanceInfoWatchResolver) (*route.RouteTarget, error) {
	return r.Resolve(context.Background(), route.RouteKey{SafeInstanceID: "inst-abc", Port: 8765})
}

// Verifies the etcd event-type mapping end to end: PUT(running) adds, DELETE removes.
func TestApplyEventPutThenDelete(t *testing.T) {
	r := NewInstanceInfoWatchResolver()

	r.applyEvent(&etcd3.Event{Type: etcd3.PUT, Key: instanceKey, Value: []byte(runningJSON)})
	if _, err := resolve(r); err != nil {
		t.Fatalf("route should resolve after PUT(running): %v", err)
	}

	r.applyEvent(&etcd3.Event{Type: etcd3.DELETE, Key: instanceKey})
	if _, err := resolve(r); !errors.Is(err, route.ErrRouteNotFound) {
		t.Errorf("route should be gone after DELETE, got %v", err)
	}
}

// SYNCED bypasses the filter and must be a no-op (not panic).
func TestApplyEventSyncedIgnored(t *testing.T) {
	r := NewInstanceInfoWatchResolver()
	r.applyEvent(&etcd3.Event{Type: etcd3.SYNCED})
	if _, err := resolve(r); !errors.Is(err, route.ErrRouteNotFound) {
		t.Errorf("SYNCED should add nothing, got %v", err)
	}
}

func TestSandboxInstanceFilter(t *testing.T) {
	// Full instance key (depth 14) is kept (filter returns false).
	if sandboxInstanceFilter(&etcd3.Event{Key: instanceKey}) {
		t.Error("full instance key should be kept (filter false)")
	}
	// Shallower keys are skipped (filter returns true).
	if !sandboxInstanceFilter(&etcd3.Event{Key: "/sn/instance/business/yrk"}) {
		t.Error("shallow key should be skipped (filter true)")
	}
}
