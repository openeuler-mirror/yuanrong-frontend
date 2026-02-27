# Docker 部署可观测性全栈（Logs / Traces / Metrics）

本文档介绍如何使用 Docker Compose 部署完整的可观测性栈，统一采集 Yuanrong 系统的日志、链路追踪和指标，并在 Grafana 中统一展示和关联查询。

## 架构概览

```
yuanrong 应用
  ├─ 日志文件 (/home/yr/log/*.log)
  ├─ Traces (OTLP gRPC :4317)
  └─ Metrics (OTLP gRPC :4317)
        │
        ▼
  OTel Collector ──┬── logs ────→ Loki (:3100)
                   ├── traces ──→ Tempo (:3200) ─→ RED metrics ─→ Prometheus
                   └── metrics ─→ Prometheus (:9090, remote write)
                                      │
                                      ▼
                              Grafana (:3000)
                        ┌──────────┼──────────┐
                        │          │          │
                      Logs      Traces    Metrics
                      (Loki)   (Tempo)  (Prometheus)
                        └──── 互相关联跳转 ────┘
```

### 组件说明

| 组件 | 版本 | 职责 | 端口 |
|------|------|------|------|
| **OTel Collector** | contrib:latest | 统一采集入口，接收日志/traces/metrics 并转发到各后端 | 4317 (gRPC) |
| **Loki** | 3.4.2 | 日志存储与查询 | 3100 |
| **Tempo** | 2.7.1 | 分布式链路追踪存储与查询 | 3200 |
| **Prometheus** | latest | 指标存储与查询 | 9090 |
| **Grafana** | latest | 统一可视化看板，三大信号关联跳转 | 3000 |

## 前置要求

- Docker 20.10 或更高版本
- Docker Compose 2.0 或更高版本（或 Docker 内置的 `docker compose` 命令）

### 安装 Docker

**Ubuntu/Debian:**
```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
```

**CentOS/RHEL:**
```bash
sudo yum install -y docker
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER
```

安装完成后，需要重新登录或执行 `newgrp docker` 使权限生效。

## 快速开始

### 1. 配置 Prometheus

编辑 `prometheus.yml`，根据你的 Frontend 服务位置修改 targets：

**如果 Frontend 服务在宿主机上运行:**
```yaml
- targets: ['host.docker.internal:8888']  # 使用 host.docker.internal 访问宿主机
```

**如果 Frontend 服务在 Docker 网络中:**
```yaml
- targets: ['frontend-service:8888']  # 使用服务名或容器名
```

**如果 Frontend 服务在其他机器上:**
```yaml
- targets: ['192.168.1.100:8888']  # 使用实际 IP 地址
```

### 2. 修复权限（首次运行或遇到权限问题时）

Prometheus 容器以 `nobody` 用户（UID 65534）运行，需要确保数据目录有正确的权限：

```bash
cd docs/monitoring/docker
./fix-permissions.sh
```

或者手动修复：
```bash
sudo chown -R 65534:65534 prometheus-data
sudo chmod -R 755 prometheus-data
```

### 3. 启动服务

```bash
cd docs/monitoring/docker
chmod +x *.sh
./start.sh
```

**注意**: `start.sh` 脚本会自动尝试修复权限，但如果遇到权限问题，请手动运行 `fix-permissions.sh`。

### 4. 访问服务

| 服务 | 地址 | 说明 |
|------|------|------|
| **Grafana** | http://localhost:3000 | 统一看板（admin / admin） |
| **Prometheus** | http://localhost:9090 | 指标查询 |
| **Loki** | http://localhost:3100 | 日志 API |
| **Tempo** | http://localhost:3200 | 链路追踪 API |

### 5. 停止服务

```bash
./stop.sh
```

## 目录结构

```
docker/
├── docker-compose.yml              # Docker Compose 编排（6 个服务）
├── otel-collector-config.yaml      # OTel Collector 配置（filelog + OTLP 接收 → Loki/Tempo/Prometheus 导出）
├── loki-config.yaml                # Loki 配置（TSDB 存储，7 天保留）
├── tempo-config.yaml               # Tempo 配置（本地存储 + RED metrics 生成）
├── prometheus.yml                  # Prometheus 抓取配置
├── start.sh                        # 启动脚本
├── stop.sh                         # 停止脚本
├── fix-permissions.sh              # 权限修复脚本
├── README.md                       # 本文档
├── prometheus-data/                # Prometheus 数据（自动创建）
├── loki-data/                      # Loki 数据（自动创建）
├── tempo-data/                     # Tempo 数据（自动创建）
├── grafana-data/                   # Grafana 数据（自动创建）
├── grafana-provisioning/           # Grafana 自动配置
│   ├── datasources/
│   │   ├── prometheus.yml          # Prometheus 数据源（exemplar → Tempo）
│   │   ├── loki.yml                # Loki 数据源（traceID → Tempo）
│   │   └── tempo.yml               # Tempo 数据源（关联 Loki + Prometheus）
│   └── dashboards/
│       └── default.yml
└── grafana-dashboards/             # Grafana 仪表板 JSON 文件
    └── yuanrong-frontend.json
```

