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
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/constants"
	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/grpc/pb/common"
	"frontend/pkg/common/faas_common/grpc/pb/core"
	"frontend/pkg/common/faas_common/grpc/pb/frontend_proxy"
	commontypes "frontend/pkg/common/faas_common/types"
	"frontend/pkg/frontend/config"
	"frontend/pkg/frontend/instancemanager"
)

var frontendProxyRequestSeq atomic.Uint64

const (
	frontendProxyRouteKey                             = "YR_ROUTE"
	frontendProxyCreateSourceKey                      = "source"
	frontendProxyCreateSource                         = "frontend"
	frontendProxyControlNotWired                      = "control-path-not-wired"
	defaultFrontendProxyTimeout                       = 60 * time.Second
	createReadyCallResultFieldNumber protowire.Number = 4
)

type grpcFrontendProxyInvokeClient struct {
	client           frontend_proxy.FrontendProxyServiceClient
	frontendClientID string
}

type grpcFrontendProxyLifecycleClient struct {
	client           frontend_proxy.FrontendProxyServiceClient
	frontendClientID string
}

func newGRPCFrontendProxyInvokeClient(client frontend_proxy.FrontendProxyServiceClient,
	frontendClientID string,
) frontendProxyInvokeClient {
	return &grpcFrontendProxyInvokeClient{
		client:           client,
		frontendClientID: frontendClientID,
	}
}

func newGRPCFrontendProxyLifecycleClient(client frontend_proxy.FrontendProxyServiceClient,
	frontendClientID string,
) frontendProxyLifecycleClient {
	return &grpcFrontendProxyLifecycleClient{
		client:           client,
		frontendClientID: frontendClientID,
	}
}

func (c *grpcFrontendProxyInvokeClient) InvokeByInstanceID(req simpleRuntimeInvokeRequest) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("frontend proxy grpc client is nil")
	}
	requestID := fmt.Sprintf("frontend-proxy-%d", frontendProxyRequestSeq.Add(1))
	ctx, cancel := simpleRuntimeInvokeContext(req.options)
	defer cancel()
	resp, err := c.client.InvokeInstance(ctx, &frontend_proxy.InvokeInstanceRequest{
		Context: &frontend_proxy.FrontendRequestContext{
			FrontendClientID: c.frontendClientID,
			TenantID:         firstArgTenantID(req.args),
			RequestID:        requestID,
			TraceID:          req.options.TraceID,
		},
		Invoke: &core.InvokeRequest{
			Function:      req.funcMeta.FuncID,
			Args:          convertSimpleRuntimeArgs(req.args),
			InstanceID:    req.instanceID,
			RequestID:     requestID,
			TraceID:       req.options.TraceID,
			InvokeOptions: convertSimpleRuntimeInvokeOptions(req.options),
		},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("frontend proxy invoke response is nil")
	}
	if err := checkFrontendProxyStatus("invoke", resp.GetStatus()); err != nil {
		return nil, err
	}
	callResult := resp.GetCallResult()
	if callResult == nil {
		return nil, fmt.Errorf("frontend proxy invoke missing call result")
	}
	if callResult.GetCode() != common.ErrorCode_ERR_NONE {
		return nil, frontendProxyBusinessError("invoke call result", callResult.GetCode(), callResult.GetMessage())
	}
	smallObjects := callResult.GetSmallObjects()
	if len(smallObjects) == 0 {
		return nil, fmt.Errorf("frontend proxy invoke call result has no small object payload")
	}
	return smallObjects[0].GetValue(), nil
}

func (c *grpcFrontendProxyLifecycleClient) CreateInstance(req simpleRuntimeCreateRequest) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("frontend proxy grpc client is nil")
	}
	requestID := fmt.Sprintf("frontend-proxy-create-%d", frontendProxyRequestSeq.Add(1))
	ctx, cancel := simpleRuntimeInvokeContext(req.options)
	defer cancel()
	resp, err := c.client.CreateInstance(ctx, &frontend_proxy.CreateInstanceRequest{
		Context: &frontend_proxy.FrontendRequestContext{
			FrontendClientID: c.frontendClientID,
			TenantID:         firstNonEmpty(firstArgTenantID(req.args), req.tenantID),
			RequestID:        requestID,
			TraceID:          req.options.TraceID,
		},
		Create: &core.CreateRequest{
			Function:      req.funcMeta.FuncID,
			Args:          convertSimpleRuntimeArgs(req.args),
			RequestID:     requestID,
			TraceID:       req.options.TraceID,
			CreateOptions: convertSimpleRuntimeCreateOptions(req.options),
		},
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("frontend proxy create response is nil")
	}
	if err := checkFrontendProxyStatus("create", resp.GetStatus()); err != nil {
		return "", err
	}
	createResp := resp.GetCreate()
	if createResp == nil {
		return "", fmt.Errorf("frontend proxy create missing create response")
	}
	if createResp.GetCode() != common.ErrorCode_ERR_NONE {
		return "", frontendProxyBusinessError("create", createResp.GetCode(), createResp.GetMessage())
	}
	instanceID := createResp.GetInstanceID()
	if instanceID == "" {
		return "", fmt.Errorf("frontend proxy create response missing instance id")
	}
	if routeAddress := resp.GetRouteAddress(); routeAddress != "" {
		instancemanager.RecordRouteOnlyInstance(req.funcMeta.FuncID, instanceID, routeAddress)
	}
	return instanceID, nil
}

