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

// Package schedulerproxy -
package schedulerproxy

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/faas_common/utils"
	"frontend/pkg/frontend/config"
)

// EventManager -
var EventManager *EventManagerInfo

// EventManagerInfo -
type EventManagerInfo struct {
	leaseIds       map[string]map[string]time.Time // instanceName : leaseId : time
	schedulerInfos map[string]*types.InstanceInfo
	sync.RWMutex
}

func init() {
	EventManager = &EventManagerInfo{
		leaseIds:       make(map[string]map[string]time.Time),
		schedulerInfos: make(map[string]*types.InstanceInfo),
		RWMutex:        sync.RWMutex{},
	}
}

// ProcessDelete -
func (m *EventManagerInfo) ProcessDelete(event *etcd3.Event, logger api.FormatLogger) {
	leaseId := ""
	instanceName, isLeaseKey := utils.GetInstanceNameFromSchedulerLeaseEtcdKey(event.Key)
	if !isLeaseKey {
		info, err := utils.GetSchedulerInfoFromEtcdKey(event.Key)
		if err != nil {
			return
		}
		instanceName = info.InstanceName
	} else {
		leaseId = utils.ParseLeaseFromSchedulerLeaseEtcdKey(event.Key)
	}

	m.Lock()
	info, _ := m.schedulerInfos[instanceName]
	if isLeaseKey {
		leases, ok := m.leaseIds[instanceName]
		if ok {
			delete(leases, leaseId)
		}
		if len(leases) == 0 {
			delete(m.leaseIds, instanceName)
		}
	} else {
		delete(m.schedulerInfos, instanceName)
	}
	m.Unlock()

	m.RLock()
	_, existLease := m.leaseIds[instanceName]
	_, existSchedulerInfo := m.schedulerInfos[instanceName]
	m.RUnlock()
	needRemove := false

	switch config.GetConfig().SchedulerKeyPrefixType {
	case constant.SchedulerKeyTypeModule:
		if existLease != existSchedulerInfo {
			needRemove = true
		}
	default:
		if !existSchedulerInfo && !isLeaseKey {
			needRemove = true
		}
	}

	defer logger.Infof("process delete event over")
	if needRemove && info != nil && Proxy.ExistInstanceName(info.InstanceName) {
		Proxy.Remove(info, logger)
		logger.Infof("deleted from ProxyManager")
	}
}

// ProcessUpdate -
func (m *EventManagerInfo) ProcessUpdate(event *etcd3.Event, logger api.FormatLogger) {
	defer logger.Infof("process update event over")
	instanceName, isLeaseKey := utils.GetInstanceNameFromSchedulerLeaseEtcdKey(event.Key)
	if isLeaseKey {
		m.Lock()
		if _, ok := m.leaseIds[instanceName]; !ok {
			m.leaseIds[instanceName] = make(map[string]time.Time)
		}
		m.leaseIds[instanceName][utils.ParseLeaseFromSchedulerLeaseEtcdKey(event.Key)] = time.Now()
		m.Unlock()
	} else {
		info, err := getSchedulerInstanceInfo(event, logger)
		if err != nil {
			return
		}
		instanceName = info.InstanceName
		m.Lock()
		m.schedulerInfos[instanceName] = info
		m.Unlock()
	}
	var updateTime time.Time
	m.RLock()
	leases, existLease := m.leaseIds[instanceName]
	if existLease {
		for _, v := range leases {
			if updateTime.After(v) {
				updateTime = v
			}
		}
	}
	info, existSchedulerInfo := m.schedulerInfos[instanceName]
	m.RUnlock()

	needUpdate := false
	switch config.GetConfig().SchedulerKeyPrefixType {
	case constant.SchedulerKeyTypeModule:
		if existLease && existSchedulerInfo {
			needUpdate = true
		}
	default:
		needUpdate = existSchedulerInfo
	}
	if !needUpdate {
		return
	}

	logger = logger.With(zap.Any("instanceName", info.InstanceName), zap.Any("instanceId", info.InstanceID))
	schedulerNodeInfo := &SchedulerNodeInfo{InstanceInfo: info, UpdateTime: updateTime}
	Proxy.Add(schedulerNodeInfo, logger)
	logger.Infof("add to ProxyManager")
}

func getSchedulerInstanceInfo(event *etcd3.Event, logger api.FormatLogger) (*types.InstanceInfo, error) {
	info, err := utils.GetSchedulerInfoFromEtcdKey(event.Key)
	if err != nil {
		return nil, err
	}
	insSpecInfo := &types.InstanceSpecification{}
	if len(event.Value) == 0 {
		return nil, fmt.Errorf("value is empty")
	}
	if len(event.Value) != 0 {
		err = json.Unmarshal(event.Value, insSpecInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal ProxyManager to insSpecInfo, error %s", err.Error())
		}
	}
	if insSpecInfo.CreateOptions != nil {
		info.Exclusivity = insSpecInfo.CreateOptions[constant.SchedulerExclusivityKey]
	}
	info.InstanceID = insSpecInfo.InstanceID
	info.Address = insSpecInfo.RuntimeAddress
	return info, nil
}
