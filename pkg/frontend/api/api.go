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

// Package api wraps different api versions, and can be easily switched between different versions
// API provides http handlers used by fast-http, the handlers should only do http context checking and should dispatch
// the actual logic to
package api

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"frontend/pkg/common/constants"
	"frontend/pkg/common/faas_common/tracer"
	commonJob "frontend/pkg/common/job"
	"frontend/pkg/frontend/api/app"
	"frontend/pkg/frontend/api/datasystem"
	frontend "frontend/pkg/frontend/api/functionsystem"
	"frontend/pkg/frontend/api/job"
	"frontend/pkg/frontend/api/lease"
	v1 "frontend/pkg/frontend/api/v1"
	"frontend/pkg/frontend/common"
	"frontend/pkg/frontend/frontendsdkadapter/handler"
	"frontend/pkg/frontend/middleware"
	"frontend/pkg/frontend/webterm"
)

const (
	// naming convention: url + method + description
	urlPostInvoke = "/serverless/v1/functions/" + common.GinUrnParamMark +
		common.FunctionUrnParam + "/invocations"
	urlShortInvoke     = "/:tenant-id/:namespace/:function/"
	urlStreamSubscribe = "/serverless/v1/stream/subscribe"
	urlGetHealthCheck  = "/healthz"
	urlClusterHealthy  = "/serverless/v1/componentshealth"
	// url to faasmanager
	urlLease          = "/client/v1/lease"
	urlLeaseKeepAlive = "/client/v1/lease/keepalive"
	// url to frontend
	urlPreCreate = "/serverless/v1/posix/instance/create"
	urlPreInvoke = "/serverless/v1/posix/instance/invoke"
	urlPreKill   = "/serverless/v1/posix/instance/kill"
	urlCreate    = "/frontend/v1/instance/create"
	urlInvoke    = "/frontend/v1/instance/invoke"
	urlKill      = "/frontend/v1/instance/kill"
	// url to datasystem
	urlPut         = "/datasystem/v1/obj/put"
	urlGet         = "/datasystem/v1/obj/get"
	urlIncreaseRef = "/datasystem/v1/obj/increaseref"
	urlDecreaseRef = "/datasystem/v1/obj/decreaseref"
	urlKvSet       = "/datasystem/v1/kv/set"
	urlKvGet       = "/datasystem/v1/kv/get"
	urlKvDel       = "/datasystem/v1/kv/del"
	urlKvMSetTx    = "/datasystem/v1/kv/msettx"
	urlUpload      = "/serverless/v2/data/kv/multiset"
	urlDownload    = "/serverless/v2/data/kv/multiget"
	urlDelete      = "/serverless/v2/data/kv/multidel"
	urlExecute     = "/serverless/v2/aggregation/execute"
	// url to app
	urlGroupApp   = "/app/v1"
	urlCreateApp  = "/posix/instance/create"
	urlListApp    = "/list"
	urlGetAppInfo = "/getappinfo" +
		constants.DynamicRouterParamPrefix + commonJob.PathParamSubmissionId
	urlStopApp = "/posix/kill" +
		constants.DynamicRouterParamPrefix + commonJob.PathParamSubmissionId
	urlDeleteApp = "/delete" +
		constants.DynamicRouterParamPrefix + commonJob.PathParamSubmissionId
)

