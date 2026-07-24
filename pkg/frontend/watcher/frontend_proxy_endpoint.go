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

package watcher

import (
	"encoding/json"
	"strings"

	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/common/util"
)

const frontendProxyEndpointPrefix = "/yr/busproxy/business/yrk/tenant/0/node/"

type frontendProxyRegistration struct {
	Node         string               `json:"node"`
	ProxyService proxyServiceMetadata `json:"proxyService"`
}

type proxyServiceMetadata struct {
	GRPCAddress      string   `json:"grpcAddress"`
	TCPTunnelAddress string   `json:"tcpTunnelAddress"`
	Capabilities     []string `json:"capabilities"`
	Version          string   `json:"version"`
	Health           string   `json:"health"`
}

func startWatchFrontendProxyEndpoint(stopCh <-chan struct{}) {
	etcdClient := etcd3.GetRouterEtcdClient()
	watcher := etcd3.NewEtcdWatcher(frontendProxyEndpointPrefix, frontendProxyEndpointFilter,
		frontendProxyEndpointHandler, stopCh, etcdClient)
	watcher.StartWatch()
}

func frontendProxyEndpointFilter(event *etcd3.Event) bool {
	if event.Type == etcd3.SYNCED || event.Type == etcd3.ERROR {
		return false
	}
	return frontendProxyNodeID(event.Key) == ""
}

func frontendProxyEndpointHandler(event *etcd3.Event) {
	nodeID := frontendProxyNodeID(event.Key)
	switch event.Type {
	case etcd3.PUT, etcd3.HISTORYUPDATE:
		endpoint, ok := parseFrontendProxyEndpoint(nodeID, event.Value)
		if !ok {
			util.DeleteFrontendProxyEndpointAtRevision(nodeID, event.Rev)
			return
		}
		endpoint.Revision = event.Rev
		util.UpsertFrontendProxyEndpoint(endpoint)
	case etcd3.DELETE, etcd3.HISTORYDELETE:
		util.DeleteFrontendProxyEndpointAtRevision(nodeID, event.Rev)
	case etcd3.SYNCED:
		log.GetLogger().Infof("frontend proxy endpoint cache synced")
	case etcd3.ERROR:
		log.GetLogger().Warnf("frontend proxy endpoint watcher error: %s", event.Value)
	default:
		log.GetLogger().Warnf("unsupported frontend proxy endpoint event type: %d", event.Type)
	}
}

func parseFrontendProxyEndpoint(nodeID string, value []byte) (util.FrontendProxyEndpoint, bool) {
	var registration frontendProxyRegistration
	if err := json.Unmarshal(value, &registration); err != nil {
		log.GetLogger().Warnf("failed to parse frontend proxy registration for node %s: %s", nodeID, err)
		return util.FrontendProxyEndpoint{}, false
	}
	if registration.Node != "" {
		nodeID = registration.Node
	}
	service := registration.ProxyService
	if nodeID == "" || (service.GRPCAddress == "" && service.TCPTunnelAddress == "") ||
		!strings.EqualFold(service.Health, "healthy") {
		return util.FrontendProxyEndpoint{}, false
	}
	capabilities := make(map[string]bool, len(service.Capabilities))
	for _, capability := range service.Capabilities {
		if capability != "" {
			capabilities[capability] = true
		}
	}
	if len(capabilities) == 0 {
		return util.FrontendProxyEndpoint{}, false
	}
	return util.FrontendProxyEndpoint{
		NodeID: nodeID, Address: service.GRPCAddress, TCPTunnelAddress: service.TCPTunnelAddress,
		Capabilities: capabilities, Version: service.Version, Health: service.Health,
	}, true
}

func frontendProxyNodeID(key string) string {
	if !strings.HasPrefix(key, frontendProxyEndpointPrefix) {
		return ""
	}
	nodeID := strings.TrimPrefix(key, frontendProxyEndpointPrefix)
	if nodeID == "" || strings.Contains(nodeID, "/") {
		return ""
	}
	return nodeID
}
