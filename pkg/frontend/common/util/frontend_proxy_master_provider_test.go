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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeFrontendProxyEndpointSource struct {
	endpoints []FrontendProxyEndpoint
	err       error
	calls     int
}

func (f *fakeFrontendProxyEndpointSource) ListFrontendProxyEndpoints(
	ctx context.Context,
) ([]FrontendProxyEndpoint, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.endpoints, nil
}

func TestFrontendProxyMasterProviderRefreshesSnapshotFromMaster(t *testing.T) {
	source := &fakeFrontendProxyEndpointSource{endpoints: []FrontendProxyEndpoint{
		{NodeID: "node-a", Address: "10.0.0.1:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
		{NodeID: "node-b", Address: "10.0.0.2:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
	}}
	provider := newFrontendProxyMasterProvider(source)

	require.NoError(t, provider.Refresh(context.Background()))

	first, ok := provider.GetNextEndpoint(frontendProxyCapabilityCreate)
	require.True(t, ok)
	second, ok := provider.GetNextEndpoint(frontendProxyCapabilityCreate)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"node-a", "node-b"}, []string{first.NodeID, second.NodeID})
	require.Equal(t, 1, source.calls)
}

func TestFrontendProxyMasterProviderFiltersInvalidEndpointRecords(t *testing.T) {
	source := &fakeFrontendProxyEndpointSource{endpoints: []FrontendProxyEndpoint{
		{NodeID: "", Address: "10.0.0.1:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
		{NodeID: "node-missing-address", Address: "", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
		{NodeID: "node-missing-capability", Address: "10.0.0.3:19090"},
		{NodeID: "node-ok", Address: "10.0.0.4:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
	}}
	provider := newFrontendProxyMasterProvider(source)

	require.NoError(t, provider.Refresh(context.Background()))

	endpoint, ok := provider.GetSoleEndpoint(frontendProxyCapabilityCreate)
	require.True(t, ok)
	require.Equal(t, "node-ok", endpoint.NodeID)
}

func TestFrontendProxyMasterProviderFiltersUnhealthyEndpointRecords(t *testing.T) {
	source := &fakeFrontendProxyEndpointSource{endpoints: []FrontendProxyEndpoint{
		{
			NodeID:       "node-unhealthy",
			Address:      "10.0.0.10:19090",
			Capabilities: map[string]bool{frontendProxyCapabilityCreate: true},
			Health:       "unhealthy",
		},
		{
			NodeID:       "node-healthy",
			Address:      "10.0.0.11:19090",
			Capabilities: map[string]bool{frontendProxyCapabilityCreate: true},
			Health:       "healthy",
		},
	}}
	provider := newFrontendProxyMasterProvider(source)

	require.NoError(t, provider.Refresh(context.Background()))

	endpoint, ok := provider.GetSoleEndpoint(frontendProxyCapabilityCreate)
	require.True(t, ok)
	require.Equal(t, "node-healthy", endpoint.NodeID)
}

func TestFrontendProxyMasterProviderKeepsLastSnapshotOnRefreshError(t *testing.T) {
	source := &fakeFrontendProxyEndpointSource{endpoints: []FrontendProxyEndpoint{
		{NodeID: "node-a", Address: "10.0.0.1:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
	}}
	provider := newFrontendProxyMasterProvider(source)
	require.NoError(t, provider.Refresh(context.Background()))

	source.err = errors.New("function master temporarily unavailable")
	require.Error(t, provider.Refresh(context.Background()))

	endpoint, ok := provider.GetSoleEndpoint(frontendProxyCapabilityCreate)
	require.True(t, ok)
	require.Equal(t, "node-a", endpoint.NodeID)
}

func TestFrontendProxyMasterProviderMarksSuspectEndpoint(t *testing.T) {
	source := &fakeFrontendProxyEndpointSource{endpoints: []FrontendProxyEndpoint{
		{NodeID: "node-a", Address: "10.0.0.1:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
		{NodeID: "node-b", Address: "10.0.0.2:19090", Capabilities: map[string]bool{frontendProxyCapabilityCreate: true}},
	}}
	provider := newFrontendProxyMasterProvider(source)
	require.NoError(t, provider.Refresh(context.Background()))

	provider.MarkSuspectAddress("10.0.0.1:19090", time.Minute)

	_, ok := provider.GetByNode("node-a", frontendProxyCapabilityCreate)
	require.False(t, ok)
	endpoint, ok := provider.GetSoleEndpoint(frontendProxyCapabilityCreate)
	require.True(t, ok)
	require.Equal(t, "node-b", endpoint.NodeID)
}
