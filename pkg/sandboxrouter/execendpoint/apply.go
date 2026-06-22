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

import (
	"encoding/json"
	"strings"
)

// kernelStatusRunning is constant.KernelInstanceStatusRunning (3). Only RUNNING
// instances are cached; any other state removes them. Kept as a local const so
// this package stays dependency-free (stdlib only) and unit-testable, mirroring
// route/apply.go.
const kernelStatusRunning int32 = 3

// EventKind is the kind of instance-info change, mirroring the etcd watch event
// types while keeping this package free of the etcd client dependency.
type EventKind int

const (
	// EventPut is an instance-info create/update.
	EventPut EventKind = iota
	// EventDelete is an instance-info removal.
	EventDelete
)

// instanceExecInfo is the minimal view of the /sn/instance JSON the exec path
// needs. The full kernel InstanceInfo proto (serialized via MessageToJsonString)
// carries far more; we only read these fields, matching the approach in
// route/instanceinfo.go. proxyGrpcAddress and containerID are NOT present in
// frontend's existing InstanceSpecification struct, which is why we parse our own.
type instanceExecInfo struct {
	InstanceID       string `json:"instanceID"`
	ProxyGrpcAddress string `json:"proxyGrpcAddress"`
	ContainerID      string `json:"containerID"`
	InstanceStatus   struct {
		Code int32 `json:"code"`
	} `json:"instanceStatus"`
}

// ApplyInstanceEvent updates the cache for one /sn/instance watch event:
//   - DELETE removes the instance's endpoint (instanceID recovered from the key).
//   - PUT of a RUNNING instance with a non-empty proxyGrpcAddress adds/replaces
//     its endpoint; any other state, missing proxyGrpcAddress, or unparseable
//     value removes it.
//
// It never panics on bad input; malformed data results in the instance having
// no cached endpoint, which the exec path treats as a miss (falling back to the
// master query).
func ApplyInstanceEvent(s *Store, kind EventKind, key string, value []byte) {
	if kind == EventDelete {
		s.Delete(instanceIDFromKey(key))
		return
	}

	var info instanceExecInfo
	if err := json.Unmarshal(value, &info); err != nil {
		// Can't identify the instance reliably; remove by key to be safe.
		s.Delete(instanceIDFromKey(key))
		return
	}
	id := info.InstanceID
	if id == "" {
		id = instanceIDFromKey(key)
	}

	if info.InstanceStatus.Code != kernelStatusRunning || info.ProxyGrpcAddress == "" {
		s.Delete(id)
		return
	}

	s.Put(Endpoint{
		InstanceID:       id,
		ProxyGrpcAddress: info.ProxyGrpcAddress,
		ContainerID:      info.ContainerID,
	})
}

// instanceIDFromKey returns the last '/'-separated segment of an instance key,
// which is the raw instanceID. A key with no '/' is returned unchanged. Mirrors
// route/apply.go:instanceIDFromKey.
func instanceIDFromKey(key string) string {
	if i := strings.LastIndexByte(key, '/'); i >= 0 {
		return key[i+1:]
	}
	return key
}
