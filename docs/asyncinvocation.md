# 异步调用实现方案

## 整体架构

```
客户端                          Frontend                           Runtime
  |                               |                                  |
  |  POST /invocations            |                                  |
  |  X-Invoke-Type: async        |                                  |
  |  X-Webhook-Url: ...          |                                  |
  |------------------------------>|                                  |
  |                               |  1. 获取 worker slot             |
  |                               |  2. 生成 requestID               |
  |                               |  3. 存入 StorageBackend(pending) |
  |                               |  4. 启动后台 goroutine          |
  |  HTTP 202 {requestId: "xxx"} |                                  |
  |<------------------------------|                                  |
  |                               |  5. goroutine: status -> running |
  |                               |--------------------------------->|
  |                               |          实际函数调用              |
  |                               |<---------------------------------|
  |                               |  6. 存储结果(completed/failed)   |
  |                               |  7. 发送 Webhook 回调            |
  |                               |                                  |
  |  GET /async-results/{id}     |                                  |
  |------------------------------>|                                  |
  |  HTTP 200 {status, result}   |                                  |
  |<------------------------------|                                  |
```

## 核心模块

### 1. 配置模块 (`config.go`)

- **AsyncConfig**: 异步调用主配置
- **WebhookConfig**: Webhook 配置（启用、超时、重试）
- **StorageConfig**: 存储配置（类型、Redis 连接信息）
- **默认配置**:
  - 最大并发数: 1000
  - 结果保留时间: 60 分钟
  - 清理间隔: 5 分钟

### 2. 存储模块 (`storage.go`)

- **StorageBackend 接口**: 统一的存储抽象
- **MemoryBackend**: 内存存储实现（默认）
- **RedisBackend**: Redis 分布式存储实现
- **自动降级**: Redis 连接失败时自动降级到内存存储
- **TTL 支持**: 自动管理结果过期

### 3. 并发控制 (`worker.go`)

- **WorkerPool**: 基于 channel 的信号量实现
- **Acquire/Release**: 获取和释放并发槽位
- **可配置**: 最大并发数可配置

### 4. Webhook 回调 (`webhook.go`)

- **WebhookPayload**: 回调内容结构
- **指数退避重试**: 1s → 2s → 4s
- **失败处理**: 记录日志但不阻塞主流程

### 5. 监控指标 (`metrics.go`)

- **async_invocation_total**: 异步调用总数（按状态、函数名）
- **async_invocation_duration_seconds**: 异步调用耗时直方图
- **async_invocation_concurrent**: 当前并发数
- **async_webhook_total**: Webhook 发送次数

### 6. 异步处理器 (`api/v1/async_invoke.go`)

- **AsyncInvokeHandler**:
  - 获取 worker slot 限制并发
  - 记录并发 gauge
  - 生成 requestID，创建 pending 状态
  - 后台 goroutine 执行调用
  - 调用完成后发送 Webhook（如果配置了）

- **GetAsyncResultHandler**:
  - 从 StorageBackend 加载结果
  - 返回对应状态的信息

## API 使用方式

### 发起异步调用

```bash
curl -X POST /serverless/v1/functions/{urn}/invocations \
  -H "X-Invoke-Type: async" \
  -H "X-Webhook-Url: https://example.com/callback" \
  -d '{"key": "value"}'
# 返回: HTTP 202 {"requestId": "abc-123"}
```

### 查询结果

```bash
curl -X GET /serverless/v1/functions/async-results/abc-123
# pending:    {"requestId": "abc-123", "status": "pending"}
# running:    {"requestId": "abc-123", "status": "running"}
# completed:  {"requestId": "abc-123", "status": "completed", "statusCode": 200, "respBody": ...}
# not found:  HTTP 404 {"error": "async result not found"}
```

### Webhook 回调格式

```json
{
  "requestId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "statusCode": 200,
  "result": "eyJrZXkiOiJ2YWx1ZSJ9",
  "completedAt": "2026-03-05T12:00:00Z"
}
```

## 配置示例

```yaml
asyncInvocation:
  enabled: true
  maxConcurrent: 1000
  resultRetentionMinutes: 60
  cleanupIntervalMinutes: 5
  webhook:
    enabled: true
    timeoutSecond: 10
    retry:
      maxAttempts: 3
      initialDelayMs: 1000
  storage:
    type: redis
    redis:
      addr: redis:6379
      password: ""
      db: 0
```

## 涉及文件

| 文件 | 描述 |
|------|------|
| `asyncinvocation/store.go` | AsyncResult 定义、内存存储实现 |
| `asyncinvocation/storage.go` | StorageBackend 接口、Redis 后端 |
| `asyncinvocation/config.go` | 配置结构体 |
| `asyncinvocation/worker.go` | 并发控制 |
| `asyncinvocation/webhook.go` | Webhook 回调 |
| `asyncinvocation/metrics.go` | Prometheus 指标 |
| `api/v1/async_invoke.go` | HTTP 处理器 |
| `api/v1/invoke.go` | 入口分发 |
| `api/api.go` | 路由注册 |

## 已完成特性

- [x] 分布式存储: 支持 Redis，多实例共享结果
- [x] Webhook 回调: 支持指数退避重试
- [x] 并发限制: WorkerPool 限制并发数
- [x] 配置化管理: YAML 配置
- [x] 监控指标: Prometheus 指标

## 待完成

- [ ] 主配置集成: 从主配置加载 AsyncConfig
- [ ] 集成测试: 多实例场景验证