## 配置说明

### Docker Compose 配置

`docker-compose.yml` 包含以下服务：

| 服务 | 镜像 | 端口 | 数据卷 |
|------|------|------|--------|
| **Loki** | grafana/loki:3.4.2 | 3100 | `./loki-data` |
| **Tempo** | grafana/tempo:2.7.1 | 3200 | `./tempo-data` |
| **OTel Collector** | otel/opentelemetry-collector-contrib:latest | 4317 | 挂载 `/home/yr/log`（只读） |
| **Prometheus** | prom/prometheus:latest | 9090 | `./prometheus-data` |
| **Grafana** | grafana/grafana:latest | 3000 | `./grafana-data` |

### OTel Collector 配置

`otel-collector-config.yaml` 定义了三条数据管道：

| 管道 | 接收器 | 导出器 | 说明 |
|------|--------|--------|------|
| **logs** | filelog（读取 `/home/yr/log/*.log`） | Loki (OTLP HTTP) | 正则解析 spdlog 格式，提取 severity/node/component |
| **traces** | OTLP gRPC (:4317) | Tempo (OTLP gRPC) | 应用通过 `TraceManager` 发送 |
| **metrics** | OTLP gRPC (:4317) | Prometheus (remote write) | 应用通过 `MetricsAdapter` 发送 |

**日志格式解析**：Collector 会解析 spdlog 格式日志，自动提取以下字段：

```
I0227 10:15:32.123456 12345 main.cpp:42] 9876,!]node1,function_proxy]Started proxy
│                      │     │            │        │     │              └─ body
│                      │     │            │        │     └─ component.name
│                      │     │            │        └─ node.name
│                      │     │            └─ process.pid
│                      │     └─ code.filepath
│                      └─ thread.id
└─ severity (D=debug, I=info, W=warn, E=error, C=fatal)
```

**自定义日志路径**：如果日志目录不是 `/home/yr/log`，需修改两处：
1. `docker-compose.yml` 中 otel-collector 的 volumes 映射
2. `otel-collector-config.yaml` 中 filelog receiver 的 `include` 路径

### Grafana 数据源关联

三个数据源已配置互相跳转：

| 起点 | 跳转目标 | 触发方式 |
|------|---------|---------|
| **Logs → Traces** | 日志中的 `traceID=xxx` 自动生成链接 | 点击 traceID 跳转到 Tempo |
| **Traces → Logs** | Trace 详情页可查看对应时间段日志 | 点击 "Logs for this span" |
| **Traces → Metrics** | Trace 详情页可查看对应指标 | 点击 "Related metrics" |
| **Metrics → Traces** | Prometheus exemplar 中嵌入 traceID | 点击 exemplar 跳转到 Tempo |

### 网络配置

- 使用 Docker bridge 网络 `monitoring`
- 所有服务在同一网络中，通过容器名互相访问
- 应用发送 traces/metrics 到 `otel-collector:4317`（容器内）或 `localhost:4317`（宿主机）

### 数据持久化

| 数据 | 目录 | 保留策略 |
|------|------|---------|
| Prometheus | `./prometheus-data` | 默认 15 天 |
| Loki | `./loki-data` | 7 天（可在 `loki-config.yaml` 中调整） |
| Tempo | `./tempo-data` | 默认保留 |
| Grafana | `./grafana-data` | 永久 |

删除容器不会删除数据（除非使用 `docker compose down -v`）

## 常用命令

### 查看服务状态

```bash
docker-compose ps
# 或
docker compose ps
```

### 查看日志

```bash
# 查看所有服务日志
docker compose logs -f

# 查看单个服务日志
docker logs -f otel-collector
docker logs -f loki
docker logs -f tempo
docker logs -f prometheus
docker logs -f grafana
```

### 重启服务

```bash
docker-compose restart
# 或
docker compose restart
```

### 更新配置

修改配置文件后，需要重启服务：

```bash
# 重启 Prometheus（重新加载配置）
docker-compose restart prometheus

# 或使用 Prometheus 的 reload API
curl -X POST http://localhost:9090/-/reload
```

### 清理数据

```bash
# 停止并删除容器和数据卷
docker-compose down -v
```

## 故障排查

### Prometheus 权限错误

如果看到类似 `permission denied` 的错误：

1. **修复数据目录权限**
   ```bash
   ./fix-permissions.sh
   ```

2. **或者重新创建数据目录**
   ```bash
   docker-compose down
   sudo rm -rf prometheus-data
   mkdir -p prometheus-data
   sudo chown -R 65534:65534 prometheus-data
   docker-compose up -d
   ```

3. **检查容器日志**
   ```bash
   docker logs prometheus
   ```

### Prometheus 无法抓取指标

1. **检查网络连接**
   ```bash
   # 在 Prometheus 容器中测试连接
   docker exec prometheus wget -O- http://host.docker.internal:8888/metrics
   ```

2. **检查配置文件**
   ```bash
   # 验证 Prometheus 配置
   docker exec prometheus promtool check config /etc/prometheus/prometheus.yml
   ```

