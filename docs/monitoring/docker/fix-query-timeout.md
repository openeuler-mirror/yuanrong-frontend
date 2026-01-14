# 修复 Prometheus 查询卡住问题

## 问题原因

在 `docker-compose.yml` 中设置了：
- `--query.max-concurrency=0`
- `--query.max-samples=0`

这两个参数设置为 0 会导致查询被阻塞或无限等待。

## 解决方案

### 方法 1: 移除这些参数（推荐）

已从配置中移除这些参数，Prometheus 将使用默认值：
- `query.max-concurrency`: 默认 20
- `query.max-samples`: 默认 50000000

### 方法 2: 设置合理的值

如果需要限制查询，可以设置合理的值：

```yaml
command:
  - '--query.max-concurrency=20'  # 最大并发查询数
  - '--query.max-samples=50000000' # 最大样本数
```

## 应用修复

### Docker 部署

```bash
cd docs/monitoring/docker

# 重启 Prometheus 容器
docker-compose restart prometheus

# 或重新创建
docker-compose up -d prometheus
```

### Kubernetes 部署

```bash
cd docs/monitoring/k8s

# 应用更新后的配置
kubectl apply -f prometheus-deployment.yaml

# 重启 Pod
kubectl rollout restart deployment/prometheus -n monitoring
```

## 验证修复

```bash
# 测试查询（应该很快返回）
curl 'http://localhost:9090/api/v1/query?query=up'

# 测试指标查询
curl 'http://localhost:9090/api/v1/query?query=function_invocations_total'
```

查询应该在几秒内返回结果，不再卡住。