func (c *grpcFrontendProxyLifecycleClient) CreateInstanceRaw(req simpleRuntimeRawCreateRequest) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("frontend proxy grpc client is nil")
	}
	createReq := &core.CreateRequest{}
	if err := proto.Unmarshal(req.create, createReq); err != nil {
		return nil, fmt.Errorf("failed to unmarshal frontend proxy create request: %w", err)
	}
	requestID := firstNonEmpty(createReq.GetRequestID(), fmt.Sprintf("frontend-proxy-create-%d", frontendProxyRequestSeq.Add(1)))
	if createReq.RequestID == "" {
		createReq.RequestID = requestID
	}
	applyRawRequestOptionsToCreate(createReq, req.options)
	ctx, cancel := rawSimpleRuntimeContext(req.options)
	defer cancel()
	resp, err := c.client.CreateInstance(ctx, &frontend_proxy.CreateInstanceRequest{
		Context: rawFrontendRequestContext(
			c.frontendClientID,
			requestID,
			createReq.GetTraceID(),
			req.options,
			tenantIDFromCreateRequest(createReq),
		),
		Create: createReq,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("frontend proxy create response is nil")
	}
	if err := checkFrontendProxyStatus("create", resp.GetStatus()); err != nil {
		return nil, err
	}
	callResult, err := createReadyCallResultFromResponse(resp)
	if err != nil {
		return nil, err
	}
	if callResult == nil {
		if createResp := resp.GetCreate(); createResp != nil && createResp.GetCode() != common.ErrorCode_ERR_NONE {
			return nil, frontendProxyBusinessError("create", createResp.GetCode(), createResp.GetMessage())
		}
		return nil, fmt.Errorf("frontend proxy create missing ready call result")
	}
	if createResp := resp.GetCreate(); createResp != nil {
		instanceID := firstNonEmpty(createResp.GetInstanceID(), callResult.GetInstanceID())
		if routeAddress := resp.GetRouteAddress(); routeAddress != "" && instanceID != "" {
			instancemanager.RecordRouteOnlyInstance(createReq.GetFunction(), instanceID, routeAddress)
		}
	}
	return marshalRuntimeNotifyFromCallResult(callResult)
}

func (c *grpcFrontendProxyLifecycleClient) KillInstance(req simpleRuntimeKillRequest) error {
	if c.client == nil {
		return fmt.Errorf("frontend proxy grpc client is nil")
	}
	requestID := firstNonEmpty(req.requestID, fmt.Sprintf("frontend-proxy-kill-%d", frontendProxyRequestSeq.Add(1)))
	ctx, cancel := simpleRuntimeInvokeContext(req.options)
	defer cancel()
	resp, err := c.client.KillInstance(ctx, &frontend_proxy.KillInstanceRequest{
		Context: frontendRequestContextFromInvokeOptions(c.frontendClientID, req.tenantID, requestID, req.options),
		Kill: &core.KillRequest{
			InstanceID: req.instanceID,
			Signal:     int32(req.signal),
			Payload:    req.payload,
			RequestID:  requestID,
		},
	})
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("frontend proxy kill response is nil")
	}
	if err := checkFrontendProxyStatus("kill", resp.GetStatus()); err != nil {
		return err
	}
	killResp := resp.GetKill()
	if killResp == nil {
		return fmt.Errorf("frontend proxy kill missing kill response")
	}
	if killResp.GetCode() != common.ErrorCode_ERR_NONE {
		return frontendProxyBusinessError("kill", killResp.GetCode(), killResp.GetMessage())
	}
	instancemanager.RemoveRouteOnlyInstance(req.instanceID)
	return nil
}

