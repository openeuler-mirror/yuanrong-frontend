# JWT Authentication Middleware

## 功能概述

JWT 认证中间件提供了智能的身份验证失败处理机制，能够根据客户端类型返回不同的响应：

- **浏览器用户**：显示友好的登录表单页面，允许用户输入 JWT Token
- **API 用户**：返回标准的 JSON 错误响应

## 客户端类型检测

中间件通过检查 HTTP 请求的 `Accept` 头来判断客户端类型：

```go
Accept: text/html  // 浏览器请求
Accept: application/json  // API 请求
```

## 浏览器用户体验

### 认证失败场景

当浏览器用户访问需要认证的页面但未提供有效 Token 时，会看到一个美观的登录表单页面。

### 登录表单功能

- 🔒 显示具体的错误信息
- 📝 提供 Token 输入框
- ✨ 现代化的 UI 设计
- 🔄 自动将 Token 添加到 URL 并刷新页面
- 🏠 返回首页链接
- 💡 帮助提示文本

### 错误消息类型

1. **未提供 Token**
   ```
   认证失败：未提供 Token，请输入有效的 JWT Token
   ```

2. **Token 无效**
   ```
   认证失败：Token 无效或格式错误 (具体错误信息)
   ```

3. **角色无权限**
   ```
   认证失败：角色 user 无权访问此资源
   ```

4. **IAM 验证失败**
   ```
   认证失败：IAM 服务器验证失败 (具体错误信息)
   ```

## API 用户响应

API 客户端收到标准的 HTTP 401 JSON 响应：

```json
{
  "error": "认证失败：未提供 Token，请输入有效的 JWT Token"
}
```

## Token 传递方式

中间件支持两种方式传递 JWT Token：

1. **HTTP 头**（推荐用于 API）
   ```
   X-Auth: eyJhbGciOiJIUzI1NiIs...
   ```

2. **URL 查询参数**（推荐用于浏览器）
   ```
   https://example.com/terminal?token=eyJhbGciOiJIUzI1NiIs...
   ```

## 使用示例

### 浏览器访问流程

1. 用户在浏览器中访问 `https://example.com/terminal`
2. 未提供 Token，显示登录表单
3. 用户输入 Token 并提交
4. 页面重定向到 `https://example.com/terminal?token=xxx`
5. 认证成功，显示目标页面

### API 调用流程

```bash
# 未认证请求
curl -X GET "http://example.com/api/instances"

# 响应 401
{
  "error": "认证失败：未提供 Token，请输入有效的 JWT Token"
}

# 带 Token 请求
curl -X GET "http://example.com/api/instances" \
  -H "X-Auth: eyJhbGciOiJIUzI1NiIs..."

# 响应 200
[{"id": "instance-1", "status": "running"}]
```

## 路径前缀支持

登录表单支持反向代理的路径前缀（通过 `X-Forwarded-Prefix` 头）：

```
X-Forwarded-Prefix: /frontend
```

表单的返回首页链接和 Token 提交后的重定向都会自动包含正确的路径前缀。

## 白名单机制

某些路径不需要认证，通过白名单机制配置：

- `/` - 欢迎页面（不需要认证）
- `/terminal/static/*` - 静态资源（不需要认证）
- `/terminal/ws` - WebSocket（在处理器中认证）

参见 `jwtauth_whitelist.go` 了解详细配置。

## 代码实现

### 核心函数

```go
// 判断是否为浏览器请求
func isBrowserRequest(ctx *gin.Context) bool {
    accept := ctx.GetHeader("Accept")
    return strings.Contains(accept, "text/html")
}

// 根据客户端类型返回相应的错误响应
func respondAuthError(ctx *gin.Context, message string) {
    if isBrowserRequest(ctx) {
        // 返回 HTML 登录表单
        ctx.Data(http.StatusUnauthorized, "text/html; charset=utf-8", htmlContent)
    } else {
        // 返回 JSON 错误
        ctx.JSON(http.StatusUnauthorized, gin.H{"error": message})
    }
    ctx.Abort()
}
```

### 集成方式

在所有认证失败点使用 `respondAuthError`：

```go
if authHeader == "" {
    respondAuthError(ctx, "认证失败：未提供 Token，请输入有效的 JWT Token")
    return
}
```

## 安全考虑

1. **Token 传输**：建议在生产环境使用 HTTPS
2. **Token 存储**：浏览器通过 URL 参数传递，不存储在 Cookie 或 localStorage
3. **错误信息**：提供足够的信息帮助调试，但不泄露敏感细节
4. **IAM 验证**：所有 Token 都会经过 IAM 服务器验证

## 配置选项

在 `config.yaml` 中控制 JWT 认证：

```yaml
iam_config:
  enable_func_token_auth: true  # 启用 JWT 认证
  iam_server: "http://iam-server:8080"  # IAM 服务器地址
```

## 测试建议

### 浏览器测试

1. 在浏览器中访问 `/terminal`（不带 token 参数）
2. 验证是否显示登录表单
3. 输入有效 Token 并提交
4. 验证是否成功跳转到目标页面

### API 测试

```bash
# 测试未认证请求
curl -v -X GET "http://localhost:8080/api/instances"

# 测试已认证请求
curl -v -X GET "http://localhost:8080/api/instances" \
  -H "X-Auth: YOUR_TOKEN"

# 测试无效 Token
curl -v -X GET "http://localhost:8080/api/instances" \
  -H "X-Auth: invalid_token"
```

## 相关文件

- `jwtauth.go` - JWT 认证中间件主文件
- `jwtauth_whitelist.go` - 白名单配置
- `jwtauth_whitelist_test.go` - 白名单测试
- `invoke_preprocessing.go` - 调用预处理中间件

## 故障排查

### 问题：浏览器总是显示 JSON 错误

**原因**：浏览器没有发送 `Accept: text/html` 头

**解决**：检查浏览器请求头，确保包含 `text/html`

### 问题：登录表单提交后还是提示未认证

**原因**：Token 格式错误或已过期

**解决**：
1. 使用 CLI 工具重新获取 Token：`yr token-require`
2. 检查 Token 是否包含空格或换行符
3. 验证 Token 是否在有效期内

### 问题：API 请求返回 HTML 而不是 JSON

**原因**：API 客户端发送了 `Accept: text/html` 头

**解决**：API 请求应该发送 `Accept: application/json` 或不发送 Accept 头

## 未来改进

- [ ] 支持 Token 刷新机制
- [ ] 记住 Token（可选的 localStorage 存储）
- [ ] 多因素认证（MFA）支持
- [ ] OAuth/OIDC 集成
- [ ] 更细粒度的权限控制
