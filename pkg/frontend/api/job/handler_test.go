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

package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"

	"yuanrong.org/kernel/runtime/libruntime/api"

	"frontend/pkg/common/constants"
	"frontend/pkg/common/faas_common/constant"
	"frontend/pkg/common/faas_common/types"
	mockUtils "frontend/pkg/common/faas_common/utils"
	"frontend/pkg/common/job"
	"frontend/pkg/frontend/common/util"
	"frontend/pkg/frontend/instancemanager"
)

func addApps(submissionId string) []*constant.AppInfo {
	var result []*constant.AppInfo
	return append(result, buildAppInfo(submissionId))
}

func buildAppInfo(submissionId string) *constant.AppInfo {
	return &constant.AppInfo{
		Key:          submissionId,
		Type:         "SUBMISSION",
		SubmissionID: submissionId,
		RuntimeEnv: map[string]interface{}{
			"working_dir": "",
			"pip":         "",
			"envVars":     "",
		},
		DriverInfo: constant.DriverInfo{
			ID: submissionId,
		},
		Status: "RUNNING",
	}
}

func storeDefaultApp(key string, code int32, statusType int32) {
	instancemanager.StoreAppInfo(key, &types.InstanceSpecification{
		InstanceID: key,
		StartTime:  "",
		InstanceStatus: types.InstanceStatus{
			Code:     code,
			Type:     statusType,
			ExitCode: 0,
		},
	})
}

func TestListJobsHandler(t *testing.T) {
	// list处理优先执行，因为存储job的appInfo是一个全局sync.map，所以其它用例添加了元素后可能会对查询结果造成一定影响
	convey.Convey("test ListJobsHandler", t, func() {
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rw)
		c.Request = &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Path: job.PathGroupJobs},
			Header: http.Header{
				"Content-Type":            []string{"application/json"},
				constants.HeaderTenantID:  []string{"123456"},
				constants.HeaderPoolLabel: []string{"abc"},
			},
			Body: nil, // 使用 io.NopCloser 包装 reader，使其满足 io.ReadCloser 接口
		}
		convey.Convey("when process success", func() {
			ListJobsHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldStartWith, "[")
		})
		convey.Convey("when list job is not empty", func() {
			storeDefaultApp("app-frontend-list1", 3, 0)
			ListJobsHandler(c)
			expectedResult, err := json.Marshal(addApps("app-frontend-list1"))
			if err != nil {
				t.Errorf("marshal expected result failed, err: %v", err)
			}
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, string(expectedResult))
		})
	})
}

func TestSubmitJobHandler(t *testing.T) {
	convey.Convey("test SubmitJobHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		submissionId := "app-frontend-job-submit1"
		util.SetAPIClientLibruntime(mock)
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rw)
		bodyBytes, _ := json.Marshal(job.SubmitRequest{
			Entrypoint:   "sleep 200",
			SubmissionId: submissionId,
			RuntimeEnv: &job.RuntimeEnv{
				WorkingDir: "file:///home/ray/codeNew/god_gt_factory/file/lidar_seg_042801.zip",
				Pip:        []string{"numpy==1.24", "scipy==1.25"},
				EnvVars: map[string]string{
					"SOURCE_REGION": "suzhou_std",
					"DEPLOY_REGION": "suzhou_std",
				},
			},
			Metadata: map[string]string{
				"autoscenes_ids": "auto_1-test",
				"task_type":      "task_1",
				"ttl":            "1250",
			},
			EntrypointResources: map[string]float64{
				"NPU": 0,
			},
			EntrypointNumCpus: 300,
			EntrypointNumGpus: 0,
			EntrypointMemory:  0,
		})
		reader := bytes.NewBuffer(bodyBytes)
		c.Request = &http.Request{
			Method: http.MethodPost,
			URL:    &url.URL{Path: job.PathGroupJobs},
			Header: http.Header{
				"Content-Type":            []string{"application/json"},
				constants.HeaderTenantID:  []string{"123456"},
				constants.HeaderPoolLabel: []string{"abc"},
			},
			Body: io.NopCloser(reader), // 使用 io.NopCloser 包装 reader，使其满足 io.ReadCloser 接口
		}
		convey.Convey("when process success", func() {
			defer gomonkey.ApplyMethodFunc(mock, "CreateInstance",
				func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (instanceID string, err error) {
					return submissionId, nil
				}).Reset()
			SubmitJobHandler(c)
			expectedResult, err := json.Marshal(map[string]string{
				"submission_id": submissionId,
			})
			if err != nil {
				t.Errorf("marshal expected result failed, err: %v", err)
			}
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldResemble, string(expectedResult))
		})
		convey.Convey("when app is exist", func() {
			storeDefaultApp(submissionId, 3, 0)
			defer gomonkey.ApplyMethodFunc(mock, "CreateInstance",
				func(funcMeta api.FunctionMeta, args []api.Arg, invokeOpt api.InvokeOptions) (instanceID string, err error) {
					return submissionId, nil
				}).Reset()
			SubmitJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusBadRequest)
			convey.So(rw.Body.String(), convey.ShouldStartWith, "\"submit job has already exist, submissionId")
		})
	})
}

