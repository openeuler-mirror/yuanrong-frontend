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

// Package instancemanager -
package instancemanager

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/types"
)

// key is instance_id/app_id/submission_id, value is *AppInfo
var appsInfo sync.Map

// GetAppDetailsByID -
func GetAppDetailsByID(submissionID string) (*constant.AppInfo, error) {
	logger := log.GetLogger().With(zap.Any("SubmissionId", submissionID))
	value, ok := appsInfo.Load(submissionID)
	if ok {
		appInfo, _ := value.(*constant.AppInfo)
		return appInfo, nil
	}
	err := fmt.Errorf("failed to get appjobInfo, submissionID: %s", submissionID)
	logger.Errorf(err.Error())
	return nil, err
}

// GetAppStatusByID -
func GetAppStatusByID(submissionID string) string {
	logger := log.GetLogger().With(zap.Any("SubmissionId", submissionID))
	value, ok := appsInfo.Load(submissionID)
	if ok {
		appInfo, ok := value.(*constant.AppInfo)
		if !ok {
			logger.Errorf("failed to Get AppDetails")
			return ""
		}
		return appInfo.Status
	}
	logger.Errorf("the submissionID not exist")
	return ""
}

// ListAppsInfo -
func ListAppsInfo() ([]*constant.AppInfo, error) {
	listAppsInfo := make([]*constant.AppInfo, 0)
	appsInfo.Range(func(k, v interface{}) bool {
		appInfo, ok := v.(*constant.AppInfo)
		if !ok {
			log.GetLogger().Errorf("list appsInfo failed!")
		}
		listAppsInfo = append(listAppsInfo, appInfo)
		return true
	})
	return listAppsInfo, nil
}

// ProcessAppInfoUpdate -
func ProcessAppInfoUpdate(event *etcd3.Event) {
	logger := log.GetLogger().With(zap.Any("etcdKey", event.Key))
	appInfo := &types.InstanceSpecification{}
	err := json.Unmarshal(event.Value, appInfo)
	if err != nil {
		logger.Errorf("failed to unmarshal app event, err: %s", err.Error())
		return
	}
	StoreAppInfo(event.Key, appInfo)
	return
}

// ProcessAppInfoDelete -
func ProcessAppInfoDelete(event *etcd3.Event) {
	keyParts := strings.Split(event.Key, constant.ETCDEventKeySeparator)
	if len(keyParts) != constant.ValidEtcdKeyLenForInstance {
		log.GetLogger().Warnf("failed to delete app %s", event.Key)
		return
	}
	appsInfo.Delete(keyParts[constant.InstanceIDIndexForInstance])
	return
}

func switch2AppStatus(statsCode constant.InstanceStatus, statusType constant.InstanceStatusType) string {
	appStatus := ""
	switch statsCode {
	case constant.KernelInstanceStatusScheduling, constant.KernelInstanceStatusCreating:
		appStatus = constant.AppStatusPending
	case constant.KernelInstanceStatusRunning, constant.KernelInstanceStatusExiting,
		constant.KernelInstanceStatusSubHealth:
		appStatus = constant.AppStatusRunning
	case constant.KernelInstanceStatusFailed, constant.KernelInstanceStatusScheduleFailed,
		constant.KernelInstanceStatusEvicting, constant.KernelInstanceStatusEvicted:
		appStatus = constant.AppStatusFailed
	case constant.KernelInstanceStatusFatal:
		if statusType == constant.KernelInstanceStatusTypeReturn {
			appStatus = constant.AppStatusSucceeded
		} else if statusType == constant.KernelInstanceStatusTypeUserKillInfo {
			appStatus = constant.AppStatusStopped
		} else {
			appStatus = constant.AppStatusFailed
		}
	default:
		log.GetLogger().Warnf("invalid appStatusCode", statsCode)
	}
	return appStatus
}

// StoreAppInfo -
func StoreAppInfo(key string, value *types.InstanceSpecification) {
	if !strings.Contains(value.InstanceID, constant.FunctionNameApp) {
		return
	}
	appStatusCode := constant.InstanceStatus(value.InstanceStatus.Code)
	appStatusType := constant.InstanceStatusType(value.InstanceStatus.Type)
	logger := log.GetLogger().With(zap.Any("etcdKey", key))
	appStatus := switch2AppStatus(appStatusCode, appStatusType)
	var delegateDownload types.LocalMetaData
	var workingDir, errType, endTime string
	err := json.Unmarshal([]byte(value.CreateOptions[constant.DelegateDownloadKey]), &delegateDownload)
	if err != nil {
		logger.Warnf("marshal CreateOptions[DELEGATE_DOWNLOAD] failed")
	} else {
		workingDir = delegateDownload.CodePath
	}
	runtimeEnv := map[string]interface{}{
		"pip":                   value.CreateOptions[constant.PostStartExec],
		constant.WorkingDirType: workingDir,
		"envVars":               value.CreateOptions[constant.DelegateEnvVar],
	}
	driverInfo := constant.DriverInfo{
		ID:            value.InstanceID,
		PID:           value.Extensions.PID,
		NodeIPAddress: strings.Split(value.RuntimeAddress, ":")[0],
	}
	if appStatus == constant.AppStatusFailed || appStatus == constant.AppStatusStopped {
		errType = value.InstanceStatus.Msg
	}
	if appStatus == constant.AppStatusFailed || appStatus == constant.AppStatusStopped ||
		appStatus == constant.AppStatusSucceeded && &value.Extensions != nil {
		endTime = value.Extensions.UpdateTimestamp
	}
	appInfo := &constant.AppInfo{ // app_id, instance_id & submission_id values are the same
		Key:            key,
		Type:           constant.AppType,
		Entrypoint:     value.CreateOptions[constant.EntryPointKey],
		SubmissionID:   value.InstanceID,
		Status:         appStatus,
		RuntimeEnv:     runtimeEnv,
		Metadata:       str2map(value.CreateOptions[constant.UserMetadataKey]),
		DriverExitCode: value.InstanceStatus.ExitCode,
		DriverInfo:     driverInfo,
		ErrorType:      errType,
		StartTime:      value.Extensions.CreateTimestamp,
		EndTime:        endTime,
		DriverNodeID:   value.FunctionProxyID,
	}
	appsInfo.Store(value.InstanceID, appInfo)
}

func str2map(jsonStr string) map[string]string {
	var result map[string]string
	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		log.GetLogger().Warnf("failed to unmarshal %s, err: %s", jsonStr, err.Error())
		return nil
	}
	return result
}
