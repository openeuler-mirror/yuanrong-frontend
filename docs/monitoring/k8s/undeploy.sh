#!/bin/bash

# 删除完整监控服务栈
# 包含: Prometheus / Loki / Tempo / OTel Collector / Grafana
# 使用方法: ./undeploy.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "删除监控服务栈..."

# Grafana
kubectl delete -f grafana-service.yaml              2>/dev/null || true
kubectl delete -f grafana-deployment.yaml           2>/dev/null || true
kubectl delete -f grafana-pvc.yaml                  2>/dev/null || true
kubectl delete -f grafana-dashboard-configmap.yaml  2>/dev/null || true
kubectl delete -f grafana-dashboards-provider.yaml  2>/dev/null || true
kubectl delete -f grafana-configmap.yaml            2>/dev/null || true
kubectl delete -f grafana-secret.yaml               2>/dev/null || true

# OTel Collector
kubectl delete -f otel-collector-service.yaml       2>/dev/null || true
kubectl delete -f otel-collector-deployment.yaml    2>/dev/null || true
kubectl delete -f otel-collector-configmap.yaml     2>/dev/null || true

# Tempo
kubectl delete -f tempo-service.yaml                2>/dev/null || true
kubectl delete -f tempo-deployment.yaml             2>/dev/null || true
kubectl delete -f tempo-pvc.yaml                    2>/dev/null || true
kubectl delete -f tempo-configmap.yaml              2>/dev/null || true

# Loki
kubectl delete -f loki-service.yaml                 2>/dev/null || true
kubectl delete -f loki-deployment.yaml              2>/dev/null || true
kubectl delete -f loki-pvc.yaml                     2>/dev/null || true
kubectl delete -f loki-configmap.yaml               2>/dev/null || true

# Prometheus
kubectl delete -f prometheus-service.yaml           2>/dev/null || true
kubectl delete -f prometheus-deployment.yaml        2>/dev/null || true
kubectl delete -f prometheus-pvc.yaml               2>/dev/null || true
kubectl delete -f prometheus-configmap.yaml         2>/dev/null || true
kubectl delete -f prometheus-rbac.yaml              2>/dev/null || true

echo "所有工作负载已删除"
echo ""
echo "注意: PVC 数据已保留（prometheus-pvc, loki-pvc, tempo-pvc, grafana-pvc）"
echo "      若要彻底清除数据，执行:"
echo "      kubectl -n monitoring delete pvc --all"
echo ""

# 可选: 删除 namespace（会删除残余所有资源）
read -p "是否删除 monitoring namespace? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    kubectl delete namespace monitoring
    echo "已删除 monitoring namespace"
else
    echo "保留 monitoring namespace"
fi

echo "删除完成"