func (c *grpcFrontendProxyInvokeClient) InvokeByInstanceIDRaw(req simpleRuntimeRawInvokeRequest) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("frontend proxy grpc client is nil")
	}
	invokeReq := &core.InvokeRequest{}
	if err := proto.Unmarshal(req.invoke, invokeReq); err != nil {
		return nil, fmt.Errorf("failed to unmarshal frontend proxy invoke request: %w", err)
	}
	requestID := firstNonEmpty(invokeReq.GetRequestID(), fmt.Sprintf("frontend-proxy-invoke-%d", frontendProxyRequestSeq.Add(1)))
	if invokeReq.RequestID == "" {
		invokeReq.RequestID = requestID
	}
	applyRawRequestOptionsToInvoke(invokeReq, req.options)
	ctx, cancel := rawSimpleRuntimeContext(req.options)
	defer cancel()
	resp, err := c.client.InvokeInstance(ctx, &frontend_proxy.InvokeInstanceRequest{
		Context: rawFrontendRequestContext(c.frontendClientID, requestID, invokeReq.GetTraceID(), req.options),
		Invoke:  invokeReq,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("frontend proxy invoke response is nil")
	}
	if callResult := resp.GetCallResult(); callResult != nil {
		return marshalRuntimeNotifyFromCallResult(callResult)
	}
	if err := checkFrontendProxyStatus("invoke", resp.GetStatus()); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("frontend proxy invoke missing call result")
}

type frontendProxyRouteResolver interface {
	ResolveFrontendProxyAddress(req simpleRuntimeInvokeRequest) (string, error)
}

type frontendProxyServiceClientFactory interface {
	ClientForAddress(address string) (frontend_proxy.FrontendProxyServiceClient, error)
}

type frontendProxyServiceClientEvictor interface {
	EvictAddress(address string)
}

type routingFrontendProxyInvokeClient struct {
	resolver         frontendProxyRouteResolver
	clientFactory    frontendProxyServiceClientFactory
	frontendClientID string
}

type routingFrontendProxyLifecycleClient struct {
	clientFactory    frontendProxyServiceClientFactory
	frontendClientID string
}

func newRoutingFrontendProxyInvokeClient() frontendProxyInvokeClient {
	return &routingFrontendProxyInvokeClient{
		resolver:         defaultFrontendProxyRouteResolver{},
		clientFactory:    newFrontendProxyGRPCClientPool(),
		frontendClientID: currentFrontendClientID(),
	}
}

func newRoutingFrontendProxyLifecycleClient() frontendProxyLifecycleClient {
	return &routingFrontendProxyLifecycleClient{
		clientFactory:    newFrontendProxyGRPCClientPool(),
		frontendClientID: currentFrontendClientID(),
	}
}

func (c *routingFrontendProxyInvokeClient) InvokeByInstanceID(req simpleRuntimeInvokeRequest) ([]byte, error) {
	if c == nil || c.resolver == nil || c.clientFactory == nil {
		return nil, fmt.Errorf("frontend proxy routing client is not initialized")
	}
	address, err := c.resolver.ResolveFrontendProxyAddress(req)
	if err != nil {
		return nil, err
	}
	serviceClient, err := c.clientFactory.ClientForAddress(address)
	if err != nil {
		evictFrontendProxyClientOnError(c.clientFactory, address, err)
		return nil, err
	}
	payload, err := newGRPCFrontendProxyInvokeClient(serviceClient, c.frontendClientID).InvokeByInstanceID(req)
	if err != nil {
		evictFrontendProxyClientOnError(c.clientFactory, address, err)
		return nil, err
	}
	return payload, nil
}

func (c *routingFrontendProxyLifecycleClient) CreateInstance(req simpleRuntimeCreateRequest) (string, error) {
	if c == nil || c.clientFactory == nil {
		return "", fmt.Errorf("frontend proxy lifecycle routing client is not initialized")
	}
	tried := make(map[string]struct{})
	var lastErr error
	for {
		endpoint, ok := resolveNextFrontendProxyEndpoint(frontendProxyCapabilityCreate)
		if !ok {
			if lastErr != nil {
				return "", lastErr
			}
			return "", fmt.Errorf("no frontend proxy endpoint supports capability %s", frontendProxyCapabilityCreate)
		}
		if _, ok := tried[endpoint.Address]; ok {
			if lastErr != nil {
				return "", lastErr
			}
			return "", fmt.Errorf("no untried frontend proxy endpoint supports capability %s", frontendProxyCapabilityCreate)
		}
		tried[endpoint.Address] = struct{}{}
		serviceClient, err := c.clientFactory.ClientForAddress(endpoint.Address)
		if err != nil {
			evictFrontendProxyClientOnError(c.clientFactory, endpoint.Address, err)
			lastErr = err
			continue
		}
		instanceID, err := newGRPCFrontendProxyLifecycleClient(serviceClient, c.frontendClientID).CreateInstance(req)
		if err != nil {
			if isFrontendProxyCreatePreDispatchStatus(err) {
				lastErr = err
				continue
			}
			evictFrontendProxyClientOnError(c.clientFactory, endpoint.Address, err)
			return "", err
		}
		return instanceID, nil
	}
}

