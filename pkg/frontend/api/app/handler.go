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

// Package app - used for car BU
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/logger/log"
	"frontend/pkg/common/faas_common/types"
	"frontend/pkg/common/job"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/instancemanager"
)

// CreateHandler -
func CreateHandler(ctx *gin.Context) {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		log.GetLogger().Errorf("failed to read request body error %v", err)
		SetCtxResponse(ctx, nil, http.StatusInternalServerError,
			fmt.Errorf("failed to read request body error %v", err))
		return
	}
	reqBody := &job.SubmitRequest{}
	err = json.Unmarshal(body, reqBody)
	if err != nil {
		log.GetLogger().Errorf("create app unmarshal request failed, err: %v", err)
		SetCtxResponse(ctx, nil, http.StatusBadRequest,
			fmt.Errorf("create app unmarshal request failed, err: %s", err))
		return
	}
	respBody, repCode, err := SubmitApp(reqBody)
	SetCtxResponse(ctx, respBody, repCode, err)
}

// SubmitApp -
func SubmitApp(reqBody *job.SubmitRequest) (map[string]string, int, error) {
	logger := log.GetLogger().With(zap.Any("SubmissionId", reqBody.SubmissionId))
	logger.Debugf("start to submit app")
	functionID := reqBody.FunctionID
	if functionID == "" {
		functionID = constant.AppFuncId
	}
	funcMeta := api.FunctionMeta{
		FuncName: constant.FunctionNameApp,
		FuncID:   functionID,
		Api:      api.ActorApi,
		Language: api.Python,
		Name:     &reqBody.SubmissionId,
	}
	invokeOpts := createInvokeOpts(reqBody)
	logger.Debugf("begin to invoke libruntime api: CreateInstanceByLibRt")
	instanceId, err := util.NewClient().CreateInstanceByLibRt(funcMeta, []api.Arg{}, invokeOpts)
	if err != nil {
		logger.Errorf("create app failed, err: %v", err)
		return nil, http.StatusInternalServerError,
			fmt.Errorf("create app failed, submissionId:[%s], err: %v", reqBody.SubmissionId, err)
	}
	logger.Debugf("submit app success")
	return map[string]string{
		"submission_id": instanceId,
	}, http.StatusOK, nil
}

// ListHandler -
func ListHandler(ctx *gin.Context) {
	respBody, repCode, err := ListApps(ctx)
	SetCtxResponse(ctx, respBody, repCode, err)
}

// ListApps -
func ListApps(ctx *gin.Context) ([]*constant.AppInfo, int, error) {
	traceID := ctx.Request.Header.Get(constant.HeaderTraceID)
	log.GetLogger().Debugf("start to list apps, traceID:%s", traceID)
	logger := log.GetLogger().With(zap.Any("traceID", traceID))
	apps, err := instancemanager.ListAppsInfo()
	if err != nil {
		logger.Errorf("list apps failed, err: %v", err)
		return nil, http.StatusInternalServerError, fmt.Errorf("list apps failed, err: %w", err)
	}
	logger.Debugf("list apps success")
	return apps, http.StatusOK, nil
}

// GetInfoHandler -
func GetInfoHandler(ctx *gin.Context) {
	submissionId := ctx.Param(job.PathParamSubmissionId)
	respBody, repCode, err := GetAppInfo(submissionId)
	SetCtxResponse(ctx, respBody, repCode, err)
}

// GetAppInfo -
func GetAppInfo(submissionId string) (*constant.AppInfo, int, error) {
	logger := log.GetLogger().With(zap.Any("SubmissionId", submissionId))
	logger.Debugf("start to get app")
	appInfo, err := instancemanager.GetAppDetailsByID(submissionId)
	if err != nil {
		logger.Errorf("not found app, err: %v", err)
		return nil, http.StatusNotFound,
			fmt.Errorf("not found app, submissionId: %s, err: %w", submissionId, err)
	}
	logger.Debugf("get app success")
	return appInfo, http.StatusOK, nil
}

// DeleteHandler -
func DeleteHandler(ctx *gin.Context) {
	respBody, repCode, err := DeleteApp(ctx)
	SetCtxResponse(ctx, respBody, repCode, err)
}

// DeleteApp -
func DeleteApp(ctx *gin.Context) (string, int, error) {
	submissionId := ctx.Param(job.PathParamSubmissionId)
	logger := log.GetLogger().With(zap.Any("SubmissionId", submissionId))
	logger.Debugf("start to delete app")
	status := instancemanager.GetAppStatusByID(submissionId)
	if status == "" {
		logger.Errorf("the app does not exist")
		return "", http.StatusNotFound, fmt.Errorf("the app does not exist, submissionId:[%s]", submissionId)
	}
	if status != constant.AppStatusSucceeded && status != constant.AppStatusFailed && status != constant.AppStatusStopped {
		logger.Errorf("the app isn't allow to delete, status: %s", status)
		return status, http.StatusForbidden,
			fmt.Errorf("the app isn't allow to delete, submissionId:[%s], status: %s", submissionId, status)
	}
	logger.Debugf("send signal to kernel to delete app, submissionId: %s", submissionId)
	err := util.NewClient().KillByLibRt(submissionId, constant.KillSignalVal, []byte("the job was manually deleted"))
	if err != nil {
		logger.Errorf("delete app failed, status:[%s] err: %v", status, err)
		return status, http.StatusInternalServerError,
			fmt.Errorf("delete app failed, submissionId:[%s], status:[%s] err: %v", submissionId, status, err)
	}
	logger.Debugf("delete app success")
	return status, http.StatusOK, nil
}

