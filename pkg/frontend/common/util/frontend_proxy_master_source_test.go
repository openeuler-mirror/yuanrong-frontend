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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrontendProxyMasterHTTPSourceListsEndpointsFromFunctionMaster(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"count": 2,
			"endpoints": [
				{
					"nodeID": "proxy-node-a",
					"address": "10.0.0.11:19090",
					"capabilities": ["faas.create", "faas.invoke", "faas.kill"],
					"version": "phase3",
					"health": "healthy"
				},
				{
					"nodeID": "proxy-node-b",
					"address": "10.0.0.12:19090",
					"capabilities": ["faas.invoke"],
					"version": "phase3",
					"health": "healthy"
				}
			]
		}`))
	}))
	defer server.Close()

	source := newFrontendProxyMasterHTTPSource(func() string {
		return strings.TrimPrefix(server.URL, "http://")
	}, server.Client())

	endpoints, err := source.ListFrontendProxyEndpoints(context.Background())

	require.NoError(t, err)
	require.Equal(t, frontendProxyMasterEndpointPath, requestedPath)
	require.Len(t, endpoints, 2)
	require.Equal(t, "proxy-node-a", endpoints[0].NodeID)
	require.Equal(t, "10.0.0.11:19090", endpoints[0].Address)
	require.Equal(t, "phase3", endpoints[0].Version)
	require.True(t, endpoints[0].Capabilities[frontendProxyCapabilityCreate])
	require.True(t, endpoints[0].Capabilities[frontendProxyCapabilityInvoke])
	require.True(t, endpoints[0].Capabilities[frontendProxyCapabilityKill])
	require.Equal(t, "healthy", endpoints[0].Health)
	require.Equal(t, "proxy-node-b", endpoints[1].NodeID)
	require.Equal(t, "phase3", endpoints[1].Version)
	require.False(t, endpoints[1].Capabilities[frontendProxyCapabilityCreate])
	require.True(t, endpoints[1].Capabilities[frontendProxyCapabilityInvoke])
	require.Equal(t, "healthy", endpoints[1].Health)
}

func TestFrontendProxyMasterHTTPClientIgnoresEnvironmentProxy(t *testing.T) {
	client := newFrontendProxyMasterHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy)
}

func TestFrontendProxyMasterHTTPSourceFailsWhenMasterAddressUnavailable(t *testing.T) {
	source := newFrontendProxyMasterHTTPSource(func() string { return "" }, nil)

	_, err := source.ListFrontendProxyEndpoints(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "function master address is empty")
}