func TestGetJobInfoHandler(t *testing.T) {
	convey.Convey("test GetJobInfoHandler", t, func() {
		submissionId := "app-frontend-get1"
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rw)
		c.Request = &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Path: job.PathGroupJobs + "/" + submissionId},
			Header: http.Header{
				"Content-Type":            []string{"application/json"},
				constants.HeaderTenantID:  []string{"123456"},
				constants.HeaderPoolLabel: []string{"abc"},
			},
			Body: nil, // 使用 io.NopCloser 包装 reader，使其满足 io.ReadCloser 接口
		}
		c.AddParam(job.PathParamSubmissionId, submissionId)
		convey.Convey("when get job is not found", func() {
			GetJobInfoHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusNotFound)
			convey.So(rw.Body.String(), convey.ShouldStartWith, "\"not found app, submissionId")
		})
		convey.Convey("when process success", func() {
			storeDefaultApp(submissionId, 3, 0)
			GetJobInfoHandler(c)
			expectedResult, err := json.Marshal(buildAppInfo(submissionId))
			if err != nil {
				t.Errorf("marshal expected result failed, err: %v", err)
			}
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, string(expectedResult))
		})
	})
}

func TestDeleteJobHandler(t *testing.T) {
	convey.Convey("test DeleteJobHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		submissionId := "app-frontend-delete1"
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rw)
		c.Request = &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Path: job.PathGroupJobs + "/" + submissionId},
			Header: http.Header{
				"Content-Type":            []string{"application/json"},
				constants.HeaderTenantID:  []string{"123456"},
				constants.HeaderPoolLabel: []string{"abc"},
			},
			Body: nil, // 使用 io.NopCloser 包装 reader，使其满足 io.ReadCloser 接口
		}
		c.AddParam(job.PathParamSubmissionId, submissionId)
		convey.Convey("when job is not found", func() {
			DeleteJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusNotFound)
			convey.So(rw.Body.String(), convey.ShouldStartWith, "\"the app does not exist, submissionId")
		})
		convey.Convey("when process success", func() {
			storeDefaultApp(submissionId, 6, 1)
			DeleteJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "true")
		})
		convey.Convey("when job is forbidden to delete, status: SUCCEEDED", func() {
			storeDefaultApp(submissionId, 3, 1)
			DeleteJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "false")
		})
		convey.Convey("when job is forbidden to delete, status: STOPPED", func() {
			storeDefaultApp(submissionId, 3, 6)
			DeleteJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "false")
		})
		convey.Convey("when job is forbidden to delete, status: FAILED", func() {
			storeDefaultApp(submissionId, 3, 0)
			DeleteJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "false")
		})
		convey.Convey("when statusCode is "+strconv.Itoa(http.StatusInternalServerError), func() {
			defer gomonkey.ApplyMethodFunc(mock, "Kill",
				func(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error {
					return errors.New("failed delete app")
				}).Reset()
			storeDefaultApp(submissionId, 6, 1)
			DeleteJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusInternalServerError)
			convey.So(rw.Body.String(), convey.ShouldStartWith, "\"delete app failed")
		})
	})
}

func TestStopJobHandler(t *testing.T) {
	convey.Convey("test StopJobHandler", t, func() {
		mock := &mockUtils.FakeLibruntimeSdkClient{}
		util.SetAPIClientLibruntime(mock)
		submissionId := "app-frontend-stop1"
		gin.SetMode(gin.TestMode)
		rw := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rw)
		c.Request = &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Path: job.PathGroupJobs + "/" + submissionId + "/stop"},
			Header: http.Header{
				"Content-Type":            []string{"application/json"},
				constants.HeaderTenantID:  []string{"123456"},
				constants.HeaderPoolLabel: []string{"abc"},
			},
			Body: nil, // 使用 io.NopCloser 包装 reader，使其满足 io.ReadCloser 接口
		}
		c.AddParam(job.PathParamSubmissionId, submissionId)
		convey.Convey("when job is not found", func() {
			StopJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusNotFound)
			convey.So(rw.Body.String(), convey.ShouldStartWith, "\"the app does not exist, submissionId")
		})
		convey.Convey("when process success", func() {
			storeDefaultApp(submissionId, 3, 1)
			StopJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "true")
		})
		convey.Convey("when job is forbidden to stop, status: RUNNING", func() {
			storeDefaultApp(submissionId, 6, 1)
			StopJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "false")
		})
		convey.Convey("when statusCode is "+strconv.Itoa(http.StatusInternalServerError), func() {
			defer gomonkey.ApplyMethodFunc(mock, "Kill",
				func(instanceID string, signal int, payload []byte, invokeOpt api.InvokeOptions) error {
					return errors.New("failed stop app")
				}).Reset()
			storeDefaultApp(submissionId, 3, 1)
			StopJobHandler(c)
			convey.So(rw.Code, convey.ShouldEqual, http.StatusOK)
			convey.So(rw.Body.String(), convey.ShouldEqual, "true")
		})
	})
}
