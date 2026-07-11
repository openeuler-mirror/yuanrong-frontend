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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const frontendProxyMasterEndpointPath = "/instance-manager/frontend-proxy-endpoints"

type frontendProxyMasterHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type frontendProxyMasterHTTPSource struct {
	masterAddr func() string
	client     frontendProxyMasterHTTPDoer
}

type frontendProxyMasterEndpointResponse struct {
	Endpoints []frontendProxyMasterEndpointRecord `json:"endpoints"`
}

type frontendProxyMasterEndpointRecord struct {
	NodeID       string   `json:"nodeID"`
	Address      string   `json:"address"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Health       string   `json:"health"`
}

func newFrontendProxyMasterHTTPSource(masterAddr func() string, client frontendProxyMasterHTTPDoer) *frontendProxyMasterHTTPSource {
	if client == nil {
		client = newFrontendProxyMasterHTTPClient()
	}
	return &frontendProxyMasterHTTPSource{
		masterAddr: masterAddr,
		client:     client,
	}
}

func newFrontendProxyMasterHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Function master discovery is an internal component-to-component query.
	// Runtime/VM environments can inherit http_proxy for external downloads;
	// using those proxies for cluster-local master addresses produces proxy
	// 502s and breaks discovery.  Disable proxy lookup for this client only.
	transport.Proxy = nil
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
}

func (s *frontendProxyMasterHTTPSource) ListFrontendProxyEndpoints(ctx context.Context) ([]FrontendProxyEndpoint, error) {
	if s == nil || s.masterAddr == nil {
		return nil, fmt.Errorf("function master address is empty")
	}
	masterAddr := strings.TrimSpace(s.masterAddr())
	if masterAddr == "" {
		return nil, fmt.Errorf("function master address is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, frontendProxyMasterURL(masterAddr), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create frontend proxy endpoint request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query frontend proxy endpoints from function master: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("function master returned status %d for frontend proxy endpoints: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded frontendProxyMasterEndpointResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode frontend proxy endpoint response: %w", err)
	}
	endpoints := make([]FrontendProxyEndpoint, 0, len(decoded.Endpoints))
	for _, record := range decoded.Endpoints {
		endpoints = append(endpoints, FrontendProxyEndpoint{
			NodeID:       record.NodeID,
			Address:      record.Address,
			Version:      strings.TrimSpace(record.Version),
			Capabilities: frontendProxyCapabilitySet(record.Capabilities),
			Health:       strings.TrimSpace(record.Health),
		})
	}
	return endpoints, nil
}

func frontendProxyMasterURL(masterAddr string) string {
	if strings.HasPrefix(masterAddr, "http://") || strings.HasPrefix(masterAddr, "https://") {
		return strings.TrimRight(masterAddr, "/") + frontendProxyMasterEndpointPath
	}
	return "http://" + strings.TrimRight(masterAddr, "/") + frontendProxyMasterEndpointPath
}

func frontendProxyCapabilitySet(capabilities []string) map[string]bool {
	if len(capabilities) == 0 {
		return nil
	}
	out := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability != "" {
			out[capability] = true
		}
	}
	return out
}
