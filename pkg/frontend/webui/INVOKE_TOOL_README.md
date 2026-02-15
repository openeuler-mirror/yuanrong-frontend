# Function Invoke Tool 使用文档

## 概述

Function Invoke Tool 是一个基于 Web 的 Serverless 函数调试工具，允许开发者通过浏览器界面快速测试和调试函数。

## 功能特性

- ✅ **Web 界面调用**：通过表单输入参数调用函数
- ✅ **JWT 认证**：需要有效的 JWT token 进行身份验证
- ✅ **JSON 支持**：请求体和响应均支持 JSON 格式
- ✅ **响应格式化**：自动格式化 JSON 响应便于阅读
- ✅ **调用历史**：本地保存最近 10 次调用记录
- ✅ **路径前缀支持**：支持 Traefik 等反向代理
- ⏳ **函数列表**：预留功能（待实现）

## 访问地址

```
http://your-domain/functions?token=YOUR_JWT_TOKEN
```

### 参数说明

- `token`: JWT 认证令牌（必需）

## 使用方法

### 1. 访问页面

在浏览器中打开 `/functions` 页面，URL 中需要包含有效的 JWT token。

### 2. 填写请求参数

- **Tenant ID**: 租户标识符（默认值：default）
- **Namespace**: 命名空间（默认值：default）
- **Function Name**: 函数名称（必填）
- **Request Body**: JSON 格式的请求体（默认值：{}）

### 3. 调用函数

点击"调用函数"按钮，或使用快捷键 `Ctrl + Enter`。

### 4. 查看响应

右侧面板会显示：
- **状态码**: HTTP 响应状态码
- **耗时**: 请求响应时间（毫秒）
- **大小**: 响应体大小
- **响应内容**: 格式化的 JSON 或原始文本

### 5. 使用历史记录

工具会自动保存最近 10 次调用记录，点击历史记录项可快速恢复之前的调用参数。

## 调用示例

### 请求配置

```
Tenant ID: tenant_001
Namespace: production
Function Name: hello-world
Request Body: {"name": "张三", "age": 30}
```

### 实际调用

工具会向以下 URL 发送 POST 请求：

```
POST /tenant_001/production/hello-world/
Content-Type: application/json
X-Auth: YOUR_JWT_TOKEN

{"name": "张三", "age": 30}
```

### 响应示例

```json
{
  "status": 200,
  "message": "Hello, 张三!",
  "timestamp": "2026-02-13T10:30:00Z"
}
```

## 功能按钮

### 调用函数
执行函数调用请求。

### 清空
清空函数名称和请求体，保留 Tenant ID 和 Namespace。

### 格式化 JSON
自动格式化请求体中的 JSON，便于阅读和编辑。

## 快捷键

- `Ctrl + Enter`: 调用函数

## 路径前缀支持

当通过 Traefik 等反向代理访问时，工具会自动读取 `X-Forwarded-Prefix` 请求头，确保所有资源和 API 调用都使用正确的路径前缀。

### Traefik 配置示例

```yaml
http:
  middlewares:
    add-prefix:
      headers:
        customRequestHeaders:
          X-Forwarded-Prefix: "/frontend"
  
  routers:
    frontend-router:
      rule: "PathPrefix(`/frontend`)"
      middlewares:
        - add-prefix
      service: frontend-service
```

## 认证要求

- 页面访问需要有效的 JWT token
- Token 通过 URL 参数传递：`?token=YOUR_TOKEN`
- 函数调用时，token 会通过 `X-Auth` 请求头传递给后端

## 数据存储

- 调用历史保存在浏览器的 localStorage 中
- 最多保存 10 条历史记录
- 清除浏览器缓存会删除历史记录

## 后续规划

### 函数列表功能（待实现）

- 展示租户下所有可用函数
- 按命名空间分组
- 快速选择函数进行调试
- 显示函数元数据（运行时、内存、超时等）

### 其他增强

- 支持更多内容类型（Form、XML、Plain Text）
- 自定义请求头
- 响应下载功能
- 调用统计和性能分析
- 批量调用和压测

## 技术实现

- **前端**: 纯 HTML/CSS/JavaScript，无额外依赖
- **后端**: Go + Gin 框架
- **认证**: JWT token 验证
- **API**: 复用现有的 ShortInvokeHandler

## 相关文件

- `invoke.go`: 函数调用工具页面处理器
- `welcome.go`: 欢迎页面（包含工具入口）
- `api.go`: 路由配置
- `PATH_PREFIX_README.md`: 路径前缀配置文档

## 故障排查

### 问题：无法访问页面

**可能原因**：
- Token 无效或过期
- 未提供 token 参数

**解决方案**：
- 检查 URL 中的 token 参数
- 使用有效的 JWT token

### 问题：调用失败

**可能原因**：
- 函数不存在
- 请求体 JSON 格式错误
- 权限不足

**解决方案**：
- 检查 Tenant ID、Namespace 和 Function Name 是否正确
- 使用"格式化 JSON"按钮验证请求体格式
- 确认 token 具有调用该函数的权限

### 问题：响应显示异常

**可能原因**：
- 响应不是 JSON 格式
- 响应体过大

**解决方案**：
- 工具会自动处理非 JSON 响应，显示为原始文本
- 对于大型响应，考虑使用 API 直接调用

## 联系方式

如有问题或建议，请联系开发团队。
