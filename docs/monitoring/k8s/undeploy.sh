#!/bin/bash

# 删除 Prometheus 和 Grafana 部署
# 使用方法: ./undeploy.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "删除监控服务..."

# 使用 kustomize 删除（如果使用）
# kubectl delete -k . 2>/dev/null || true

# 或手动删除资源
kubectl delete -f grafana-service.yaml 2>/dev/null || true
kubectl delete -f grafana-deployment.yaml 2>/dev/null || true
kubectl delete -f grafana-pvc.yaml 2>/dev/null || true
kubectl delete -f grafana-dashboard-configmap.yaml 2>/dev/null || true
kubectl delete -f grafana-configmap.yaml 2>/dev/null || true
kubectl delete -f grafana-secret.yaml 2>/dev/null || true

kubectl delete -f prometheus-service.yaml 2>/dev/null || true
kubectl delete -f prometheus-deployment.yaml 2>/dev/null || true
kubectl delete -f prometheus-pvc.yaml 2>/dev/null || true
kubectl delete -f prometheus-configmap.yaml 2>/dev/null || true

# 可选: 删除 namespace（会删除所有资源）
read -p "是否删除 monitoring namespace? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    kubectl delete namespace monitoring
    echo "已删除 monitoring namespace"
else
    echo "保留 monitoring namespace"
fi

echo "删除完成"
