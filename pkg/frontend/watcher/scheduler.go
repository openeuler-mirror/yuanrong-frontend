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
	"strings"

	"go.uber.org/zap"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/schedulerproxy"
)

// start to watch the module schedulers by the etcd
func startWatchScheduler(stopCh <-chan struct{}) {
	etcdClient := etcd3.GetRouterEtcdClient()
	watcher := etcd3.NewEtcdWatcher(constant.SchedulerHashPrefix, schedulerFilter, schedulerHandler,
		stopCh, etcdClient)
	watcher.StartWatch()
}

func schedulerFilter(event *etcd3.Event) bool {
	return !strings.Contains(event.Key, constant.SchedulerHashPrefix)
}

func schedulerHandler(event *etcd3.Event) {
	logger := log.GetLogger().With(zap.Any("eventType", event.Type), zap.Any("eventKey", event.Key),
		zap.Any("revisionId", event.Rev))
	logger.Infof("recv scheduler event type")
	if event.Type == etcd3.SYNCED {
		return
	}
	switch event.Type {
	case etcd3.SYNCED:
		logger.Infof("faaSFrontend scheduler ready to receive etcd kv")
	case etcd3.PUT:
		schedulerproxy.EventManager.ProcessUpdate(event, logger)
	case etcd3.DELETE:
		schedulerproxy.EventManager.ProcessDelete(event, logger)
	default:
		logger.Warnf("unsupported event, type is %d", event.Type)
	}
}
