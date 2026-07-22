/*
 * Copyright (c) Huawei Technologies Co., Ltd. 2026. All rights reserved.
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

package util

import (
	"net"
	"sync"
	"time"
)

const (
	frontendProxyCapabilityInvoke  = "faas.invoke"
	frontendProxyCapabilityCreate  = "faas.create"
	frontendProxyCapabilityKill    = "faas.kill"
	proxyCapabilityTCPTunnel       = "tcp.tunnel"
	defaultFrontendProxySuspectTTL = 30 * time.Second
)

type frontendProxyEndpoint struct {
	NodeID           string
	Address          string
	TCPTunnelAddress string
	Version          string
	Capabilities     map[string]bool
	Health           string
	Revision         int64
}

// FrontendProxyEndpoint is the discovery record consumed by the Go-native
// frontend->proxy invoke path. It intentionally carries only routing metadata;
// request auth, tenant, and trace context still come from the invoke request.
type FrontendProxyEndpoint = frontendProxyEndpoint

type frontendProxyDiscovery interface {
	GetByNode(nodeID string, capability string) (frontendProxyEndpoint, bool)
	GetSoleEndpoint(capability string) (frontendProxyEndpoint, bool)
	GetNextEndpoint(capability string) (frontendProxyEndpoint, bool)
}

type frontendProxySuspectMarker interface {
	MarkSuspectAddress(address string, ttl time.Duration)
}

type frontendProxySuspectChecker interface {
	IsSuspectAddress(address string) bool
}

type frontendProxyDiscoveryByHost interface {
	GetByHost(host string, capability string) (frontendProxyEndpoint, bool)
}

var defaultFrontendProxyDiscovery = newMemoryFrontendProxyDiscovery()

var frontendProxyDiscoveryState = struct {
	sync.RWMutex
	discovery frontendProxyDiscovery
}{discovery: defaultFrontendProxyDiscovery}

func currentFrontendProxyDiscovery() frontendProxyDiscovery {
	frontendProxyDiscoveryState.RLock()
	defer frontendProxyDiscoveryState.RUnlock()
	return frontendProxyDiscoveryState.discovery
}

func setFrontendProxyDiscovery(discovery frontendProxyDiscovery) {
	frontendProxyDiscoveryState.Lock()
	defer frontendProxyDiscoveryState.Unlock()
	frontendProxyDiscoveryState.discovery = discovery
}

func resetFrontendProxyDiscovery() {
	defaultFrontendProxyDiscovery.ReplaceSnapshot(nil)
	setFrontendProxyDiscovery(defaultFrontendProxyDiscovery)
}

func setFrontendProxyDiscoveryForTest(discovery frontendProxyDiscovery) func() {
	frontendProxyDiscoveryState.Lock()
	old := frontendProxyDiscoveryState.discovery
	frontendProxyDiscoveryState.discovery = discovery
	frontendProxyDiscoveryState.Unlock()

	return func() {
		frontendProxyDiscoveryState.Lock()
		defer frontendProxyDiscoveryState.Unlock()
		frontendProxyDiscoveryState.discovery = old
	}
}

// ReplaceFrontendProxyDiscoverySnapshot atomically replaces the process-wide discovery snapshot.
func ReplaceFrontendProxyDiscoverySnapshot(endpoints []FrontendProxyEndpoint) {
	defaultFrontendProxyDiscovery.ReplaceSnapshot(endpoints)
	setFrontendProxyDiscovery(defaultFrontendProxyDiscovery)
}

// UseFrontendProxyDiscoveryCache selects the etcd-backed process-wide endpoint cache.
func UseFrontendProxyDiscoveryCache() {
	setFrontendProxyDiscovery(defaultFrontendProxyDiscovery)
}

// UpsertFrontendProxyEndpoint applies an etcd registration update to the process-wide discovery index.
func UpsertFrontendProxyEndpoint(endpoint FrontendProxyEndpoint) {
	defaultFrontendProxyDiscovery.Upsert(endpoint)
}

// DeleteFrontendProxyEndpointAtRevision removes an endpoint unless a newer event has already been applied.
func DeleteFrontendProxyEndpointAtRevision(nodeID string, revision int64) {
	defaultFrontendProxyDiscovery.Delete(nodeID, revision)
}

// LookupFrontendProxyEndpoint resolves a healthy endpoint by owner node and capability.
func LookupFrontendProxyEndpoint(nodeID string, capability string) (FrontendProxyEndpoint, bool) {
	return resolveFrontendProxyEndpointByNode(nodeID, capability)
}

// LookupProxyTCPTunnelAddress resolves the published TCP tunnel endpoint for an owning proxy.
func LookupProxyTCPTunnelAddress(nodeID string) (string, bool) {
	discovery := currentFrontendProxyDiscovery()
	if discovery == nil {
		return "", false
	}
	endpoint, ok := discovery.GetByNode(nodeID, proxyCapabilityTCPTunnel)
	if !ok || !isRoutableProxyAddress(endpoint.TCPTunnelAddress) {
		return "", false
	}
	return endpoint.TCPTunnelAddress, true
}

// MarkFrontendProxyEndpointSuspect temporarily removes an unhealthy address from candidate selection.
func MarkFrontendProxyEndpointSuspect(address string) {
	if address == "" {
		return
	}
	marker, ok := currentFrontendProxyDiscovery().(frontendProxySuspectMarker)
	if !ok {
		return
	}
	marker.MarkSuspectAddress(address, defaultFrontendProxySuspectTTL)
}

func frontendProxyAddressIsSuspect(address string) bool {
	if address == "" {
		return false
	}
	checker, ok := currentFrontendProxyDiscovery().(frontendProxySuspectChecker)
	if !ok {
		return false
	}
	return checker.IsSuspectAddress(address)
}

type memoryFrontendProxyDiscovery struct {
	mu           sync.RWMutex
	endpoints    map[string]frontendProxyEndpoint
	order        []string
	next         uint64
	suspectUntil map[string]time.Time
	revisions    map[string]int64
}

func endpointAddressForCapability(endpoint frontendProxyEndpoint, capability string) string {
	if capability == proxyCapabilityTCPTunnel {
		return endpoint.TCPTunnelAddress
	}
	return endpoint.Address
}

func isRoutableProxyAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsUnspecified()
}

func newMemoryFrontendProxyDiscovery() *memoryFrontendProxyDiscovery {
	return &memoryFrontendProxyDiscovery{
		endpoints:    make(map[string]frontendProxyEndpoint),
		suspectUntil: make(map[string]time.Time),
		revisions:    make(map[string]int64),
	}
}

func (d *memoryFrontendProxyDiscovery) ReplaceSnapshot(endpoints []frontendProxyEndpoint) {
	d.mu.Lock()
	defer d.mu.Unlock()

	next := make(map[string]frontendProxyEndpoint, len(endpoints))
	addresses := make(map[string]struct{}, len(endpoints))
	order := make([]string, 0, len(endpoints))
	revisions := make(map[string]int64, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.NodeID == "" || (endpoint.Address == "" && endpoint.TCPTunnelAddress == "") {
			continue
		}
		if _, exists := next[endpoint.NodeID]; !exists {
			order = append(order, endpoint.NodeID)
		}
		next[endpoint.NodeID] = endpoint
		addresses[endpoint.Address] = struct{}{}
		addresses[endpoint.TCPTunnelAddress] = struct{}{}
		revisions[endpoint.NodeID] = endpoint.Revision
	}
	d.endpoints = next
	d.order = order
	d.next = 0
	d.revisions = revisions
	for address := range d.suspectUntil {
		if _, ok := addresses[address]; !ok {
			delete(d.suspectUntil, address)
		}
	}
}

func (d *memoryFrontendProxyDiscovery) Upsert(endpoint frontendProxyEndpoint) {
	if d == nil || endpoint.NodeID == "" || (endpoint.Address == "" && endpoint.TCPTunnelAddress == "") {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if current := d.revisions[endpoint.NodeID]; endpoint.Revision > 0 && current > endpoint.Revision {
		return
	}
	if _, exists := d.endpoints[endpoint.NodeID]; !exists {
		d.order = append(d.order, endpoint.NodeID)
	}
	d.endpoints[endpoint.NodeID] = endpoint
	if endpoint.Revision > 0 {
		d.revisions[endpoint.NodeID] = endpoint.Revision
	}
}

func (d *memoryFrontendProxyDiscovery) Delete(nodeID string, revision int64) {
	if d == nil || nodeID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if current := d.revisions[nodeID]; revision > 0 && current > revision {
		return
	}
	if revision > 0 {
		d.revisions[nodeID] = revision
	}
	endpoint, exists := d.endpoints[nodeID]
	if !exists {
		return
	}
	delete(d.endpoints, nodeID)
	delete(d.suspectUntil, endpoint.Address)
	delete(d.suspectUntil, endpoint.TCPTunnelAddress)
	for i, current := range d.order {
		if current == nodeID {
			d.order = append(d.order[:i], d.order[i+1:]...)
			break
		}
	}
	if len(d.order) == 0 {
		d.next = 0
	} else {
		d.next %= uint64(len(d.order))
	}
}

func (d *memoryFrontendProxyDiscovery) GetSoleEndpoint(capability string) (frontendProxyEndpoint, bool) {
	if d == nil {
		return frontendProxyEndpoint{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var sole frontendProxyEndpoint
	count := 0
	for _, endpoint := range d.endpoints {
		if capability != "" && (endpoint.Capabilities == nil || !endpoint.Capabilities[capability]) {
			continue
		}
		if d.isSuspectAddressLocked(endpointAddressForCapability(endpoint, capability), now) {
			continue
		}
		sole = endpoint
		count++
		if count > 1 {
			return frontendProxyEndpoint{}, false
		}
	}
	if count != 1 {
		return frontendProxyEndpoint{}, false
	}
	return sole, true
}

func (d *memoryFrontendProxyDiscovery) GetNextEndpoint(capability string) (frontendProxyEndpoint, bool) {
	if d == nil {
		return frontendProxyEndpoint{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.order) == 0 {
		return frontendProxyEndpoint{}, false
	}
	now := time.Now()
	for i := 0; i < len(d.order); i++ {
		idx := int(d.next % uint64(len(d.order)))
		d.next++
		endpoint, ok := d.endpoints[d.order[idx]]
		if !ok {
			continue
		}
		if capability != "" && (endpoint.Capabilities == nil || !endpoint.Capabilities[capability]) {
			continue
		}
		if d.isSuspectAddressLocked(endpointAddressForCapability(endpoint, capability), now) {
			continue
		}
		return endpoint, true
	}
	return frontendProxyEndpoint{}, false
}

func (d *memoryFrontendProxyDiscovery) GetByNode(nodeID string, capability string) (frontendProxyEndpoint, bool) {
	if d == nil || nodeID == "" {
		return frontendProxyEndpoint{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	endpoint, ok := d.endpoints[nodeID]
	if !ok {
		return frontendProxyEndpoint{}, false
	}
	if d.isSuspectAddressLocked(endpointAddressForCapability(endpoint, capability), time.Now()) {
		return frontendProxyEndpoint{}, false
	}
	if capability == "" {
		return endpoint, true
	}
	if endpoint.Capabilities == nil || !endpoint.Capabilities[capability] {
		return frontendProxyEndpoint{}, false
	}
	return endpoint, true
}

func (d *memoryFrontendProxyDiscovery) GetByHost(host string, capability string) (frontendProxyEndpoint, bool) {
	if d == nil || host == "" {
		return frontendProxyEndpoint{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var matched frontendProxyEndpoint
	count := 0
	for _, endpoint := range d.endpoints {
		endpointHost, _, err := net.SplitHostPort(endpointAddressForCapability(endpoint, capability))
		if err != nil || endpointHost != host {
			continue
		}
		if capability != "" && (endpoint.Capabilities == nil || !endpoint.Capabilities[capability]) {
			continue
		}
		if d.isSuspectAddressLocked(endpointAddressForCapability(endpoint, capability), now) {
			continue
		}
		matched = endpoint
		count++
		if count > 1 {
			return frontendProxyEndpoint{}, false
		}
	}
	return matched, count == 1
}

func (d *memoryFrontendProxyDiscovery) MarkSuspectAddress(address string, ttl time.Duration) {
	if d == nil || address == "" || ttl <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.suspectUntil == nil {
		d.suspectUntil = make(map[string]time.Time)
	}
	d.suspectUntil[address] = time.Now().Add(ttl)
}

func (d *memoryFrontendProxyDiscovery) IsSuspectAddress(address string) bool {
	if d == nil || address == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.isSuspectAddressLocked(address, time.Now())
}

func (d *memoryFrontendProxyDiscovery) isSuspectAddressLocked(address string, now time.Time) bool {
	if d == nil || address == "" || len(d.suspectUntil) == 0 {
		return false
	}
	until, ok := d.suspectUntil[address]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(d.suspectUntil, address)
	return false
}
