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

package webui

import (
	"fmt"
	"net/http"
)

// HandleAPIDoc displays the API documentation page
func HandleAPIDoc(w http.ResponseWriter, r *http.Request) {
	// Get path prefix from X-Forwarded-Prefix header
	pathPrefix := r.Header.Get("X-Forwarded-Prefix")

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>API Documentation - YuanRong</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: #f5f5f5;
            min-height: 100vh;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 24px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .header h1 {
            font-size: 32px;
            font-weight: 600;
            margin-bottom: 8px;
        }
        .header .subtitle {
            font-size: 16px;
            opacity: 0.9;
        }
        .back-link {
            display: inline-block;
            color: white;
            text-decoration: none;
            margin-bottom: 16px;
            opacity: 0.9;
            transition: opacity 0.2s;
        }
        .back-link:hover {
            opacity: 1;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 32px 24px;
        }
        .intro-section {
            background: white;
            border-radius: 8px;
            padding: 24px;
            margin-bottom: 24px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .intro-section h2 {
            font-size: 20px;
            color: #2d3748;
            margin-bottom: 16px;
        }
        .intro-section p {
            line-height: 1.6;
            color: #4a5568;
            margin-bottom: 12px;
        }
        .auth-info {
            background: #fff5e6;
            border-left: 4px solid #f59e0b;
            padding: 16px;
            border-radius: 4px;
            margin-top: 16px;
        }
        .auth-info strong {
            color: #92400e;
        }
        .api-section {
            background: white;
            border-radius: 8px;
            padding: 24px;
            margin-bottom: 24px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .api-section h2 {
            font-size: 24px;
            color: #2d3748;
            margin-bottom: 20px;
            padding-bottom: 12px;
            border-bottom: 2px solid #e2e8f0;
        }
        .api-group {
            margin-bottom: 32px;
        }
        .api-group h3 {
            font-size: 18px;
            color: #4a5568;
            margin-bottom: 16px;
            display: flex;
            align-items: center;
        }
        .api-group h3::before {
            content: '📁';
            margin-right: 8px;
        }
        .api-endpoint {
            border: 1px solid #e2e8f0;
            border-radius: 6px;
            margin-bottom: 16px;
            overflow: hidden;
        }
        .api-header {
            background: #f8f9fa;
            padding: 12px 16px;
            display: flex;
            align-items: center;
            cursor: pointer;
            transition: background 0.2s;
        }
        .api-header:hover {
            background: #e9ecef;
        }
        .method {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 4px;
            font-weight: 600;
            font-size: 12px;
            margin-right: 12px;
            min-width: 60px;
            text-align: center;
        }
        .method.get { background: #10b981; color: white; }
        .method.post { background: #3b82f6; color: white; }
        .method.put { background: #f59e0b; color: white; }
        .method.delete { background: #ef4444; color: white; }
        .endpoint-path {
            font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
            font-size: 14px;
            color: #2d3748;
            flex: 1;
        }
        .api-details {
            padding: 16px;
            border-top: 1px solid #e2e8f0;
            display: none;
        }
        .api-endpoint.expanded .api-details {
            display: block;
        }
        .api-description {
            color: #4a5568;
            margin-bottom: 16px;
            line-height: 1.6;
        }
        .param-section {
            margin-top: 16px;
        }
        .param-section h4 {
            font-size: 14px;
            color: #2d3748;
            margin-bottom: 8px;
            font-weight: 600;
        }
        .param-table {
            width: 100%%;
            border-collapse: collapse;
            font-size: 13px;
        }
        .param-table th {
            background: #f8f9fa;
            padding: 8px 12px;
            text-align: left;
            font-weight: 600;
            color: #4a5568;
            border-bottom: 2px solid #e2e8f0;
        }
        .param-table td {
            padding: 8px 12px;
            border-bottom: 1px solid #e2e8f0;
            color: #4a5568;
        }
        .param-table code {
            background: #f1f5f9;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
            font-size: 12px;
        }
        .example-section {
            margin-top: 16px;
        }
        .example-section h4 {
            font-size: 14px;
            color: #2d3748;
            margin-bottom: 8px;
            font-weight: 600;
        }
        .code-block {
            background: #1e1e1e;
            color: #d4d4d4;
            padding: 16px;
            border-radius: 6px;
            overflow-x: auto;
            font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
            font-size: 13px;
            line-height: 1.5;
        }
        .expand-icon {
            margin-left: auto;
            color: #718096;
            transition: transform 0.2s;
        }
        .api-endpoint.expanded .expand-icon {
            transform: rotate(90deg);
        }
    </style>
</head>
<body>
    <div class="header">
        <a href="%s/" class="back-link">← 返回首页</a>
        <h1>📚 API Documentation</h1>
        <div class="subtitle">YuanRong Serverless Platform REST API 参考文档</div>
    </div>
    
    <div class="container">
        <div class="intro-section">
            <h2>概述</h2>
            <p>YuanRong Frontend 提供了一套完整的 RESTful API，用于管理和调用 Serverless 函数、容器实例、数据存储等资源。</p>
            <p>所有 API 端点都支持标准的 HTTP 方法（GET、POST、PUT、DELETE），请求和响应均为 JSON 格式。</p>
            
            <div class="auth-info">
                <strong>⚠️ 认证要求：</strong> 大多数 API 端点需要 JWT Token 认证。请在请求头中包含 <code>X-Auth: YOUR_JWT_TOKEN</code> 或通过 URL 参数 <code>?token=YOUR_JWT_TOKEN</code> 传递。
            </div>
        </div>

        <div class="api-section">
            <h2>API 端点</h2>

            <!-- Function APIs -->
            <div class="api-group">
                <h3>函数管理</h3>
                
                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v1/functions/:urn/invocations</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            调用指定的 Serverless 函数。URN 格式为 <code>urn:tenant:namespace:function</code>。
                        </div>
                        <div class="param-section">
                            <h4>路径参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>urn</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>函数的 URN 标识符</td>
                                </tr>
                            </table>
                        </div>
                        <div class="param-section">
                            <h4>请求体</h4>
                            <table class="param-table">
                                <tr>
                                    <th>格式</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td>JSON</td>
                                    <td>传递给函数的参数，格式由函数定义决定</td>
                                </tr>
                            </table>
                        </div>
                        <div class="example-section">
                            <h4>请求示例</h4>
                            <pre class="code-block">curl -X POST "%s/serverless/v1/functions/urn:tenant_001:default:hello/invocations" \
  -H "Content-Type: application/json" \
  -H "X-Auth: YOUR_JWT_TOKEN" \
  -d '{"name": "World"}'</pre>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/:tenant-id/:namespace/:function/</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            简短路径调用函数。这是一个更简洁的调用方式。
                        </div>
                        <div class="param-section">
                            <h4>路径参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>tenant-id</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>租户 ID</td>
                                </tr>
                                <tr>
                                    <td><code>namespace</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>命名空间</td>
                                </tr>
                                <tr>
                                    <td><code>function</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>函数名称</td>
                                </tr>
                            </table>
                        </div>
                        <div class="example-section">
                            <h4>请求示例</h4>
                            <pre class="code-block">curl -X POST "%s/tenant_001/default/hello/" \
  -H "Content-Type: application/json" \
  -H "X-Auth: YOUR_JWT_TOKEN" \
  -d '{"name": "World"}'</pre>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method get">GET</span>
                        <span class="endpoint-path">/serverless/v1/stream/subscribe</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            订阅函数的流式输出。建立 SSE (Server-Sent Events) 连接以接收实时数据。
                        </div>
                        <div class="example-section">
                            <h4>请求示例</h4>
                            <pre class="code-block">curl -X GET "%s/serverless/v1/stream/subscribe?function=hello" \
  -H "X-Auth: YOUR_JWT_TOKEN"</pre>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Instance APIs -->
            <div class="api-group">
                <h3>实例管理</h3>
                
                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v1/posix/instance/create</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            创建一个新的容器实例。
                        </div>
                        <div class="param-section">
                            <h4>请求体参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>image</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>容器镜像名称</td>
                                </tr>
                                <tr>
                                    <td><code>tenant_id</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>租户 ID</td>
                                </tr>
                                <tr>
                                    <td><code>namespace</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>命名空间</td>
                                </tr>
                            </table>
                        </div>
                        <div class="example-section">
                            <h4>请求示例</h4>
                            <pre class="code-block">curl -X POST "%s/serverless/v1/posix/instance/create" \
  -H "Content-Type: application/json" \
  -H "X-Auth: YOUR_JWT_TOKEN" \
  -d '{
    "image": "ubuntu:20.04",
    "tenant_id": "tenant_001",
    "namespace": "default"
  }'</pre>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v1/posix/instance/invoke</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            在指定实例中执行命令。
                        </div>
                        <div class="param-section">
                            <h4>请求体参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>instance_id</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>实例 ID</td>
                                </tr>
                                <tr>
                                    <td><code>command</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>要执行的命令</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v1/posix/instance/kill</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            终止指定的实例。
                        </div>
                        <div class="param-section">
                            <h4>请求体参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>instance_id</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>要终止的实例 ID</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method get">GET</span>
                        <span class="endpoint-path">/api/instances</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            列出当前租户下的所有实例。
                        </div>
                        <div class="param-section">
                            <h4>查询参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>tenant_id</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>租户 ID</td>
                                </tr>
                            </table>
                        </div>
                        <div class="example-section">
                            <h4>请求示例</h4>
                            <pre class="code-block">curl -X GET "%s/api/instances?tenant_id=tenant_001" \
  -H "X-Auth: YOUR_JWT_TOKEN"</pre>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Data System APIs -->
            <div class="api-group">
                <h3>数据系统</h3>
                
                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/datasystem/v1/kv/set</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            设置键值对数据。
                        </div>
                        <div class="param-section">
                            <h4>请求体参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>key</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>键名</td>
                                </tr>
                                <tr>
                                    <td><code>value</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>值</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/datasystem/v1/kv/get</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            获取键值对数据。
                        </div>
                        <div class="param-section">
                            <h4>请求体参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>key</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>键名</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/datasystem/v1/kv/del</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            删除键值对数据。
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v2/data/kv/multiset</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            批量设置多个键值对。
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v2/data/kv/multiget</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            批量获取多个键值对。
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/serverless/v2/data/kv/multidel</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            批量删除多个键值对。
                        </div>
                    </div>
                </div>
            </div>

            <!-- Job APIs -->
            <div class="api-group">
                <h3>作业管理</h3>
                
                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/jobs</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            提交一个新的作业。
                        </div>
                        <div class="param-section">
                            <h4>请求体参数</h4>
                            <table class="param-table">
                                <tr>
                                    <th>参数名</th>
                                    <th>类型</th>
                                    <th>必需</th>
                                    <th>描述</th>
                                </tr>
                                <tr>
                                    <td><code>job_name</code></td>
                                    <td>string</td>
                                    <td>是</td>
                                    <td>作业名称</td>
                                </tr>
                                <tr>
                                    <td><code>job_spec</code></td>
                                    <td>object</td>
                                    <td>是</td>
                                    <td>作业规格定义</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method get">GET</span>
                        <span class="endpoint-path">/jobs</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            列出所有作业。
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method get">GET</span>
                        <span class="endpoint-path">/jobs/:id</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            获取指定作业的详细信息。
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method delete">DELETE</span>
                        <span class="endpoint-path">/jobs/:id</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            删除指定的作业。
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method post">POST</span>
                        <span class="endpoint-path">/jobs/:id/stop</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            停止正在运行的作业。
                        </div>
                    </div>
                </div>
            </div>

            <!-- Health Check -->
            <div class="api-group">
                <h3>系统监控</h3>
                
                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method get">GET</span>
                        <span class="endpoint-path">/healthz</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            健康检查端点。返回服务状态。
                        </div>
                        <div class="example-section">
                            <h4>响应示例</h4>
                            <pre class="code-block">{
  "status": "ok",
  "timestamp": "2026-02-13T10:30:00Z"
}</pre>
                        </div>
                    </div>
                </div>

                <div class="api-endpoint" onclick="toggleDetails(this)">
                    <div class="api-header">
                        <span class="method get">GET</span>
                        <span class="endpoint-path">/serverless/v1/componentshealth</span>
                        <span class="expand-icon">▶</span>
                    </div>
                    <div class="api-details">
                        <div class="api-description">
                            集群组件健康状态检查。返回所有组件的健康状态。
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <div class="intro-section">
            <h2>错误处理</h2>
            <p>所有 API 在发生错误时都会返回标准的 HTTP 状态码和 JSON 错误响应：</p>
            <pre class="code-block">{
  "error": "错误描述信息",
  "code": "ERROR_CODE",
  "details": "详细错误信息"
}</pre>
            <p><strong>常见状态码：</strong></p>
            <ul style="margin-left: 20px; margin-top: 12px; line-height: 1.8;">
                <li><strong>200 OK</strong> - 请求成功</li>
                <li><strong>400 Bad Request</strong> - 请求参数错误</li>
                <li><strong>401 Unauthorized</strong> - 未授权，需要有效的 Token</li>
                <li><strong>403 Forbidden</strong> - 禁止访问</li>
                <li><strong>404 Not Found</strong> - 资源不存在</li>
                <li><strong>500 Internal Server Error</strong> - 服务器内部错误</li>
            </ul>
        </div>
    </div>

    <script>
        function toggleDetails(element) {
            element.classList.toggle('expanded');
        }
    </script>
</body>
</html>`, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix, pathPrefix)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
