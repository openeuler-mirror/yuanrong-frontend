# Kubernetes 部署监控服务栈

在 Kubernetes 集群中部署完整可观测性三支柱：**Metrics（Prometheus）+ Logs（Loki）+ Traces（Tempo）**，通过 OpenTelemetry Collector 统一采集，Grafana 统一展示。

## 服务清单

| 服务 | 镜像 | 端口 | 说明 |
|------|------|------|------|
| Prometheus | prom/prometheus:latest | 9090 | 指标存储，支持 Kubernetes 服务发现 |
| Loki | grafana/loki:3.4.2 | 3100 | 日志聚合，7 天保留 |
| Tempo | grafana/tempo:2.7.1 | 3200 / 4317 / 4318 | 链路追踪，RED metrics 生成 |
| OTel Collector | otel/opentelemetry-collector-contrib | 4317 / 4318 / 8889 | 统一采集器，filelog + OTLP |
| Grafana | grafana/grafana:latest | 3000 | 可视化，自动配置三个数据源 |

## 前置要求

- Kubernetes 集群 (1.20+)
- kubectl 已配置并可以访问集群
- 集群有足够资源（建议至少 4 CPU, 8GB 内存）
- StorageClass 已配置（用于 PVC）

## 快速开始

```bash
cd docs/monitoring/k8s
chmod +x deploy.sh undeploy.sh

# 方式一：脚本部署（推荐）
./deploy.sh

# 方式二：Kustomize 一键部署
kubectl apply -k .
```

## 访问服务

### 端口转发（测试用）

```bash
kubectl port-forward -n monitoring svc/grafana     3000:3000 &
kubectl port-forward -n monitoring svc/prometheus  9090:9090 &
kubectl port-forward -n monitoring svc/loki        3100:3100 &
kubectl port-forward -n monitoring svc/tempo       3200:3200 &
```

访问地址：
- Grafana: http://localhost:3000  （admin / admin）
- Prometheus: http://localhost:9090
- Loki: http://localhost:3100
- Tempo: http://localhost:3200

### NodePort

```bash
kubectl get nodes -o wide   # 获取节点 IP
# Prometheus: http://<node-ip>:30090
# Grafana:    http://<node-ip>:30300
```

## 文件说明

### Prometheus

| 文件 | 说明 |
|------|------|
| `prometheus-rbac.yaml` | ServiceAccount + ClusterRole（k8s 服务发现所需） |
| `prometheus-configmap.yaml` | 抓取配置（支持 k8s SD 和静态配置） |
| `prometheus-deployment.yaml` | Deployment（含权限修复 initContainer 可选） |
| `prometheus-service.yaml` | ClusterIP + NodePort(:30090) |
| `prometheus-pvc.yaml` | 20Gi 数据持久化 |
| `prometheus-initcontainer.yaml` | 权限修复替代方案（可选） |

### Loki

| 文件 | 说明 |
|------|------|
| `loki-configmap.yaml` | 单节点配置，TSDB schema，7 天保留 |
| `loki-deployment.yaml` | Deployment |
| `loki-service.yaml` | ClusterIP |
| `loki-pvc.yaml` | 20Gi 数据持久化 |

### Tempo

| 文件 | 说明 |
|------|------|
| `tempo-configmap.yaml` | 本地存储，RED metrics → Prometheus |
| `tempo-deployment.yaml` | Deployment（initContainer 等待 Prometheus 就绪） |
| `tempo-service.yaml` | ClusterIP（http/otlp-grpc/otlp-http） |
| `tempo-pvc.yaml` | 20Gi 数据持久化 |

### OTel Collector

| 文件 | 说明 |
|------|------|
| `otel-collector-configmap.yaml` | filelog + OTLP 接收，三路 pipeline 输出 |
| `otel-collector-deployment.yaml` | Deployment + hostPath 挂载 `/home/yr/log` |
| `otel-collector-service.yaml` | ClusterIP |

### Grafana

