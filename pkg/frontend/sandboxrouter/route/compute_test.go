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

const (
	expectedRouteCount         = 2
	expectedInternalRouteCount = 4
	realFormatSandboxPort      = 8765
)

func portForward(entries string) map[string]string {
	return map[string]string{"portForward": entries}
}

// TestComputeRoutesNewFormat: "protocol:hostPort:containerPort".
// route key uses containerPort (sandbox port); target uses hostPort (node port).
func TestComputeRoutesNewFormat(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "a@b",
		ProxyGrpcAddress: "10.0.1.23:22423",
		Extensions:       portForward(`["https:40001:8080"]`),
	}
	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d routes, want 1", len(got))
	}
	r := got[0]
	if r.Key != (Key{SafeInstanceID: "a-at-b", Port: 8080}) {
		t.Errorf("key = %+v, want {a-at-b,8080}", r.Key)
	}
	if r.Scheme != "https" || r.TargetURL.String() != "https://10.0.1.23:40001" {
		t.Errorf("target = %q scheme %q, want https://10.0.1.23:40001", r.TargetURL.String(), r.Scheme)
	}
}

// TestComputeRoutesRealFormat locks the production format produced by
// sandbox_executor.cpp: protocol is the lowercased transport ("tcp"), so the
// backend scheme is http (only "https" maps to an https backend).
func TestComputeRoutesRealFormat(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "ece63302-914c-4904-8200-00000000009e",
		ProxyGrpcAddress: "192.168.17.149:22772",
		Extensions:       portForward(`["tcp:31080:8765"]`),
	}
	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d routes, want 1", len(got))
	}
	r := got[0]
	if r.Key.Port != realFormatSandboxPort || r.Scheme != "http" ||
		r.TargetURL.String() != "http://192.168.17.149:31080" {
		t.Errorf("got %+v / %q, want http://192.168.17.149:31080 port 8765", r.Key, r.TargetURL)
	}
}

// TestComputeRoutesOldFormat: "hostPort:containerPort" defaults to http.
func TestComputeRoutesOldFormat(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23:22423",
		Extensions:       portForward(`["31080:8080"]`),
	}
	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d routes, want 1", len(got))
	}
	if got[0].Key.Port != 8080 || got[0].Scheme != "http" ||
		got[0].TargetURL.String() != "http://10.0.1.23:31080" {
		t.Errorf("got %+v / %q, want http://10.0.1.23:31080 port 8080", got[0].Key, got[0].TargetURL)
	}
}

func TestComputeRoutesMultiplePorts(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23:22423",
		Extensions:       portForward(`["https:40001:8080","31090:9090"]`),
	}
	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != expectedRouteCount {
		t.Fatalf("got %d routes, want %d", len(got), expectedRouteCount)
	}
}

func TestComputeRoutesRouteKindsRemainAvailableToInternalRouter(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23:22423",
		Extensions: portForward(
			`["direct+http:40001:50090","tunnel+http:40002:8765","direct+http:40003:8766","public+https:40004:8443"]`,
		),
	}

	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != expectedInternalRouteCount {
		t.Fatalf("got %d routes, want %d", len(got), expectedInternalRouteCount)
	}
	wantPorts := []uint16{50090, 8765, 8766, 8443}
	wantSchemes := []string{"http", "http", "http", "https"}
	for i := range got {
		if got[i].Key.Port != wantPorts[i] || got[i].Scheme != wantSchemes[i] {
			t.Errorf("route[%d] = port %d scheme %q, want port %d scheme %q",
				i, got[i].Key.Port, got[i].Scheme, wantPorts[i], wantSchemes[i])
		}
	}
}

func TestComputeRoutesNoPortForward(t *testing.T) {
	for _, ext := range []map[string]string{nil, {}, {"portForward": ""}, {"other": "x"}} {
		got, err := ComputeRoutes(&InstanceInfo{
			InstanceID:       "inst",
			ProxyGrpcAddress: "10.0.1.23:1",
			Extensions:       ext,
		})
		if err != nil || len(got) != 0 {
			t.Errorf("ext=%v: got %d routes err %v, want 0 routes nil err", ext, len(got), err)
		}
	}
}

func TestComputeRoutesMalformedJSON(t *testing.T) {
	_, err := ComputeRoutes(&InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23:1",
		Extensions:       portForward(`not-json`),
	})
	if err == nil {
		t.Error("expected error for malformed portForward JSON")
	}
}

// Malformed individual entries are skipped; valid ones still produce routes.
func TestComputeRoutesSkipsBadEntries(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23:22423",
		Extensions:       portForward(`["a:b:c:d","http:x:notnum","40001:8080"]`),
	}
	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Key.Port != 8080 {
		t.Errorf("got %d routes %+v, want 1 with port 8080", len(got), got)
	}
}

func TestComputeRoutesSkipsUnknownKindsAndUnsupportedSchemes(t *testing.T) {
	info := &InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23:22423",
		Extensions: portForward(
			`["direct+udp:40001:53","unknown+http:40002:80","http:40003:8080"]`,
		),
	}
	got, err := ComputeRoutes(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Key.Port != 8080 {
		t.Fatalf("got %+v, want only public HTTP port 8080", got)
	}
}

// No ':' in proxyGrpcAddress -> ExtractIP returns "" -> ErrInvalidTarget.
func TestComputeRoutesInvalidProxyAddr(t *testing.T) {
	_, err := ComputeRoutes(&InstanceInfo{
		InstanceID:       "inst",
		ProxyGrpcAddress: "10.0.1.23",
		Extensions:       portForward(`["40001:8080"]`),
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("got err %v, want ErrInvalidTarget", err)
	}
}