// StopHandler -
func StopHandler(ctx *gin.Context) {
	respBody, repCode, err := StopApp(ctx)
	SetCtxResponse(ctx, respBody, repCode, err)
}

// StopApp -
func StopApp(ctx *gin.Context) (string, int, error) {
	submissionId := ctx.Param(job.PathParamSubmissionId)
	logger := log.GetLogger().With(zap.Any("SubmissionId", submissionId))
	logger.Debugf("start to stop app")
	status := instancemanager.GetAppStatusByID(submissionId)
	if status == "" {
		logger.Errorf("the app does not exist")
		return "", http.StatusNotFound, fmt.Errorf("the app does not exist, submissionId:[%s]", submissionId)
	}
	if status != constant.AppStatusRunning {
		logger.Errorf("the app isn't allow to stop, status: %s", status)
		return status, http.StatusForbidden,
			fmt.Errorf("the app isn't allow to stop, submissionId:[%s], status: %s", submissionId, status)
	}
	err := util.NewClient().KillByLibRt(submissionId, constant.StopAppSignalVal, []byte("the job was manually stopped"))
	if err != nil {
		logger.Debugf("stop app failed, status:[%s] err: %v", status, err)
	}
	logger.Debugf("stop app success")
	return status, http.StatusOK, nil
}

// SetCtxResponse set ctx response
func SetCtxResponse(ctx *gin.Context, data interface{}, code int, err error) {
	if data == nil {
		log.GetLogger().Warnf("the body of ctx response is empty")
	}
	ctx.JSON(code, job.BuildJobResponse(data, code, err))
}

func createInvokeOpts(reqBody *job.SubmitRequest) api.InvokeOptions {
	logger := log.GetLogger().With(zap.Any("SubmissionId", reqBody.SubmissionId))
	invokeOpts := api.InvokeOptions{
		Cpu:             int(reqBody.EntrypointNumCpus),
		Memory:          reqBody.EntrypointMemory,
		CustomResources: reqBody.EntrypointResources,
		CreateOpt:       buildCreateOpt(reqBody),
		Timeout:         constant.AppInvokeTimeout,
	}
	if reqBody.Labels != "" {
		invokeOpts.ScheduleAffinities = generateScheduleAffinity(invokeOpts.ScheduleAffinities, reqBody.Labels)
	}
	logger.Debugf("create app invokeOpts is: %v", invokeOpts)
	return invokeOpts
}

func buildCreateOpt(reqBody *job.SubmitRequest) map[string]string {
	logger := log.GetLogger().With(zap.Any("SubmissionId", reqBody.SubmissionId))
	createOpt := make(map[string]string)
	if reqBody.CreateOptions != nil {
		createOpt = reqBody.CreateOptions
	}
	createOpt[constant.EntryPointKey] = reqBody.Entrypoint
	if reqBody.RuntimeEnv != nil {
		if len(reqBody.RuntimeEnv.Pip) != 0 {
			pipCmd := fmt.Sprintf("%s %s && %s", constant.PipInstallPrefix,
				strings.Join(reqBody.RuntimeEnv.Pip, " "), constant.PipCheckSuffix)
			createOpt[constant.PostStartExec] = pipCmd
		}
		if reqBody.RuntimeEnv.WorkingDir != "" {
			codePath := reqBody.RuntimeEnv.WorkingDir
			delegateDownLoadValue := types.LocalMetaData{
				StorageType: constant.WorkingDirType,
				CodePath:    codePath,
			}
			workingDir, err := json.Marshal(delegateDownLoadValue)
			if err != nil {
				logger.Warnf("workingDir JSON marshaling failed: %s", err.Error())
			}
			createOpt[constant.DelegateDownloadKey] = string(workingDir)
		}
		if len(reqBody.RuntimeEnv.EnvVars) != 0 {
			envVars := reqBody.RuntimeEnv.EnvVars
			envVarsJsonByte, err := json.Marshal(envVars)
			if err != nil {
				logger.Warnf("env Vars JSON marshaling failed: %s", err.Error())
			}
			createOpt[constant.DelegateEnvVar] = string(envVarsJsonByte)
		}
		if len(reqBody.Metadata) != 0 {
			userMetadataJsonByte, err := json.Marshal(reqBody.Metadata)
			if err != nil {
				logger.Warnf("userMetadata JSON marshaling failed: %s", err.Error())
			}
			createOpt[constant.UserMetadataKey] = string(userMetadataJsonByte)
		}
	}
	return createOpt
}

func generateScheduleAffinity(scheduleAffinity []api.Affinity, label string) []api.Affinity {
	if label == "" {
		return scheduleAffinity
	}
	labels := strings.Split(label, ",")
	for _, poolLabel := range labels {
		if strings.TrimSpace(poolLabel) == constant.UnUseAntiOtherLabelsKey {
			continue
		}
		affinity := api.Affinity{
			Kind:                     api.AffinityKindResource,
			Affinity:                 api.PreferredAffinity,
			PreferredPriority:        true,
			PreferredAntiOtherLabels: !strings.Contains(label, constant.UnUseAntiOtherLabelsKey),
			LabelOps: []api.LabelOperator{
				{
					Type:        api.LabelOpExists,
					LabelKey:    strings.TrimSpace(poolLabel),
					LabelValues: nil,
				},
			},
		}
		scheduleAffinity = append(scheduleAffinity, affinity)
	}
	return scheduleAffinity
}