func (c *routingFrontendProxyLifecycleClient) CreateInstanceRaw(req simpleRuntimeRawCreateRequest) ([]byte, error) {
	if c == nil || c.clientFactory == nil {
		return nil, fmt.Errorf("frontend proxy lifecycle routing client is not initialized")
	}
	tried := make(map[string]struct{})
	var lastErr error
	for {
		endpoint, ok := resolveNextFrontendProxyEndpoint(frontendProxyCapabilityCreate)
		if !ok {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no frontend proxy endpoint supports capability %s", frontendProxyCapabilityCreate)
		}
		if _, ok := tried[endpoint.Address]; ok {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no untried frontend proxy endpoint supports capability %s", frontendProxyCapabilityCreate)
		}
		tried[endpoint.Address] = struct{}{}
		serviceClient, err := c.clientFactory.ClientForAddress(endpoint.Address)
		if err != nil {
			evictFrontendProxyClientOnError(c.clientFactory, endpoint.Address, err)
			lastErr = err
			continue
		}
		notify, err := newGRPCFrontendProxyLifecycleClient(serviceClient, c.frontendClientID).CreateInstanceRaw(req)
		if err != nil {
			if isFrontendProxyCreatePreDispatchStatus(err) {
				lastErr = err
				continue
			}
			evictFrontendProxyClientOnError(c.clientFactory, endpoint.Address, err)
			return nil, err
		}
		return notify, nil
	}
}

func (c *routingFrontendProxyLifecycleClient) KillInstance(req simpleRuntimeKillRequest) error {
	if c == nil || c.clientFactory == nil {
		return fmt.Errorf("frontend proxy lifecycle routing client is not initialized")
	}
	address := resolveProxyAddressByKill(req)
	if !isHostPort(address) {
		if endpoint, ok := resolveSoleFrontendProxyEndpoint(frontendProxyCapabilityKill); ok {
			address = endpoint.Address
		}
	}
	if !isHostPort(address) {
		return fmt.Errorf("frontend proxy kill route is not configured for instance %s", req.instanceID)
	}
	serviceClient, err := c.clientFactory.ClientForAddress(address)
	if err != nil {
		evictFrontendProxyClientOnError(c.clientFactory, address, err)
		return err
	}
	if err := newGRPCFrontendProxyLifecycleClient(serviceClient, c.frontendClientID).KillInstance(req); err != nil {
		evictFrontendProxyClientOnError(c.clientFactory, address, err)
		return err
	}
	return nil
}

func (c *routingFrontendProxyInvokeClient) InvokeByInstanceIDRaw(req simpleRuntimeRawInvokeRequest) ([]byte, error) {
	if c == nil || c.clientFactory == nil {
		return nil, fmt.Errorf("frontend proxy routing client is not initialized")
	}
	invokeReq := &core.InvokeRequest{}
	if err := proto.Unmarshal(req.invoke, invokeReq); err != nil {
		return nil, fmt.Errorf("failed to unmarshal frontend proxy invoke route request: %w", err)
	}
	requestRoute := ""
	if invokeReq.GetInvokeOptions() != nil {
		requestRoute = invokeReq.GetInvokeOptions().GetCustomTag()[frontendProxyRouteKey]
	}
	// Raw function-system requests can carry an old YR_ROUTE from their serialized
	// request. Prefer the frontend watcher cache for the current owning proxy when
	// it is available; fall back to the request route only when the instance route
	// is not known locally. Do not retry after an invoke is sent, because unknown
	// status requests may not be safe to replay.
	address := resolveProxyAddressByRawInvoke(invokeReq)
	if !isHostPort(address) {
		address = resolveFrontendProxyAddressFromRoute(requestRoute)
	}
	if !isHostPort(address) {
		if endpoint, ok := resolveSoleFrontendProxyEndpoint(frontendProxyCapabilityInvoke); ok {
			address = endpoint.Address
		}
	}
	if !isHostPort(address) {
		address = configuredFrontendProxyAddressIfHealthy()
	}
	if !isHostPort(address) {
		return nil, fmt.Errorf("frontend proxy invoke route is not configured for instance %s", invokeReq.GetInstanceID())
	}
	serviceClient, err := c.clientFactory.ClientForAddress(address)
	if err != nil {
		evictFrontendProxyClientOnError(c.clientFactory, address, err)
		return nil, err
	}
	notify, err := newGRPCFrontendProxyInvokeClient(serviceClient, c.frontendClientID).InvokeByInstanceIDRaw(req)
	if err != nil {
		evictFrontendProxyClientOnError(c.clientFactory, address, err)
		return nil, err
	}
	return notify, nil
}

type defaultFrontendProxyRouteResolver struct{}

func (defaultFrontendProxyRouteResolver) ResolveFrontendProxyAddress(req simpleRuntimeInvokeRequest) (string, error) {
	route := req.options.CreateOpt[frontendProxyRouteKey]
	// Prefer frontend's current instance cache over request-scoped route tags.
	// The cache is updated by the instance watcher and represents the current
	// owning proxy; a request tag can be absent or stale after reschedule/failover.
	if address := resolveProxyAddressByInstance(req); address != "" {
		return address, nil
	}
	if route == "" {
		return "", fmt.Errorf("frontend proxy route %s is empty for instance %s", frontendProxyRouteKey, req.instanceID)
	}
	if address := resolveFrontendProxyAddressFromRoute(route); address != "" {
		return address, nil
	}
	return "", fmt.Errorf("frontend proxy route %q is not resolvable for instance %s", route, req.instanceID)
}

