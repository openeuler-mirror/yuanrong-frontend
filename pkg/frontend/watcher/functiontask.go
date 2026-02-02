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

package watcher

import (
	"encoding/json"
	"strings"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/functiontask"
)

const (
	// NodeEtcdPrefix is the etcd prefix for live node
	NodeEtcdPrefix = "/sn/workers/business/yrk/tenant/0/function/function-task"
	// ValidNodePathCount is the seprator count of Node Path
	ValidNodePathCount = 13
)

// StartWatchFunctionProxy -
func startWatchFunctionProxy(stopCh <-chan struct{}) {
	etcdClient := etcd3.GetRouterEtcdClient()
	watcher := etcd3.NewEtcdWatcher(NodeEtcdPrefix, IsTaskNode, processFunctionTaskEvent, stopCh, etcdClient)
	watcher.StartWatch()
}

func processFunctionTaskEvent(event *etcd3.Event) {
	switch event.Type {
	case etcd3.PUT:
		ft := &struct {
			NodeIP string // 目前只对nodeIP感兴趣，其他字段不重要，基本没用，如果有用到，再加上
		}{}
		err := json.Unmarshal(event.Value, ft)
		if err != nil {
			log.GetLogger().Errorf("error is %s", err)
			log.GetLogger().Warnf("unmarshal functiontask etcd event failed: %s", event.Value)
			return
		}
		functiontask.GetBusProxies().Add(event.Key, ft.NodeIP)

	case etcd3.DELETE:
		functiontask.GetBusProxies().Delete(event.Key)
	case etcd3.ERROR:
		log.GetLogger().Warnf("etcd error event: %s", event.Value)
	default:
		log.GetLogger().Warnf("unsupported event: %s", event.Value)
	}
}

// IsTaskNode /sn/workers/business/yrk/tenant/0/function/function-task/version/$latest/defaultaz/node01
func IsTaskNode(event *etcd3.Event) bool {
	strs := strings.Split(event.Key, constant.KeySeparator)
	if len(strs) != ValidNodePathCount {
		return true
	}
	if strs[6] != "0" || strs[8] != "function-task" {
		return true
	}
	return false
}
