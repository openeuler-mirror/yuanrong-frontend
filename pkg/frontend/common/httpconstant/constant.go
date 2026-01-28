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

// Package httpconstant -
package httpconstant

const (
	// SemicolonReplacer replaces ";" to other character to solve golang.org/issue/25192
	SemicolonReplacer = "#"
)

// Http request constant for http trigger
const (
	// HeaderInvokeURN  -
	HeaderInvokeURN = "X-Tag-VersionUrn"
	// HeaderCPUSize is cpu size specified by invoke
	HeaderCPUSize = "X-Instance-Cpu"
	// HeaderMemorySize is cpu memory specified by invoke
	HeaderMemorySize = "X-Instance-Memory"
	// HeaderPoolLabel is pool label
	HeaderPoolLabel = "X-Pool-Label"
	// HeaderInvokeTag -
	HeaderInvokeTag = "X-Invoke-Tag"
	// HeaderInstanceLabel -
	HeaderInstanceLabel = "X-Instance-Label"
	// HeaderContentType -
	HeaderContentType = "Content-Type"
	// HeaderBillingDuration -
	HeaderBillingDuration = "X-Billing-Duration"
	// HeaderInnerCode -
	HeaderInnerCode = "X-Inner-Code"
	// HeaderInvokeSummary -
	HeaderInvokeSummary = "X-Invoke-Summary"
	// HeaderLogResult -
	HeaderLogResult = "X-Log-Result"
	// HeaderLogType -
	HeaderLogType = "X-Log-Type"
	// DefaultLogFlag is the default flag for log
	DefaultLogFlag = "None"
	// HeaderAuthTimestamp is the timestamp for authorization
	HeaderAuthTimestamp = "X-Timestamp-Auth"
	// HeaderAuthorization is authorization
	HeaderAuthorization = "authorization"
	// AppID name of module
	AppID = "frontendinvoke"
	// HeaderInvokeAlias indicates alias of current invocation
	HeaderInvokeAlias = "x-invoke-alias"
	// HeaderRetryFlag -
	HeaderRetryFlag = "X-Retry-Flag"
	// HeaderWorkerCost -
	HeaderWorkerCost = "X-Worker-Cost"
	// HeaderCallInstance -
	HeaderCallInstance = "X-Call-Instance"
	// HeaderCallNode -
	HeaderCallNode = "X-Call-Node"
	// AuthType -
	AuthType = "X-Authorization-Type"
	// AuthHeader -
	AuthHeader = "Authorization"
	// SAAuthHeader -
	SAAuthHeader = "service-account"
	// HeaderInstanceSession -
	HeaderInstanceSession = "X-Instance-Session"
)

const (
	// ContentTypeHeaderKey -
	ContentTypeHeaderKey = "Content-Type"
	// ApplicationJSON -
	ApplicationJSON = "application/json"
	// StreamContentType -
	StreamContentType = "application/octet-stream"
	// FormContentType -
	FormContentType = "application/x-www-form-urlencoded"
	// MultipartFormContentType -
	MultipartFormContentType = "multipart/form-data"
	// AcceptEventStream Accept:text/event-stream
	AcceptEventStream = "text/event-stream"

	// ContentType -
	ContentType = "Content-Type"
)

const (
	// DefaultGraphReadBufferSize In FunctionGraph mode, the size of this message needs to be set to 32 KB.
	DefaultGraphReadBufferSize = 32 * 1024
)

const (
	// HeaderLuBanNTraceID -
	HeaderLuBanNTraceID = "lubanops-ntrace-id"
	// HeaderLuBanGTraceID -
	HeaderLuBanGTraceID = "lubanops-gtrace-id"
	// HeaderLuBanSpanID -
	HeaderLuBanSpanID = "lubanops-nspan-id"
	// HeaderLuBanEvnID -
	HeaderLuBanEvnID = "lubanops-nenv-id"
	// HeaderLuBanEventID -
	HeaderLuBanEventID = "lubanops-sevent-id"
	// HeaderLuBanDomainID -
	HeaderLuBanDomainID = "lubanops-ndomain-id"
)

const (
	// CrossHeaderKeyCrossCluster -
	CrossHeaderKeyCrossCluster = "X-System-Cross-Cluster"
	// CrossHeaderKeyClusterID -
	CrossHeaderKeyClusterID = "X-System-Cluster-Id"
	// CrossHeaderKeyTimestamp -
	CrossHeaderKeyTimestamp = "X-System-Timestamp"
	// CrossHeaderKeySignature -
	CrossHeaderKeySignature = "X-System-Signature"
)

const (
	// HeaderStreamName -
	HeaderStreamName = "X-Stream-Name"
	// HeaderExpectNum -
	HeaderExpectNum = "X-Expect-Num"
	// HeaderTimeoutMs -
	HeaderTimeoutMs = "X-Timeout-Ms"
)