| 文件 | 说明 |
|------|------|
| `grafana-configmap.yaml` | 三个数据源（Prometheus/Loki/Tempo），含联动配置 |
| `grafana-dashboards-provider.yaml` | 仪表板加载器配置 |
| `grafana-dashboard-configmap.yaml` | yuanrong-frontend 仪表板 |
| `grafana-deployment.yaml` | Deployment |
| `grafana-service.yaml` | ClusterIP + NodePort(:30300) |
| `grafana-pvc.yaml` | 10Gi 数据持久化 |
| `grafana-secret.yaml` | admin 密码（生产环境替换） |

## 关键配置说明

### OTel Collector — 日志采集路径

OTel Collector 通过 `hostPath` 挂载宿主机 `/home/yr/log` 目录。如果日志只在特定节点，需在 `otel-collector-deployment.yaml` 中配置：

```yaml
# 指定节点
nodeSelector:
  kubernetes.io/hostname: <node-name>
```

如需在每个节点采集，将 Deployment 改为 **DaemonSet**。

### Prometheus — 服务发现

默认配置使用 Kubernetes 服务发现（`kubernetes_sd_configs`），自动发现有 `metrics` 端口名称的 Service。

Frontend Service 需包含：
```yaml
ports:
- name: metrics    # 端口名称必须为 metrics
  port: 8888
```

或添加注解：
```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8888"
```

### Grafana — 数据源联动

三个数据源已配置联动：
- **Prometheus → Tempo**: exemplar traceID 跳转
- **Loki → Tempo**: 日志中提取 traceID 跳转
- **Tempo → Loki/Prometheus**: trace 详情中查关联日志/指标

### 资源配置

| 服务 | 内存请求 | 内存上限 | CPU 请求 | CPU 上限 |
|------|---------|---------|---------|---------|
| Prometheus | 512Mi | 2Gi | 250m | 1000m |
| Loki | 256Mi | 1Gi | 100m | 500m |
| Tempo | 512Mi | 2Gi | 200m | 1000m |
| OTel Collector | 512Mi | 2Gi | 200m | 1000m |
| Grafana | 256Mi | 512Mi | 100m | 500m |

## 故障排查

### 常用命令

```bash
# 查看所有 Pod 状态
kubectl get pods -n monitoring

# 查看某个 Pod 日志
kubectl logs -n monitoring deployment/loki
kubectl logs -n monitoring deployment/tempo
kubectl logs -n monitoring deployment/otel-collector
kubectl logs -n monitoring deployment/prometheus
kubectl logs -n monitoring deployment/grafana

# 查看 PVC
kubectl get pvc -n monitoring

# 验证 Prometheus 配置
kubectl exec -n monitoring deployment/prometheus -- \
  promtool check config /etc/prometheus/prometheus.yml
```

### Prometheus 权限错误

若看到 `permission denied` 相关错误，切换到 initContainer 方案：

```bash
kubectl apply -f prometheus-initcontainer.yaml
```

### OTel Collector 无法读取日志

检查节点上 `/home/yr/log` 目录是否存在，并确认 Pod 调度到了正确节点：

```bash
kubectl get pod -n monitoring -l app=otel-collector -o wide
```

### 清理部署

```bash
./undeploy.sh

# 完全清除包括数据
kubectl -n monitoring delete pvc --all
kubectl delete namespace monitoring
```

## Ingress 示例

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: monitoring-ingress
  namespace: monitoring
spec:
  rules:
  - host: grafana.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: grafana
            port:
              number: 3000
  - host: prometheus.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: prometheus
            port:
              number: 9090
```

## 生产环境建议

1. 替换 `grafana-secret.yaml` 中的默认密码，或使用外部 Secret 管理（Vault / ESO）
2. 为各 PVC 指定合适的 `storageClassName`
3. 配置 `HorizontalPodAutoscaler` 或资源配额
4. 在 Grafana 前配置 Ingress + TLS
5. 考虑使用 [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts) Helm Chart 替代手动部署
