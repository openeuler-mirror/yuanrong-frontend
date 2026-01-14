# 监控部署指南

本目录包含三种部署 Prometheus 和 Grafana 的方式：

1. **进程部署** (`process/`) - 直接在系统上运行 Prometheus 和 Grafana 进程
2. **Docker 部署** (`docker/`) - 使用 Docker 和 Docker Compose 运行
3. **Kubernetes 部署** (`k8s/`) - 在 Kubernetes 集群中部署

## 选择部署方式

### 使用进程部署，如果你：
- 不想使用 Docker 或 Kubernetes
- 需要更细粒度的系统控制
- 在生产环境中已有进程管理工具（如 systemd）
- 需要直接访问系统资源

### 使用 Docker 部署，如果你：
- 已经使用 Docker 环境
- 需要快速部署和测试
- 需要环境隔离
- 需要易于迁移和扩展
- 但不需要 Kubernetes 的编排能力

### 使用 Kubernetes 部署，如果你：
- 已有 Kubernetes 集群
- 需要高可用和自动扩缩容
- 需要与其他 Kubernetes 服务集成
- 需要服务发现和自动配置
- 生产环境部署

## 目录结构

```
monitoring/
├── process/              # 进程部署相关文件
│   ├── prometheus.yml
│   ├── grafana-dashboard.json
│   ├── start-monitoring.sh
│   ├── stop-monitoring.sh
│   └── README.md
│
├── docker/               # Docker 部署相关文件
│   ├── docker-compose.yml
│   ├── prometheus.yml
│   ├── start.sh
│   ├── stop.sh
│   └── README.md
│
└── k8s/                  # Kubernetes 部署相关文件
    ├── namespace.yaml
    ├── prometheus-*.yaml
    ├── grafana-*.yaml
    ├── deploy.sh
    ├── undeploy.sh
    └── README.md
```

## 快速开始

### 进程部署

```bash
cd process
./start-monitoring.sh
```

详细说明请参考: [process/README.md](process/README.md)

### Docker 部署

```bash
cd docker
./start.sh
```

详细说明请参考: [docker/README.md](docker/README.md)

### Kubernetes 部署

```bash
cd k8s
./deploy.sh
```

详细说明请参考: [k8s/README.md](k8s/README.md)

## 指标说明

三种部署方式都监控相同的指标：

- **`function_invocations_total`**: 函数调用总数
  - 标签: `function_name`, `http_code`
  
- **`function_invocation_duration_seconds`**: 函数调用耗时
  - 标签: `function_name`
  - 自动生成: `_bucket`, `_sum`, `_count`

## 访问地址

### 进程部署和 Docker 部署

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (默认用户名/密码: admin/admin)

### Kubernetes 部署

**方式一：端口转发（推荐用于测试）**
```bash
kubectl port-forward -n monitoring svc/prometheus 9090:9090
kubectl port-forward -n monitoring svc/grafana 3000:3000
```

**方式二：NodePort（如果已创建）**
- **Prometheus**: http://<node-ip>:30090
- **Grafana**: http://<node-ip>:30300

**方式三：Ingress（需要配置）**
- 根据 Ingress 配置的域名访问

## 注意事项

1. **端口冲突**: 确保 9090 和 3000 端口未被占用
2. **Frontend 服务端口**: 根据实际情况修改 Prometheus 配置中的 targets
3. **数据持久化**: 
   - 进程部署: 数据保存在 `process/prometheus/data` 和 `process/grafana/data`
   - Docker 部署: 数据保存在 `docker/prometheus-data` 和 `docker/grafana-data`
   - Kubernetes 部署: 数据保存在 PVC（PersistentVolumeClaim）中

## 故障排查

### 检查指标端点

访问你的 Frontend 服务指标端点，确认指标正常：
```
http://your-frontend-host:port/metrics
```

应该能看到：
- `function_invocations_total`
- `function_invocation_duration_seconds_bucket`
- `function_invocation_duration_seconds_sum`
- `function_invocation_duration_seconds_count`

### 检查 Prometheus 抓取

访问 Prometheus 的 Targets 页面：
```
http://localhost:9090/targets
```

确认 `yuanrong-frontend` 目标状态为 "UP"。

### 检查 Grafana 数据源

在 Grafana 中：
1. Configuration -> Data Sources
2. 选择 Prometheus
3. 点击 "Save & Test"

应该显示 "Data source is working"。

## 部署方式对比

| 特性 | 进程部署 | Docker 部署 | Kubernetes 部署 |
|------|---------|------------|----------------|
| 安装复杂度 | 中等 | 低 | 中等 |
| 启动速度 | 快 | 快 | 中等 |
| 资源隔离 | 无 | 容器隔离 | Pod 隔离 |
| 高可用 | 需手动配置 | 需手动配置 | 原生支持 |
| 自动扩缩容 | 不支持 | 不支持 | 支持 |
| 服务发现 | 手动配置 | 手动配置 | 自动发现 |
| 适用场景 | 单机/简单环境 | 开发/测试 | 生产环境 |

## 更多信息

- 进程部署详细文档: [process/README.md](process/README.md)
- Docker 部署详细文档: [docker/README.md](docker/README.md)
- Kubernetes 部署详细文档: [k8s/README.md](k8s/README.md)
- 快速开始指南: [process/QUICK_START.md](process/QUICK_START.md)