func resolveFrontendProxyAddressFromRoute(route string) string {
	return resolveFrontendProxyAddressFromRouteWithCapability(route, frontendProxyCapabilityInvoke)
}

func resolveFrontendProxyAddressFromRouteWithCapability(route string, capability string) string {
	if isHostPort(route) {
		if frontendProxyAddressIsSuspect(route) {
			return ""
		}
		return route
	}
	if endpoint, ok := resolveFrontendProxyEndpointByNode(route, capability); ok {
		return endpoint.Address
	}
	return ""
}

func resolveFrontendProxyEndpointByNode(nodeID string, capability string) (frontendProxyEndpoint, bool) {
	discovery := currentFrontendProxyDiscovery()
	if discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	endpoint, ok := discovery.GetByNode(nodeID, capability)
	if !ok || !isHostPort(endpoint.Address) {
		return frontendProxyEndpoint{}, false
	}
	return endpoint, true
}

func resolveSoleFrontendProxyEndpoint(capability string) (frontendProxyEndpoint, bool) {
	discovery := currentFrontendProxyDiscovery()
	if discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	endpoint, ok := discovery.GetSoleEndpoint(capability)
	if !ok || !isHostPort(endpoint.Address) {
		return frontendProxyEndpoint{}, false
	}
	return endpoint, true
}

func resolveNextFrontendProxyEndpoint(capability string) (frontendProxyEndpoint, bool) {
	discovery := currentFrontendProxyDiscovery()
	if discovery == nil {
		return frontendProxyEndpoint{}, false
	}
	endpoint, ok := discovery.GetNextEndpoint(capability)
	if !ok || !isHostPort(endpoint.Address) {
		return frontendProxyEndpoint{}, false
	}
	return endpoint, true
}

func resolveProxyAddressByInstance(req simpleRuntimeInvokeRequest) string {
	if req.instanceID == "" {
		return ""
	}
	var instance *commontypes.InstanceSpecification
	if req.funcMeta.FuncID != "" {
		instance = instancemanager.GetGlobalInstanceScheduler().GetInstanceByID(req.funcMeta.FuncID, req.instanceID)
	}
	if instance == nil {
		instance = instancemanager.GetGlobalInstanceScheduler().GetInstanceByIDAcrossFunctions(req.instanceID)
	}
	return proxyAddressFromInstanceForCapability(instance, frontendProxyCapabilityInvoke)
}

func resolveProxyAddressByKill(req simpleRuntimeKillRequest) string {
	if req.instanceID == "" {
		return ""
	}
	var instance *commontypes.InstanceSpecification
	if instance == nil {
		instance = instancemanager.GetGlobalInstanceScheduler().GetInstanceByIDAcrossFunctions(req.instanceID)
	}
	if address := proxyAddressFromInstanceForCapability(instance, frontendProxyCapabilityKill); address != "" {
		return address
	}
	if req.options.CreateOpt != nil {
		return resolveFrontendProxyAddressFromRouteWithCapability(req.options.CreateOpt[frontendProxyRouteKey],
			frontendProxyCapabilityKill)
	}
	return ""
}

func resolveProxyAddressByRawInvoke(req *core.InvokeRequest) string {
	if req == nil || req.GetInstanceID() == "" {
		return ""
	}
	var instance *commontypes.InstanceSpecification
	if req.GetFunction() != "" {
		instance = instancemanager.GetGlobalInstanceScheduler().GetInstanceByID(req.GetFunction(), req.GetInstanceID())
	}
	if instance == nil {
		instance = instancemanager.GetGlobalInstanceScheduler().GetInstanceByIDAcrossFunctions(req.GetInstanceID())
	}
	return proxyAddressFromInstance(instance)
}

func isHostPort(address string) bool {
	host, port, err := net.SplitHostPort(address)
	return err == nil && strings.TrimSpace(host) != "" && strings.TrimSpace(port) != ""
}

func proxyAddressFromInstance(instance *commontypes.InstanceSpecification) string {
	return proxyAddressFromInstanceForCapability(instance, frontendProxyCapabilityInvoke)
}

func proxyAddressFromInstanceForCapability(instance *commontypes.InstanceSpecification, capability string) string {
	if instance == nil {
		return ""
	}
	if isHostPort(instance.FunctionProxyID) {
		if frontendProxyAddressIsSuspect(instance.FunctionProxyID) {
			return ""
		}
		return instance.FunctionProxyID
	}
	if endpoint, ok := resolveFrontendProxyEndpointByNode(instance.FunctionProxyID, capability); ok {
		return endpoint.Address
	}
	// If discovery already knows this owning proxy node but it is not eligible
	// for this frontend capability, do not bypass rollout by synthesizing an
	// address from RuntimeAddress + static port. RuntimeAddress fallback is only
	// for transition cases where the owning node is not yet published in
	// discovery.
	if frontendProxyNodeExistsInDiscovery(instance.FunctionProxyID) {
		return ""
	}
	return proxyAddressFromInstanceRuntimeAddress(instance)
}

