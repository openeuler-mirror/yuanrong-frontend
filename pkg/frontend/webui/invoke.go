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

// HandleInvokePage displays the function invocation debug page
func HandleInvokePage(w http.ResponseWriter, r *http.Request) {
	// Get path prefix from X-Forwarded-Prefix header
	pathPrefix := r.Header.Get("X-Forwarded-Prefix")

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Function Invoke - YuanRong</title>
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
            height: 100vh;
            display: flex;
            flex-direction: column;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 16px 24px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .back-link {
            display: inline-block;
            color: white;
            text-decoration: none;
            margin-bottom: 8px;
            opacity: 0.9;
            transition: opacity 0.2s;
            font-size: 14px;
        }
        .back-link:hover {
            opacity: 1;
        }
        .header h1 {
            font-size: 24px;
            font-weight: 600;
        }
        .header .subtitle {
            font-size: 14px;
            opacity: 0.9;
            margin-top: 4px;
        }
        .container {
            flex: 1;
            display: flex;
            overflow: hidden;
        }
        .panel {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: white;
            margin: 16px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .panel-header {
            padding: 16px 20px;
            background: #f8f9fa;
            border-bottom: 1px solid #e9ecef;
            font-weight: 600;
            color: #2d3748;
        }
        .panel-body {
            flex: 1;
            padding: 20px;
            overflow-y: auto;
        }
        .form-group {
            margin-bottom: 20px;
        }
        .form-label {
            display: block;
            margin-bottom: 8px;
            font-weight: 500;
            color: #4a5568;
            font-size: 14px;
        }
        .form-input {
            width: 100%%;
            padding: 10px 12px;
            border: 1px solid #cbd5e0;
            border-radius: 6px;
            font-size: 14px;
            font-family: inherit;
            transition: border-color 0.2s;
        }
        .form-input:focus {
            outline: none;
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }
        .form-textarea {
            min-height: 200px;
            font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
            resize: vertical;
        }
        .btn-group {
            display: flex;
            gap: 12px;
            margin-top: 24px;
        }
        .btn {
            padding: 10px 24px;
            border: none;
            border-radius: 6px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
        }
        .btn-primary {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
        }
        .btn-primary:hover:not(:disabled) {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }
        .btn-primary:disabled {
            opacity: 0.6;
            cursor: not-allowed;
        }
        .btn-secondary {
            background: #e2e8f0;
            color: #4a5568;
        }
        .btn-secondary:hover {
            background: #cbd5e0;
        }
        .response-area {
            background: #1e1e1e;
            color: #d4d4d4;
            padding: 16px;
            border-radius: 6px;
            font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
            font-size: 13px;
            overflow-x: auto;
            min-height: 300px;
            white-space: pre-wrap;
            word-break: break-all;
        }
        .response-meta {
            display: flex;
            gap: 24px;
            padding: 12px 16px;
            background: #f8f9fa;
            border-radius: 6px;
            margin-bottom: 16px;
            font-size: 13px;
        }
        .meta-item {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .meta-label {
            color: #718096;
            font-weight: 500;
        }
        .meta-value {
            color: #2d3748;
            font-weight: 600;
        }
        .status-200 { color: #48bb78; }
        .status-400 { color: #ed8936; }
        .status-500 { color: #f56565; }
        .empty-state {
            text-align: center;
            color: #a0aec0;
            padding: 60px 20px;
        }
        .empty-state-icon {
            font-size: 48px;
            margin-bottom: 16px;
        }
        .history-section {
            margin-top: 24px;
            padding-top: 24px;
            border-top: 1px solid #e9ecef;
        }
        .history-item {
            padding: 12px;
            background: #f8f9fa;
            border-radius: 6px;
            margin-bottom: 8px;
            cursor: pointer;
            transition: background 0.2s;
            font-size: 13px;
        }
        .history-item:hover {
            background: #e9ecef;
        }
        .history-path {
            font-weight: 600;
            color: #2d3748;
            margin-bottom: 4px;
        }
        .history-time {
            color: #718096;
            font-size: 12px;
        }
    </style>
</head>
<body>
    <div class="header">
        <a href="%s/" class="back-link">← Back to Home</a>
        <h1>🚀 Function Invoke Tool</h1>
        <div class="subtitle">Developer Debug Tool - Quickly test and debug Serverless functions</div>
    </div>
    
    <div class="container">
        <!-- Request Panel -->
        <div class="panel">
            <div class="panel-header">Request Config</div>
            <div class="panel-body">
                <div class="form-group">
                    <label class="form-label">Tenant ID *</label>
                    <input type="text" class="form-input" id="tenantId" placeholder="e.g. tenant_001" value="default">
                </div>
                
                <div class="form-group">
                    <label class="form-label">Namespace *</label>
                    <input type="text" class="form-input" id="namespace" placeholder="e.g. default" value="default">
                </div>
                
                <div class="form-group">
                    <label class="form-label">Function Name *</label>
                    <input type="text" class="form-input" id="functionName" placeholder="e.g. hello-world">
                </div>
                
                <div class="form-group">
                    <label class="form-label">Request Body (JSON)</label>
                    <textarea class="form-input form-textarea" id="requestBody" placeholder='{"key": "value"}'>{}</textarea>
                </div>
                
                <div class="btn-group">
                    <button class="btn btn-primary" id="invokeBtn" onclick="invokeFunction()">
                        <span id="invokeBtnText">Invoke</span>
                    </button>
                    <button class="btn btn-secondary" onclick="clearForm()">Clear</button>
                    <button class="btn btn-secondary" onclick="formatJSON()">Format JSON</button>
                </div>
                
                <div class="history-section" id="historySection" style="display: none;">
                    <div class="form-label">Call History</div>
                    <div id="historyList"></div>
                </div>
            </div>
        </div>
        
        <!-- Response Panel -->
        <div class="panel">
            <div class="panel-header">Response</div>
            <div class="panel-body">
                <div id="responseMetaContainer" style="display: none;">
                    <div class="response-meta">
                        <div class="meta-item">
                            <span class="meta-label">Status:</span>
                            <span class="meta-value" id="responseStatus">-</span>
                        </div>
                        <div class="meta-item">
                            <span class="meta-label">Duration:</span>
                            <span class="meta-value" id="responseDuration">-</span>
                        </div>
                        <div class="meta-item">
                            <span class="meta-label">Size:</span>
                            <span class="meta-value" id="responseSize">-</span>
                        </div>
                    </div>
                </div>
                
                <div id="responseContainer">
                    <div class="empty-state">
                        <div class="empty-state-icon">📬</div>
                        <div>No response data</div>
                        <div style="margin-top: 8px; font-size: 13px;">Configure request params and click "Invoke"</div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    
    <script>
        const pathPrefix = '%s';
        
        // Get token from URL parameter
        const urlParams = new URLSearchParams(window.location.search);
        const token = urlParams.get('token') || '';
        
        // Load history on page load
        window.addEventListener('DOMContentLoaded', function() {
            loadHistory();
        });
        
        async function invokeFunction() {
            const tenantId = document.getElementById('tenantId').value.trim();
            const namespace = document.getElementById('namespace').value.trim();
            const functionName = document.getElementById('functionName').value.trim();
            const requestBody = document.getElementById('requestBody').value.trim();
            
            // Validation
            if (!tenantId || !namespace || !functionName) {
                alert('Please fill in Tenant ID, Namespace, and Function Name');
                return;
            }
            
            // Validate JSON
            let body = requestBody;
            if (body) {
                try {
                    JSON.parse(body);
                } catch (e) {
                    alert('Request body is not valid JSON: ' + e.message);
                    return;
                }
            }
            
            // Disable button
            const btn = document.getElementById('invokeBtn');
            const btnText = document.getElementById('invokeBtnText');
            btn.disabled = true;
            btnText.textContent = 'Invoking...';
            
            const startTime = Date.now();
            const url = pathPrefix + '/invocations/' + tenantId + '/' + namespace + '/' + functionName + '/';
            
            try {
                const headers = {
                    'Content-Type': 'application/json'
                };
                if (token) {
                    headers['X-Auth'] = token;
                }
                
                const response = await fetch(url, {
                    method: 'POST',
                    headers: headers,
                    body: body || '{}'
                });
                
                const duration = Date.now() - startTime;
                const responseText = await response.text();
                
                // Try to parse as JSON
                let responseData;
                let isJSON = false;
                try {
                    responseData = JSON.parse(responseText);
                    isJSON = true;
                } catch (e) {
                    responseData = responseText;
                }
                
                // Display response
                displayResponse(response.status, duration, responseData, isJSON);
                
                // Save to history
                saveToHistory({
                    tenantId,
                    namespace,
                    functionName,
                    requestBody: body,
                    timestamp: Date.now()
                });
                
            } catch (error) {
                const duration = Date.now() - startTime;
                displayResponse(0, duration, { error: error.message }, true);
            } finally {
                btn.disabled = false;
                btnText.textContent = 'Invoke';
            }
        }
        
        function displayResponse(status, duration, data, isJSON) {
            // Show meta container
            document.getElementById('responseMetaContainer').style.display = 'block';
            
            // Update meta info
            const statusEl = document.getElementById('responseStatus');
            statusEl.textContent = status || 'Error';
            statusEl.className = 'meta-value';
            if (status >= 200 && status < 300) {
                statusEl.classList.add('status-200');
            } else if (status >= 400 && status < 500) {
                statusEl.classList.add('status-400');
            } else if (status >= 500) {
                statusEl.classList.add('status-500');
            }
            
            document.getElementById('responseDuration').textContent = duration + ' ms';
            
            const dataStr = isJSON ? JSON.stringify(data, null, 2) : String(data);
            document.getElementById('responseSize').textContent = formatBytes(new Blob([dataStr]).size);
            
            // Display response body
            const container = document.getElementById('responseContainer');
            container.innerHTML = '<pre class="response-area">' + escapeHtml(dataStr) + '</pre>';
        }
        
        function clearForm() {
            document.getElementById('requestBody').value = '{}';
            document.getElementById('functionName').value = '';
        }
        
        function formatJSON() {
            const textarea = document.getElementById('requestBody');
            try {
                const json = JSON.parse(textarea.value);
                textarea.value = JSON.stringify(json, null, 2);
            } catch (e) {
                alert('Cannot format: ' + e.message);
            }
        }
        
        function saveToHistory(item) {
            const history = JSON.parse(localStorage.getItem('invokeHistory') || '[]');
            history.unshift(item);
            // Keep only last 10
            if (history.length > 10) {
                history.pop();
            }
            localStorage.setItem('invokeHistory', JSON.stringify(history));
            loadHistory();
        }
        
        function loadHistory() {
            const history = JSON.parse(localStorage.getItem('invokeHistory') || '[]');
            if (history.length === 0) {
                document.getElementById('historySection').style.display = 'none';
                return;
            }
            
            document.getElementById('historySection').style.display = 'block';
            const listEl = document.getElementById('historyList');
            listEl.innerHTML = history.map((item, index) => {
                const date = new Date(item.timestamp);
                return '<div class="history-item" onclick="loadFromHistory(' + index + ')">' +
                    '<div class="history-path">/' + escapeHtml(item.tenantId) + '/' + 
                    escapeHtml(item.namespace) + '/' + escapeHtml(item.functionName) + '/</div>' +
                    '<div class="history-time">' + date.toLocaleString() + '</div>' +
                    '</div>';
            }).join('');
        }
        
        function loadFromHistory(index) {
            const history = JSON.parse(localStorage.getItem('invokeHistory') || '[]');
            const item = history[index];
            if (!item) return;
            
            document.getElementById('tenantId').value = item.tenantId;
            document.getElementById('namespace').value = item.namespace;
            document.getElementById('functionName').value = item.functionName;
            document.getElementById('requestBody').value = item.requestBody || '{}';
        }
        
        function formatBytes(bytes) {
            if (bytes < 1024) return bytes + ' B';
            if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB';
            return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
        }
        
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        // Enter key to invoke
        document.addEventListener('keydown', function(e) {
            if (e.ctrlKey && e.key === 'Enter') {
                invokeFunction();
            }
        });
    </script>
</body>
</html>`, pathPrefix, pathPrefix)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
