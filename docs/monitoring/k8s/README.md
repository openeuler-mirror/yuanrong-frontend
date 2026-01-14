# Kubernetes 部署 Prometheus 和 Grafana 监控

本文档介绍如何在 Kubernetes 集群中部署 Prometheus 和 Grafana 来监控 Yuanrong Frontend 服务。

## 前置要求

- Kubernetes 集群 (1.20+)
- kubectl 已配置并可以访问集群
- 集群有足够的资源（建议至少 2 CPU, 4GB 内存）
- 存储类（StorageClass）已配置（用于 PVC）

## 快速开始

### 1. 配置 Prometheus

编辑 `prometheus-configmap.yaml`，根据你的 Frontend 服务配置修改：

**方式一：使用 Kubernetes 服务发现（推荐）**

如果 Frontend 服务在 Kubernetes 中运行，Prometheus 会自动发现：

```yaml
kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
        - default  # 修改为实际的 namespace
```

确保 Frontend Service 的端口有名称 `metrics`，或使用注解：
```yaml
metadata:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8888"
    prometheus.io/path: "/metrics"
```

**方式二：使用静态配置**

如果 Frontend 服务在集群外或使用固定地址：

```yaml
static_configs:
  - targets: ['yuanrong-frontend-service:8888']  # Service名称:端口
```

### 2. 部署服务

```bash
cd docs/monitoring/k8s
chmod +x *.sh
./deploy.sh
```

### 3. 访问服务

**方式一：端口转发（推荐用于测试）**

```bash
# Prometheus
kubectl port-forward -n monitoring svc/prometheus 9090:9090

# Grafana
kubectl port-forward -n monitoring svc/grafana 3000:3000
```

然后访问：
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000

**方式二：NodePort（如果已创建）**

```bash
# 获取节点 IP
kubectl get nodes -o wide

# 访问服务
# Prometheus: http://<node-ip>:30090
# Grafana: http://<node-ip>:30300
```

**方式三：Ingress（需要配置 Ingress Controller）**

创建 Ingress 资源来暴露服务。

### 4. 删除部署

```bash
./undeploy.sh
```

## 文件说明

### 核心配置文件

- `namespace.yaml` - 创建 monitoring namespace
- `prometheus-configmap.yaml` - Prometheus 配置
- `prometheus-deployment.yaml` - Prometheus 部署
- `prometheus-service.yaml` - Prometheus 服务（ClusterIP 和 NodePort）
- `prometheus-pvc.yaml` - Prometheus 数据持久化
- `grafana-configmap.yaml` - Grafana 数据源配置
- `grafana-dashboard-configmap.yaml` - Grafana 仪表板
- `grafana-deployment.yaml` - Grafana 部署
- `grafana-service.yaml` - Grafana 服务（ClusterIP 和 NodePort）
- `grafana-pvc.yaml` - Grafana 数据持久化
- `grafana-secret.yaml` - Grafana 管理员密码（示例）

### 脚本文件

- `deploy.sh` - 一键部署所有资源
- `undeploy.sh` - 删除所有资源
- `kustomization.yaml` - Kustomize 配置文件（可选）

## 配置说明

### Prometheus 配置

Prometheus 使用 ConfigMap 存储配置，支持：

1. **Kubernetes 服务发现**: 自动发现集群中的服务
2. **静态配置**: 手动指定目标地址
3. **Relabel 配置**: 自定义标签

### Grafana 配置

- **数据源**: 自动配置 Prometheus 数据源
- **仪表板**: 自动加载预配置的仪表板
- **密码**: 通过 Secret 管理（默认 admin/admin）

### 资源限制

默认资源配置：

- **Prometheus**:
  - 请求: 512Mi 内存, 250m CPU
  - 限制: 2Gi 内存, 1000m CPU

- **Grafana**:
  - 请求: 256Mi 内存, 100m CPU
  - 限制: 512Mi 内存, 500m CPU

可根据实际需求调整。

### 存储配置

- **Prometheus**: 20Gi 持久化存储，保留 30 天数据
- **Grafana**: 10Gi 持久化存储

确保集群有可用的 StorageClass。

## 高级配置

### 使用 Kustomize

如果使用 Kustomize 管理配置：