func frontendProxyNodeExistsInDiscovery(nodeID string) bool {
	if nodeID == "" {
		return false
	}
	discovery := currentFrontendProxyDiscovery()
	if discovery == nil {
		return false
	}
	_, ok := discovery.GetByNode(nodeID, "")
	return ok
}

func proxyAddressFromInstanceRuntimeAddress(instance *commontypes.InstanceSpecification) string {
	if instance == nil || instance.RuntimeAddress == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(instance.RuntimeAddress)
	if err != nil || host == "" {
		return ""
	}
	address := net.JoinHostPort(host, constants.GRPCPort)
	if configuredAddress := configuredFrontendProxyAddress(); isHostPort(configuredAddress) {
		_, port, splitErr := net.SplitHostPort(configuredAddress)
		if splitErr == nil && port != "" {
			address = net.JoinHostPort(host, port)
		}
	}
	if frontendProxyAddressIsSuspect(address) {
		return ""
	}
	return address
}

func configuredFrontendProxyAddress() string {
	conf := config.GetConfig()
	if conf == nil {
		return ""
	}
	return conf.FrontendProxyAddress
}

func configuredFrontendProxyAddressIfHealthy() string {
	address := configuredFrontendProxyAddress()
	if !isHostPort(address) || frontendProxyAddressIsSuspect(address) {
		return ""
	}
	return address
}

type frontendProxyGRPCClientPool struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

func newFrontendProxyGRPCClientPool() *frontendProxyGRPCClientPool {
	return &frontendProxyGRPCClientPool{conns: make(map[string]*grpc.ClientConn)}
}

func (p *frontendProxyGRPCClientPool) ClientForAddress(address string) (frontend_proxy.FrontendProxyServiceClient, error) {
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("frontend proxy address is empty")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if conn, ok := p.conns[address]; ok {
		return frontend_proxy.NewFrontendProxyServiceClient(conn), nil
	}
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    time.Hour,
			Timeout: 10 * time.Second,
		}),
	)
	if err != nil {
		return nil, err
	}
	p.conns[address] = conn
	return frontend_proxy.NewFrontendProxyServiceClient(conn), nil
}

func evictFrontendProxyClientOnError(factory frontendProxyServiceClientFactory, address string, err error) {
	if err == nil || address == "" {
		return
	}
	var statusErr *frontendProxyStatusErr
	if errors.As(err, &statusErr) {
		return
	}
	var businessErr *frontendProxyBusinessErr
	if errors.As(err, &businessErr) {
		return
	}
	evictor, ok := factory.(frontendProxyServiceClientEvictor)
	if ok {
		evictor.EvictAddress(address)
	}
	MarkFrontendProxyEndpointSuspect(address)
}

func (p *frontendProxyGRPCClientPool) EvictAddress(address string) {
	if strings.TrimSpace(address) == "" {
		return
	}
	p.mu.Lock()
	conn, ok := p.conns[address]
	if ok {
		delete(p.conns, address)
	}
	p.mu.Unlock()
	if ok {
		_ = conn.Close()
	}
}

func simpleRuntimeInvokeContext(options api.InvokeOptions) (context.Context, context.CancelFunc) {
	timeout := defaultFrontendProxyTimeout
	if options.Timeout > 0 {
		timeout = time.Duration(options.Timeout) * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

func rawSimpleRuntimeContext(api.RawRequestOption) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultFrontendProxyTimeout)
}

func frontendRequestContextFromInvokeOptions(frontendClientID, tenantID, requestID string,
	options api.InvokeOptions,
) *frontend_proxy.FrontendRequestContext {
	ctx := &frontend_proxy.FrontendRequestContext{
		FrontendClientID: frontendClientID,
		TenantID:         tenantID,
		RequestID:        requestID,
		TraceID:          options.TraceID,
	}
	if traceParent := options.CustomExtensions[traceParentExtensionKey]; traceParent != "" {
		ctx.Labels = map[string]string{traceParentExtensionKey: traceParent}
	}
	return ctx
}

func rawFrontendRequestContext(frontendClientID, requestID, traceID string, option api.RawRequestOption,
	tenantID ...string,
) *frontend_proxy.FrontendRequestContext {
	ctx := &frontend_proxy.FrontendRequestContext{
		FrontendClientID: frontendClientID,
		RequestID:        requestID,
		TraceID:          traceID,
	}
	if len(tenantID) > 0 {
		ctx.TenantID = tenantID[0]
	}
	if option.TraceParent != "" {
		ctx.Labels = map[string]string{traceParentExtensionKey: option.TraceParent}
	}
	return ctx
}

