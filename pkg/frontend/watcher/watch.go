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

// Package watcher -
package watcher

import (
	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/frontend/config"
)

// StartWatch start watching etcd with scheduler, alias and function
func StartWatch(stopCh <-chan struct{}) error {
	log.GetLogger().Infof("FaaS-Frontend etcd watcher starting...")
	go startWatchScheduler(stopCh)
	go startWatchRemoteClientLease(stopCh)
	go startWatchFunctionMeta(stopCh)
	go startWatchFunctionCR(stopCh)
	if config.GetConfig().BusinessType == constant.BusinessTypeFG {
		go startWatchCAEFunctionMeta(stopCh)
		go startWatchFunctionProxy(stopCh)
	}
	go startWatchCAEFunctionMeta(stopCh)
	go startWatchFunctionProxy(stopCh)
	go startWatchAlias(stopCh)
	go startWatchTenantQOS(stopCh)
	go startWatchInstanceInfo(stopCh)
	go startWatchInstanceConfig(stopCh)
	return nil
}