```bash
kubectl apply -k .
```

### 配置 Ingress

创建 Ingress 资源暴露服务：

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: monitoring-ingress
  namespace: monitoring
spec:
  rules:
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
```

### 配置 RBAC（如果需要）

如果 Prometheus 需要访问 Kubernetes API 进行服务发现，可能需要 RBAC：

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: prometheus
  namespace: monitoring
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus
rules:
- apiGroups: [""]
  resources:
  - nodes
  - nodes/proxy
  - services
  - endpoints
  - pods
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: prometheus
subjects:
- kind: ServiceAccount
  name: prometheus
  namespace: monitoring
```

然后在 Deployment 中指定 ServiceAccount。

### 配置持久化存储类

如果使用特定的 StorageClass：

```yaml
# prometheus-pvc.yaml
spec:
  storageClassName: fast-ssd  # 取消注释并修改
  ...
```

## 监控 Frontend 服务

### 如果 Frontend 在 Kubernetes 中

1. **确保 Service 端口有名称**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: yuanrong-frontend
spec:
  ports:
  - name: metrics  # 重要：端口名称
    port: 8888
    targetPort: 8888
```

2. **或使用注解**:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: yuanrong-frontend
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8888"
    prometheus.io/path: "/metrics"
```

### 如果 Frontend 在集群外

在 `prometheus-configmap.yaml` 中使用静态配置：

```yaml
- job_name: 'yuanrong-frontend'
  static_configs:
    - targets: ['external-host:8888']
```

## 故障排查

### 检查 Pod 状态

```bash
kubectl get pods -n monitoring
kubectl describe pod <pod-name> -n monitoring
kubectl logs <pod-name> -n monitoring
```

### 检查服务

```bash
kubectl get svc -n monitoring
kubectl describe svc prometheus -n monitoring
```

### 检查配置

```bash
# 查看 Prometheus 配置
kubectl get configmap prometheus-config -n monitoring -o yaml

# 验证 Prometheus 配置
kubectl exec -n monitoring deployment/prometheus -- promtool check config /etc/prometheus/prometheus.yml
```

### 检查存储

```bash
kubectl get pvc -n monitoring
kubectl describe pvc prometheus-pvc -n monitoring
```

### Prometheus 权限错误

如果看到类似 `permission denied` 的错误（特别是 `/prometheus/queries.active`）：

**方案一：使用 securityContext（已配置）**

当前的 `prometheus-deployment.yaml` 已经配置了 `securityContext`，应该可以正常工作。如果仍有问题，检查 PVC 的权限：

```bash
# 检查 Pod 状态
kubectl describe pod -l app=prometheus -n monitoring

# 如果使用 initContainer 方案，使用以下文件
kubectl apply -f prometheus-initcontainer.yaml
```

**方案二：使用 initContainer**

如果 `securityContext` 方案不工作，可以使用 `prometheus-initcontainer.yaml`，它会在启动前修复权限：

```bash
kubectl apply -f prometheus-initcontainer.yaml
```

**方案三：禁用 active query tracker**

如果不需要查询追踪，可以在 Deployment 中添加参数（已添加）：
- `--query.max-concurrency=0`
- `--query.max-samples=0`

### 测试连接

```bash
# 在 Prometheus Pod 中测试连接
kubectl exec -n monitoring deployment/prometheus -- wget -O- http://yuanrong-frontend-service:8888/metrics
```

## 生产环境建议

1. **使用 Helm Chart**: 考虑使用 Prometheus Operator 或社区 Helm Chart
2. **配置高可用**: 部署多个 Prometheus 实例
3. **使用 Alertmanager**: 配置告警规则和通知
4. **配置资源配额**: 设置 Namespace 资源限制
5. **定期备份**: 备份 Prometheus 和 Grafana 数据
6. **使用 TLS**: 配置 Ingress 使用 HTTPS
7. **监控资源使用**: 监控 Prometheus 和 Grafana 的资源消耗

## 参考资源

- [Kubernetes 官方文档](https://kubernetes.io/docs/)
- [Prometheus Kubernetes 配置](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config)
- [Grafana Kubernetes 部署](https://grafana.com/docs/grafana/latest/setup-grafana/installation/kubernetes/)
- [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
