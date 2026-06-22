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

package resolver

import (
	"context"
	"errors"
	"strings"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/sandboxrouter/execendpoint"
	"frontend/pkg/sandboxrouter/route"
)

// instanceEtcdKeyLen is the '/'-separated depth of a full instance key
// (/sn/instance/business/yrk/tenant/{t}/function/{f}/version/{v}/defaultaz/{r}/{id}),
// mirroring the filter in frontend's existing instanceinfo watcher.
const instanceEtcdKeyLen = 14

// errNoEtcdClient is returned by Start when the shared router etcd client is
// not initialised.
var errNoEtcdClient = errors.New("sandboxrouter: router etcd client not initialised")

// InstanceInfoWatchResolver maintains a route cache by watching /sn/instance
// and replicating the FunctionMaster route computation. It reuses frontend's
// existing etcd watch infrastructure and is fully decoupled from the Traefik
// etcd-KV registry / HTTP provider.
type InstanceInfoWatchResolver struct {
	cache *route.RouteCache
}

// NewInstanceInfoWatchResolver returns a resolver with an empty cache.
func NewInstanceInfoWatchResolver() *InstanceInfoWatchResolver {
	return &InstanceInfoWatchResolver{cache: route.NewRouteCache()}
}

// Start lists existing instances and watches for changes until stopCh closes.
// The watcher emits a PUT per existing key on startup, so the initial cache is
// populated without a separate full-list step.
func (r *InstanceInfoWatchResolver) Start(stopCh <-chan struct{}) error {
	client := etcd3.GetRouterEtcdClient()
	if client == nil {
		return errNoEtcdClient
	}
	watcher := etcd3.NewEtcdWatcher(constant.InstancePathPrefix, sandboxInstanceFilter, r.applyEvent, stopCh, client)
	watcher.StartWatch()
	log.GetLogger().Infof("sandboxrouter: watching instance info under %s", constant.InstancePathPrefix)
	return nil
}

// Resolve answers from the in-memory cache (no I/O; ctx unused).
func (r *InstanceInfoWatchResolver) Resolve(_ context.Context, key route.RouteKey) (*route.RouteTarget, error) {
	return r.cache.Get(key)
}

// applyEvent maps an etcd watch event onto the route cache. PUT/DELETE are
// translated to the etcd-free route.ApplyInstanceEvent; SYNCED/ERROR are ignored.
// The same event also feeds the exec endpoint cache (execendpoint), so the web
// terminal / file-copy exec path can resolve an instance's proxyGrpcAddress
// locally instead of querying the master, reusing this one watch.
func (r *InstanceInfoWatchResolver) applyEvent(event *etcd3.Event) {
	switch event.Type {
	case etcd3.PUT:
		route.ApplyInstanceEvent(r.cache, route.EventPut, event.Key, event.Value)
		execendpoint.ApplyInstanceEvent(execendpoint.Default(), execendpoint.EventPut, event.Key, event.Value)
	case etcd3.DELETE:
		route.ApplyInstanceEvent(r.cache, route.EventDelete, event.Key, event.PrevValue)
		execendpoint.ApplyInstanceEvent(execendpoint.Default(), execendpoint.EventDelete, event.Key, event.PrevValue)
	}
}

// sandboxInstanceFilter returns true to SKIP an event. It keeps only full
// instance keys (depth instanceEtcdKeyLen); sub-keys and other records are
// skipped. Non-sandbox instances pass through but yield no routes.
func sandboxInstanceFilter(event *etcd3.Event) bool {
	return len(strings.Split(event.Key, constant.ETCDEventKeySeparator)) != instanceEtcdKeyLen
}
