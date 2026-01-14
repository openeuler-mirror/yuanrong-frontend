#!/bin/bash

# 部署 Prometheus 和 Grafana 到 Kubernetes 集群
# 使用方法: ./deploy.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 检查 kubectl 是否安装
if ! command -v kubectl &> /dev/null; then
    echo "错误: kubectl 未安装"
    echo "请访问 https://kubernetes.io/docs/tasks/tools/ 安装 kubectl"
    exit 1
fi

# 检查 Kubernetes 连接
if ! kubectl cluster-info &> /dev/null; then
    echo "错误: 无法连接到 Kubernetes 集群"
    echo "请检查 kubeconfig 配置"
    exit 1
fi

echo "开始部署监控服务到 Kubernetes..."

# 创建 namespace
echo "创建 monitoring namespace..."
kubectl apply -f namespace.yaml

# 创建 Secret（如果不存在）
if ! kubectl get secret grafana-secret -n monitoring &> /dev/null; then
    echo "创建 Grafana Secret..."
    kubectl create secret generic grafana-secret \
        --from-literal=admin-password='admin' \
        -n monitoring \
        --dry-run=client -o yaml | kubectl apply -f -
else
    echo "Grafana Secret 已存在，跳过创建"
fi

# 部署 Prometheus
echo "部署 Prometheus..."
kubectl apply -f prometheus-configmap.yaml
kubectl apply -f prometheus-pvc.yaml
kubectl apply -f prometheus-deployment.yaml
kubectl apply -f prometheus-service.yaml

# 部署 Grafana
echo "部署 Grafana..."
kubectl apply -f grafana-secret.yaml
kubectl apply -f grafana-configmap.yaml
kubectl apply -f grafana-dashboard-configmap.yaml
kubectl apply -f grafana-pvc.yaml
kubectl apply -f grafana-deployment.yaml
kubectl apply -f grafana-service.yaml

echo ""
echo "等待 Pod 启动..."
kubectl wait --for=condition=ready pod -l app=prometheus -n monitoring --timeout=120s || true
kubectl wait --for=condition=ready pod -l app=grafana -n monitoring --timeout=120s || true

echo ""
echo "=========================================="
echo "部署完成"
echo "=========================================="
echo ""
echo "查看 Pod 状态:"
kubectl get pods -n monitoring
echo ""
echo "查看 Service:"
kubectl get svc -n monitoring
echo ""
echo "访问服务:"
echo "  Prometheus (端口转发): kubectl port-forward -n monitoring svc/prometheus 9090:9090"
echo "  Grafana (端口转发):    kubectl port-forward -n monitoring svc/grafana 3000:3000"
echo ""
echo "或使用 NodePort (如果已创建):"
echo "  Prometheus: http://<node-ip>:30090"
echo "  Grafana:    http://<node-ip>:30300"
echo ""
echo "默认 Grafana 用户名/密码: admin/admin"
echo ""
echo "删除部署: ./undeploy.sh"
echo "=========================================="