func applyRawRequestOptionsToInvoke(invokeReq *core.InvokeRequest, option api.RawRequestOption) {
	if invokeReq == nil || option.TraceParent == "" {
		return
	}
	if invokeReq.InvokeOptions == nil {
		invokeReq.InvokeOptions = &core.InvokeOptions{}
	}
	if invokeReq.InvokeOptions.CustomTag == nil {
		invokeReq.InvokeOptions.CustomTag = map[string]string{}
	}
	invokeReq.InvokeOptions.CustomTag[traceParentExtensionKey] = option.TraceParent
}

func applyRawRequestOptionsToCreate(createReq *core.CreateRequest, option api.RawRequestOption) {
	if createReq == nil {
		return
	}
	if createReq.CreateOptions == nil {
		createReq.CreateOptions = map[string]string{}
	}
	if _, ok := createReq.CreateOptions[frontendProxyCreateSourceKey]; !ok {
		createReq.CreateOptions[frontendProxyCreateSourceKey] = frontendProxyCreateSource
	}
	if option.TraceParent != "" {
		createReq.CreateOptions[traceParentExtensionKey] = option.TraceParent
	}
}

func tenantIDFromCreateRequest(createReq *core.CreateRequest) string {
	if createReq == nil {
		return ""
	}
	for _, key := range []string{"tenantID", "tenantId", "tenant"} {
		if value := createReq.GetCreateOptions()[key]; value != "" {
			return value
		}
	}
	function := strings.Trim(createReq.GetFunction(), "/")
	if idx := strings.Index(function, "/"); idx > 0 {
		return function[:idx]
	}
	return ""
}

func checkFrontendProxyStatus(operation string, status *frontend_proxy.FrontendProxyStatus) error {
	if status == nil || status.GetCode() == common.ErrorCode_ERR_NONE {
		return nil
	}
	return frontendProxyStatusError(operation, status)
}

func isFrontendProxyCreatePreDispatchStatus(err error) bool {
	var statusErr *frontendProxyStatusErr
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.operation == "create" && statusErr.retryReason == frontendProxyControlNotWired
}

type frontendProxyStatusErr struct {
	operation   string
	code        common.ErrorCode
	message     string
	retryable   bool
	retryReason string
}

type frontendProxyBusinessErr struct {
	operation string
	code      common.ErrorCode
	message   string
}

func (e *frontendProxyBusinessErr) Error() string {
	if e == nil {
		return "frontend proxy failed with nil business error"
	}
	return fmt.Sprintf("frontend proxy %s failed, code: %v, message: %s",
		e.operation, e.code, e.message)
}

func (e *frontendProxyStatusErr) Error() string {
	if e == nil {
		return "frontend proxy failed with nil status error"
	}
	message := fmt.Sprintf("frontend proxy %s failed, code: %v, message: %s",
		e.operation, e.code, e.message)
	if e.retryable {
		message += ", retryable: true"
	}
	if e.retryReason != "" {
		message += fmt.Sprintf(", retryReason: %s", e.retryReason)
	}
	return message
}

func frontendProxyStatusError(operation string, status *frontend_proxy.FrontendProxyStatus) error {
	if status == nil {
		return fmt.Errorf("frontend proxy %s failed with nil status", operation)
	}
	return &frontendProxyStatusErr{
		operation:   operation,
		code:        status.GetCode(),
		message:     status.GetMessage(),
		retryable:   status.GetRetryable(),
		retryReason: status.GetRetryReason(),
	}
}

func frontendProxyBusinessError(operation string, code common.ErrorCode, message string) error {
	return &frontendProxyBusinessErr{
		operation: operation,
		code:      code,
		message:   message,
	}
}

func marshalRuntimeNotifyFromCallResult(callResult *core.CallResult) ([]byte, error) {
	var out []byte
	if callResult.GetRequestID() != "" {
		out = protowire.AppendTag(out, 1, protowire.BytesType)
		out = protowire.AppendString(out, callResult.GetRequestID())
	}
	out = protowire.AppendTag(out, 2, protowire.VarintType)
	out = protowire.AppendVarint(out, uint64(callResult.GetCode()))
	if callResult.GetMessage() != "" {
		out = protowire.AppendTag(out, 3, protowire.BytesType)
		out = protowire.AppendString(out, callResult.GetMessage())
	}
	for _, smallObject := range callResult.GetSmallObjects() {
		payload, err := proto.Marshal(smallObject)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal frontend proxy invoke small object: %w", err)
		}
		out = protowire.AppendTag(out, 4, protowire.BytesType)
		out = protowire.AppendBytes(out, payload)
	}
	for _, stackTraceInfo := range callResult.GetStackTraceInfos() {
		payload, err := proto.Marshal(stackTraceInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal frontend proxy create stack trace info: %w", err)
		}
		out = protowire.AppendTag(out, 5, protowire.BytesType)
		out = protowire.AppendBytes(out, payload)
	}
	if runtimeInfo := callResult.GetRuntimeInfo(); runtimeInfo != nil {
		payload, err := proto.Marshal(runtimeInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal frontend proxy create runtime info: %w", err)
		}
		out = protowire.AppendTag(out, 7, protowire.BytesType)
		out = protowire.AppendBytes(out, payload)
	}
	return out, nil
}

