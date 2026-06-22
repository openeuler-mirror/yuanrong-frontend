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

package execendpoint

import "testing"

const instanceKey = "/sn/instance/business/yrk/tenant/default/function/0-svc/version/$latest/defaultaz/req0/inst-abc"

// Shape mirrors a real /sn/instance value (MessageToJsonString output) reduced
// to the fields the exec path reads.
const runningJSON = `{"instanceID":"inst-abc","proxyGrpcAddress":"10.0.0.1:22774",` +
	`"containerID":"sbox-4fb6aa1c","instanceStatus":{"code":3},` +
	`"extensions":{"portForward":"[\"tcp:31080:8765\"]"}}`

func TestApplyEventPutRunning(t *testing.T) {
	s := NewStore()
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(runningJSON))

	got, ok := s.Get("inst-abc")
	if !ok {
		t.Fatal("RUNNING instance should be cached after PUT")
	}
	if got.ProxyGrpcAddress != "10.0.0.1:22774" {
		t.Errorf("ProxyGrpcAddress = %q, want 10.0.0.1:22774", got.ProxyGrpcAddress)
	}
	if got.ContainerID != "sbox-4fb6aa1c" {
		t.Errorf("ContainerID = %q, want sbox-4fb6aa1c", got.ContainerID)
	}
}

func TestApplyEventPutThenDelete(t *testing.T) {
	s := NewStore()
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(runningJSON))
	if _, ok := s.Get("inst-abc"); !ok {
		t.Fatal("should be cached after PUT")
	}
	ApplyInstanceEvent(s, EventDelete, instanceKey, nil)
	if _, ok := s.Get("inst-abc"); ok {
		t.Error("should be gone after DELETE")
	}
}

func TestApplyEventNonRunningRemoved(t *testing.T) {
	s := NewStore()
	// Pre-populate, then a non-RUNNING update (code 5) must evict it.
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(runningJSON))
	const exitedJSON = `{"instanceID":"inst-abc","proxyGrpcAddress":"10.0.0.1:22774","instanceStatus":{"code":5}}`
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(exitedJSON))
	if _, ok := s.Get("inst-abc"); ok {
		t.Error("non-RUNNING instance must not stay cached")
	}
}

func TestApplyEventEmptyProxyRemoved(t *testing.T) {
	s := NewStore()
	const noProxyJSON = `{"instanceID":"inst-abc","proxyGrpcAddress":"","instanceStatus":{"code":3}}`
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(noProxyJSON))
	if _, ok := s.Get("inst-abc"); ok {
		t.Error("RUNNING instance with empty proxyGrpcAddress must not be cached")
	}
}

func TestApplyEventMalformedJSONRemoved(t *testing.T) {
	s := NewStore()
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(runningJSON))
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte("{not json"))
	if _, ok := s.Get("inst-abc"); ok {
		t.Error("malformed PUT must evict by key, not keep stale entry")
	}
}

func TestApplyEventInstanceIDFromKeyFallback(t *testing.T) {
	s := NewStore()
	// Value omits instanceID; it must be recovered from the key's last segment.
	const noIDJSON = `{"proxyGrpcAddress":"10.0.0.1:22774","containerID":"sbox-x","instanceStatus":{"code":3}}`
	ApplyInstanceEvent(s, EventPut, instanceKey, []byte(noIDJSON))
	if _, ok := s.Get("inst-abc"); !ok {
		t.Error("instanceID should fall back to the key's last segment")
	}
}
