# 路径前缀配置说明

## 概述
当通过 Traefik 或其他反向代理部署 frontend 服务时，可能需要在路径前添加前缀（如 `/frontend`）。`webui` 包已支持自动获取路径前缀。

## 工作原理

代码会按以下优先级获取路径前缀：

1. **HTTP 请求头 `X-Forwarded-Prefix`**（推荐）
   - Traefik 会自动设置此请求头
   - 无需额外配置，即可自动适配

2. **环境变量 `PATH_PREFIX`**（备选）
   - 如需使用环境变量，需在代码中取消注释
   - 适用于固定前缀的场景

## Traefik 配置示例

### 方式一：Traefik 自动设置（推荐）

Traefik 默认会为带有路径前缀的路由设置 `X-Forwarded-Prefix` 请求头：

```yaml
# docker-compose.yml 或 traefik 配置
labels:
  # 路由配置
  - "traefik.http.routers.frontend.rule=PathPrefix(`/frontend`)"
  - "traefik.http.routers.frontend.service=frontend"
  
  # 中间件：去除路径前缀后转发给后端
  - "traefik.http.middlewares.frontend-stripprefix.stripprefix.prefixes=/frontend"
  - "traefik.http.routers.frontend.middlewares=frontend-stripprefix"
  
  # 服务配置
  - "traefik.http.services.frontend.loadbalancer.server.port=8080"
```

**说明**：
- Traefik 会自动设置 `X-Forwarded-Prefix: /frontend`
- 中间件 `stripprefix` 会去除 `/frontend` 前缀后转发给后端服务
- 后端服务收到的请求路径为 `/terminal/ws`，但请求头包含 `X-Forwarded-Prefix: /frontend`
  - 代码会读取此请求头，自动在返回的 HTML 中补全路径前缀

如果需要使用环境变量，需先修改代码：

```go
// 在相应的 handler 中取消此行注释
pathPrefix = os.Getenv("PATH_PREFIX")
```

然后在部署配置中设置环境变量：

```yaml
# docker-compose.yml
services:
  frontend:
    environment:
      - PATH_PREFIX=/frontend
```

## 路径转换示例

假设 Traefik 配置了 `/frontend` 前缀：

| 浏览器请求路径 | Traefik 转发路径 | X-Forwarded-Prefix | HTML 中生成的路径 |
|---------------|-----------------|-------------------|------------------|
| `/frontend/terminal` | `/terminal` | `/frontend` | `/frontend/terminal/static/xterm.js` |
| `/frontend/api/instances` | `/api/instances` | `/frontend` | `/frontend/api/instances` |
| `/frontend/terminal/ws` | `/terminal/ws` | `/frontend` | `ws://host/frontend/terminal/ws` |

## 验证配置

1. **检查请求头**：
   ```bash
   curl -H "X-Forwarded-Prefix: /frontend" http://localhost:8080/terminal
   ```
   查看返回的 HTML 中，静态资源路径是否为 `/frontend/terminal/static/xterm.js`

2. **浏览器开发者工具**：
   - 打开浏览器 Network 面板
   - 访问 `/frontend/terminal`
   - 检查静态资源请求路径是否正确

## 注意事项

1. **路径前缀不应以 `/` 结尾**：
   - ✅ 正确：`/frontend`
   - ❌ 错误：`/frontend/`

2. **如果不需要前缀**：
   - 无需任何配置
   - `pathPrefix` 为空字符串时，路径为 `/terminal/static/xterm.js`

3. **多级路径前缀支持**：
   - ✅ 支持：`/app/frontend`
   - ✅ 支持：`/v1/terminal`

## 故障排查

### 问题：静态资源 404
**原因**：路径前缀配置不正确

**解决**：
1. 检查 Traefik 是否正确设置 `X-Forwarded-Prefix` 请求头
2. 检查浏览器 Network 面板，确认请求的完整 URL
3. 使用 `curl` 测试，查看响应 HTML 中的路径

### 问题：WebSocket 连接失败
**原因**：WebSocket 路径前缀不匹配

**解决**：
1. 确认 WebSocket 路径包含正确前缀：`ws://host/frontend/terminal/ws`
2. 检查 Traefik 配置是否正确处理 WebSocket 升级请求

## 相关代码

主要修改位于 `webterm.go`（Web Terminal 页面处理器）的 `HandleIndex` 函数：
- 获取路径前缀逻辑
- 使用 `fmt.Sprintf` 动态生成带前缀的 HTML

影响的路径：
- `/terminal/static/xterm.css`
- `/terminal/static/xterm.js`
- `/terminal/static/xterm-addon-fit.js`
- `/api/instances`
- `/terminal/ws`