func createReadyCallResultFromResponse(resp *frontend_proxy.CreateInstanceResponse) (*core.CallResult, error) {
	if resp == nil {
		return nil, fmt.Errorf("frontend proxy create response is nil")
	}
	if callResult, typedFieldKnown, err := typedCreateReadyCallResultFromResponse(resp); err != nil || typedFieldKnown {
		return callResult, err
	}
	unknown := resp.ProtoReflect().GetUnknown()
	for len(unknown) > 0 {
		number, wireType, n := protowire.ConsumeTag(unknown)
		if n < 0 {
			return nil, fmt.Errorf("failed to parse frontend proxy create unknown field tag: %v", protowire.ParseError(n))
		}
		unknown = unknown[n:]
		if number == createReadyCallResultFieldNumber && wireType == protowire.BytesType {
			payload, n := protowire.ConsumeBytes(unknown)
			if n < 0 {
				return nil, fmt.Errorf("failed to parse frontend proxy create ready call result: %v", protowire.ParseError(n))
			}
			callResult := &core.CallResult{}
			if err := proto.Unmarshal(payload, callResult); err != nil {
				return nil, fmt.Errorf("failed to unmarshal frontend proxy create ready call result: %w", err)
			}
			return callResult, nil
		}
		n = protowire.ConsumeFieldValue(number, wireType, unknown)
		if n < 0 {
			return nil, fmt.Errorf("failed to skip frontend proxy create unknown field %d: %v",
				number, protowire.ParseError(n))
		}
		unknown = unknown[n:]
	}
	return nil, nil
}

func typedCreateReadyCallResultFromResponse(resp *frontend_proxy.CreateInstanceResponse) (*core.CallResult, bool, error) {
	message := resp.ProtoReflect()
	field := message.Descriptor().Fields().ByName(protoreflect.Name("callResult"))
	if field == nil {
		return nil, false, nil
	}
	if field.Kind() != protoreflect.MessageKind {
		return nil, true, fmt.Errorf("frontend proxy create callResult field has invalid kind %s", field.Kind())
	}
	if !message.Has(field) {
		return nil, true, nil
	}
	payload, err := proto.Marshal(message.Get(field).Message().Interface())
	if err != nil {
		return nil, true, fmt.Errorf("failed to marshal typed frontend proxy create ready call result: %w", err)
	}
	callResult := &core.CallResult{}
	if err := proto.Unmarshal(payload, callResult); err != nil {
		return nil, true, fmt.Errorf("failed to unmarshal typed frontend proxy create ready call result: %w", err)
	}
	return callResult, true, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func currentFrontendClientID() string {
	if podName := os.Getenv(constant.PodNameEnvKey); podName != "" {
		return "frontend:" + podName
	}
	return "frontend"
}

func convertSimpleRuntimeArgs(args []api.Arg) []*common.Arg {
	converted := make([]*common.Arg, 0, len(args))
	for _, arg := range args {
		converted = append(converted, &common.Arg{
			Type:       common.Arg_ArgType(arg.Type),
			Value:      arg.Data,
			NestedRefs: arg.NestedObjectIDs,
		})
	}
	return converted
}

func convertSimpleRuntimeInvokeOptions(options api.InvokeOptions) *core.InvokeOptions {
	customTag := make(map[string]string, len(options.CustomExtensions)+len(options.CreateOpt))
	for key, value := range options.CustomExtensions {
		customTag[key] = value
	}
	for key, value := range options.CreateOpt {
		customTag[key] = value
	}
	return &core.InvokeOptions{CustomTag: customTag}
}

func convertSimpleRuntimeCreateOptions(options api.InvokeOptions) map[string]string {
	createOptions := make(map[string]string, len(options.CustomExtensions)+len(options.CreateOpt)+1)
	for key, value := range options.CustomExtensions {
		createOptions[key] = value
	}
	for key, value := range options.CreateOpt {
		createOptions[key] = value
	}
	if _, ok := createOptions[frontendProxyCreateSourceKey]; !ok {
		createOptions[frontendProxyCreateSourceKey] = frontendProxyCreateSource
	}
	return createOptions
}

func firstArgTenantID(args []api.Arg) string {
	for _, arg := range args {
		if arg.TenantID != "" {
			return arg.TenantID
		}
	}
	return ""
}