// InitRoute -
func InitRoute(r *gin.Engine) {
	// Apply invoke preprocessing middleware to:
	// 1. Mark invoke URLs for role-based authentication
	// 2. Detect public functions and skip JWT authentication
	r.Use(middleware.InvokePreprocessMiddleware())

	// Apply global JWT authentication middleware with whitelist support
	// For invoke URLs: allow RoleUser and RoleDeveloper
	// For other URLs: only allow RoleDeveloper
	r.Use(middleware.GlobalJWTAuthMiddleware())

	r.GET(urlGetHealthCheck, v1.HealthzHandler)
	r.GET(urlClusterHealthy, v1.ClusterHealthHandler)                    // Health check
	r.POST(urlPostInvoke, tracer.WrapGinHandler(v1.InvokeHandler))       // Invocation
	r.POST(urlShortInvoke, tracer.WrapGinHandler(v1.ShortInvokeHandler)) // Invocation
	r.GET(urlStreamSubscribe, v1.SubscribeHandler)                       // Subscribe Stream
	r.PUT(urlLease, lease.NewLeaseHandler)
	r.DELETE(urlLease, lease.DelLeaseHandler)
	r.POST(urlLeaseKeepAlive, lease.KeepAliveHandler)
	r.POST(urlPreCreate, frontend.CreateHandler)
	r.POST(urlPreInvoke, frontend.InvokeHandler)
	r.POST(urlPreKill, frontend.KillHandler)
	r.POST(urlCreate, frontend.CreateHandler)
	r.POST(urlInvoke, frontend.InvokeHandler)
	r.POST(urlKill, frontend.KillHandler)
	r.POST(urlPut, datasystem.PutHandler)
	r.POST(urlGet, datasystem.GetHandler)
	r.POST(urlIncreaseRef, datasystem.IncreaseRefHandler)
	r.POST(urlDecreaseRef, datasystem.DecreaseRefHandler)
	r.POST(urlKvSet, datasystem.KvSetHandler)
	r.POST(urlKvMSetTx, datasystem.KvMSetTxHandler)
	r.POST(urlKvGet, datasystem.KvGetHandler)
	r.POST(urlKvDel, datasystem.KvDelHandler)
	r.NoRoute(v1.ProxyHandler)
	r.POST(urlUpload, handler.MultiSetHandler)
	r.POST(urlDownload, handler.MultiGetHandler)
	r.POST(urlDelete, handler.MultiDelHandler)
	r.POST(urlExecute, handler.ExecuteHandler)
	// app 外部请求经过dashboard，再请求到frontend，处理job
	appGroup := r.Group(urlGroupApp)
	{
		appGroup.POST(urlCreateApp, app.CreateHandler)
		appGroup.GET(urlListApp, app.ListHandler)
		appGroup.GET(urlGetAppInfo, app.GetInfoHandler)
		appGroup.DELETE(urlDeleteApp, app.DeleteHandler)
		appGroup.POST(urlStopApp, app.StopHandler)
	}
	// job 外部请求直接访问frontend，处理job
	jobGroup := r.Group(commonJob.PathGroupJobs)
	{
		jobGroup.POST("", job.SubmitJobHandler)
		jobGroup.GET("", job.ListJobsHandler)
		jobGroup.GET(commonJob.PathGetJobs, job.GetJobInfoHandler)
		jobGroup.DELETE(commonJob.PathDeleteJobs, job.DeleteJobHandler)
		jobGroup.POST(commonJob.PathStopJobs, job.StopJobHandler)
	}

	// web terminal
	terminalGroup := r.Group("/terminal")
	{
		terminalGroup.GET("", gin.WrapF(webterm.HandleIndex))
		terminalGroup.GET("/ws", gin.WrapF(webterm.HandleWebSocket))
		staticFS, _ := fs.Sub(webterm.StaticFiles, "static")
		terminalGroup.GET("/static/*filepath", gin.WrapH(http.StripPrefix("/terminal/static", http.FileServer(http.FS(staticFS)))))
	}
	r.GET("api/instances", gin.WrapF(webterm.HandleInstances))

	// Function invoke tool (requires authentication)
	r.GET("/functions", gin.WrapF(webterm.HandleInvokePage))

	// API documentation page (no authentication required)
	r.GET("/api-docs", gin.WrapF(webterm.HandleAPIDoc))

	// Welcome/introduction page (no authentication required)
	r.GET("/", gin.WrapF(webterm.HandleWelcome))
}