3. **查看 Prometheus 目标状态**
   - 访问 http://localhost:9090/targets
   - 检查 `yuanrong-frontend` 目标状态

### Grafana 无法连接 Prometheus

1. **检查数据源配置**
   - 登录 Grafana
   - 进入 Configuration -> Data Sources
   - 检查 Prometheus URL 是否为 `http://prometheus:9090`

2. **测试连接**
   - 在数据源配置页面点击 "Save & Test"
   - 查看错误信息

### 容器无法启动

1. **检查端口占用**
   ```bash
   # 检查端口是否被占用
   netstat -tuln | grep -E '9090|3000'
   ```

2. **查看容器日志**
   ```bash
   docker-compose logs
   ```

3. **检查文件权限**
   ```bash
   # 确保目录有正确的权限
   chmod -R 755 .
   ```

## Grafana 查询示例

### 日志查询（Loki - LogQL）

在 Grafana → Explore → 选择 Loki 数据源：

```logql
# 查看所有日志
{service_name="yuanrong-functionsystem"}

# 按组件过滤
{component_name="function_proxy"}

# 按节点和级别过滤
{node_name="node1"} | severity >= "error"

# 关键字搜索
{service_name="yuanrong-functionsystem"} |= "timeout"

# 统计每分钟错误数
count_over_time({service_name="yuanrong-functionsystem"} | severity = "error" [1m])
```

### 链路追踪（Tempo）

在 Grafana → Explore → 选择 Tempo 数据源：

- **Search**: 按 service name、duration、status 搜索 traces
- **TraceQL**: `{resource.service.name="yuanrong-functionsystem" && duration > 1s}`
- **Service Graph**: 自动生成服务拓扑图（基于 Tempo metrics generator）

### 指标查询（Prometheus - PromQL）

在 Grafana → Explore → 选择 Prometheus 数据源：

```promql
# Tempo 自动生成的 RED metrics（需要应用发送 traces）
# 请求速率
rate(traces_spanmetrics_calls_total{service="yuanrong-functionsystem"}[5m])

# 错误率
rate(traces_spanmetrics_calls_total{service="yuanrong-functionsystem", status_code="STATUS_CODE_ERROR"}[5m])

# P99 延迟
histogram_quantile(0.99, rate(traces_spanmetrics_duration_milliseconds_bucket{service="yuanrong-functionsystem"}[5m]))
```

## 应用接入配置

yuanrong-functionsystem 各组件已通过 `ModuleSwitcher` 集成 OTel。确保以下配置指向 OTel Collector：

**Trace 配置**（JSON，传给 `--trace_config` 参数）：
```json
{
  "otlpGrpcExporter": {
    "enable": true,
    "endpoint": "localhost:4317"
  }
}
```

**日志配置**（JSON，传给 `--log_config` 参数）：
```json
{
  "filepath": "/home/yr/log",
  "level": "DEBUG",
  "alsologtostderr": true
}
```

> 日志采集不需要修改应用代码，OTel Collector 直接读取日志文件。

## 生产环境建议

1. **使用环境变量文件**
   - 创建 `.env` 文件存储敏感信息
   - 在 `docker-compose.yml` 中使用 `${VARIABLE}` 引用

2. **配置资源限制**
   ```yaml
   services:
     prometheus:
       deploy:
         resources:
           limits:
             cpus: '2'
             memory: 4G
   ```

3. **使用 Docker Secrets**
   - 存储 Grafana 管理员密码
   - 存储其他敏感配置

4. **配置日志驱动**
   ```yaml
   services:
     prometheus:
       logging:
         driver: "json-file"
         options:
           max-size: "10m"
           max-file: "3"
   ```

5. **使用健康检查**
   - 已在 `docker-compose.yml` 中配置
   - 可以添加自动重启策略

6. **定期备份**
   ```bash
   # 备份 Prometheus 数据
   tar -czf prometheus-backup-$(date +%Y%m%d).tar.gz prometheus-data/
   
   # 备份 Grafana 数据
   tar -czf grafana-backup-$(date +%Y%m%d).tar.gz grafana-data/
   ```

## 与进程部署的区别

| 特性 | Docker 部署 | 进程部署 |
|------|------------|---------|
| 安装 | 需要 Docker | 需要手动安装二进制文件 |
| 配置 | docker-compose.yml | 多个配置文件 |
| 启动 | `docker-compose up` | 需要手动启动多个进程 |
| 隔离 | 容器隔离 | 系统进程 |
| 数据持久化 | Docker 卷 | 本地目录 |
| 网络 | Docker 网络 | 本地网络 |

## 参考资源

- [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib)
- [Grafana Loki 文档](https://grafana.com/docs/loki/latest/)
- [Grafana Tempo 文档](https://grafana.com/docs/tempo/latest/)
- [Prometheus 文档](https://prometheus.io/docs/)
- [Grafana 文档](https://grafana.com/docs/grafana/latest/)
- [Filelog Receiver 配置](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver)
