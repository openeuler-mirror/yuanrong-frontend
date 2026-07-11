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
	"context"
	"strings"
	"time"
)

// frontendProxyEndpointSource is the boundary between frontend runtime routing
// and the system component that aggregates proxy registrations. Phase 3's
// default production source will query function master; tests can inject a
// source without teaching route selection about etcd or registry internals.
type frontendProxyEndpointSource interface {
	ListFrontendProxyEndpoints(ctx context.Context) ([]FrontendProxyEndpoint, error)
}

type frontendProxyMasterProvider struct {
	source    frontendProxyEndpointSource
	discovery *memoryFrontendProxyDiscovery
}

func newFrontendProxyMasterProvider(source frontendProxyEndpointSource) *frontendProxyMasterProvider {
	return &frontendProxyMasterProvider{
		source:    source,
		discovery: newMemoryFrontendProxyDiscovery(),
	}
}

func (p *frontendProxyMasterProvider) Refresh(ctx context.Context) error {
	endpoints, err := p.source.ListFrontendProxyEndpoints(ctx)
	if err != nil {
		return err
	}
	p.discovery.ReplaceSnapshot(normalizeFrontendProxyEndpoints(endpoints))
	return nil
}

func (p *frontendProxyMasterProvider) GetByNode(nodeID string, capability string) (frontendProxyEndpoint, bool) {
	if p == nil || p.discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	return p.discovery.GetByNode(nodeID, capability)
}

func (p *frontendProxyMasterProvider) GetByHost(host string, capability string) (frontendProxyEndpoint, bool) {
	if p == nil || p.discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	return p.discovery.GetByHost(host, capability)
}

func (p *frontendProxyMasterProvider) GetSoleEndpoint(capability string) (frontendProxyEndpoint, bool) {
	if p == nil || p.discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	return p.discovery.GetSoleEndpoint(capability)
}

func (p *frontendProxyMasterProvider) GetNextEndpoint(capability string) (frontendProxyEndpoint, bool) {
	if p == nil || p.discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	return p.discovery.GetNextEndpoint(capability)
}

func (p *frontendProxyMasterProvider) MarkSuspectAddress(address string, ttl time.Duration) {
	if p == nil || p.discovery == nil {
		return
	}
	p.discovery.MarkSuspectAddress(address, ttl)
}

func (p *frontendProxyMasterProvider) IsSuspectAddress(address string) bool {
	if p == nil || p.discovery == nil {
		return false
	}
	return p.discovery.IsSuspectAddress(address)
}

func normalizeFrontendProxyEndpoints(endpoints []FrontendProxyEndpoint) []FrontendProxyEndpoint {
	filtered := make([]FrontendProxyEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.NodeID == "" || endpoint.Address == "" || len(endpoint.Capabilities) == 0 {
			continue
		}
		if endpoint.Health != "" && strings.ToLower(strings.TrimSpace(endpoint.Health)) != "healthy" {
			continue
		}
		filtered = append(filtered, endpoint)
	}
	return filtered
}
