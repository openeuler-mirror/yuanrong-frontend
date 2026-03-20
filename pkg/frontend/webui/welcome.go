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
	"net/url"

	"frontend/pkg/frontend/config"
)

// HandleWelcome displays the welcome/introduction page
func HandleWelcome(w http.ResponseWriter, r *http.Request) {
	pathPrefix := r.Header.Get("X-Forwarded-Prefix")
	iamConfig := config.GetConfig().IamConfig
	authNotice := ""
	if iamConfig.Addr == "" {
		authNotice = `<p class="auth-notice">IAM server is not configured. Set iamConfig.addr to enable login.</p>`
	}
	authButtons := fmt.Sprintf(`
	        <div class="auth-section">
	            <h3>🔐 Authentication</h3>
	            <div class="auth-group">
	                <a href="%s/auth/login?next=%s" class="cta-button">Sign In →</a>
	                <a href="%s/auth/register?next=%s" class="cta-button secondary">Register →</a>
	                <a href="%s/auth/login?next=%s" class="cta-button secondary">Get API Token →</a>
	            </div>
	            %s
	        </div>`,
		pathPrefix, url.QueryEscape(pathPrefix+"/terminal"),
		pathPrefix, url.QueryEscape(pathPrefix+"/terminal"),
		pathPrefix, url.QueryEscape(pathPrefix+"/auth/token-page"),
		authNotice)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>YuanRong Frontend Platform</title>
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
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            max-width: 800px;
            width: 100%%;
            padding: 60px 40px;
            text-align: center;
        }
        .logo {
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            font-size: 36px;
            color: #2d3748;
            margin-bottom: 16px;
        }
        .subtitle {
            font-size: 18px;
            color: #718096;
            margin-bottom: 40px;
        }
        .description {
            text-align: left;
            margin-bottom: 40px;
            line-height: 1.8;
            color: #4a5568;
        }
        .cta-button {
            display: inline-block;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 16px 48px;
            border-radius: 8px;
            text-decoration: none;
            font-size: 18px;
            font-weight: 600;
            transition: transform 0.2s, box-shadow 0.2s;
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
            margin: 0 8px;
        }
        .cta-button:hover {
            transform: translateY(-2px);
            box-shadow: 0 6px 20px rgba(102, 126, 234, 0.6);
        }
        .cta-button.secondary {
            background: white;
            color: #667eea;
            border: 2px solid #667eea;
        }
        .cta-button.secondary:hover {
            background: #f7fafc;
            box-shadow: 0 6px 20px rgba(102, 126, 234, 0.3);
        }
        .cta-group {
            margin-top: 20px;
        }
        .auth-section {
            margin-top: 28px;
            padding-top: 24px;
            border-top: 1px solid #e2e8f0;
            text-align: center;
        }
        .auth-section h3 {
            font-size: 16px;
            color: #4a5568;
            margin-bottom: 12px;
        }
        .auth-group {
            display: flex;
            justify-content: center;
            gap: 16px;
            flex-wrap: wrap;
        }
        .auth-notice {
            margin-top: 12px;
            font-size: 13px;
            color: #718096;
        }
        .docs-section {
            margin-top: 40px;
            padding-top: 24px;
            border-top: 1px solid #e2e8f0;
            text-align: center;
        }
        .docs-section h3 {
            font-size: 16px;
            color: #4a5568;
            margin-bottom: 12px;
        }
        .docs-links {
            display: flex;
            justify-content: center;
            gap: 32px;
            flex-wrap: wrap;
        }
        .docs-link {
            display: inline-block;
            color: #667eea;
            text-decoration: none;
            font-size: 14px;
            transition: color 0.2s;
        }
        .docs-link:hover {
            color: #764ba2;
            text-decoration: underline;
        }
        .footer {
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #e2e8f0;
            color: #a0aec0;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="logo">🌐</div>
        <h1>YuanRong Frontend Platform</h1>
        <p class="subtitle">Serverless Platform Web Management Portal</p>
        
        <div class="description">
            <p><strong>YuanRong Frontend Platform</strong> provides developers with an all-in-one web management interface,
            supporting function invocation, container instance management, online terminal access, and more.
            No client software installation required — manage everything through your browser.</p>
        </div>

        <div class="cta-group">
            <a href="%s/terminal" class="cta-button">Web Terminal →</a>
            <a href="%s/functions" class="cta-button secondary">Function Invoke →</a>
        </div>

        %s

        <div class="docs-section">
            <h3>📚 Developer Resources</h3>
            <div class="docs-links">
                <a href="%s/api-docs" class="docs-link">API Docs →</a>
                <a href="http://docs.openyuanrong.org/" class="docs-link" target="_blank">Official Docs →</a>
            </div>
        </div>

        <div class="footer">
            <p>Powered by YuanRong Serverless Platform</p>
            <p>© 2025-2026 Huawei Technologies Co., Ltd. All rights reserved.</p>
        </div>
    </div>
</body>
</html>`, pathPrefix, pathPrefix, authButtons, pathPrefix)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
