#!/bin/bash

# 部署完整监控服务栈到 Kubernetes 集群
# 包含: Prometheus / Loki / Tempo / OTel Collector / Grafana
# 使用方法: ./deploy.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

NAMESPACE="monitoring"

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

echo "开始部署监控服务栈到 Kubernetes..."

# 创建 namespace
echo "[1/6] 创建 monitoring namespace..."
kubectl apply -f namespace.yaml

# 部署 Prometheus（Tempo / OTel 依赖其 remote_write，先部署）
echo "[2/6] 部署 Prometheus..."
kubectl apply -f prometheus-rbac.yaml
kubectl apply -f prometheus-configmap.yaml
kubectl apply -f prometheus-pvc.yaml
kubectl apply -f prometheus-deployment.yaml
kubectl apply -f prometheus-service.yaml

# 部署 Loki
echo "[3/6] 部署 Loki..."
kubectl apply -f loki-configmap.yaml
kubectl apply -f loki-pvc.yaml
kubectl apply -f loki-deployment.yaml
kubectl apply -f loki-service.yaml

# 部署 Tempo（initContainer 等待 Prometheus 就绪）
echo "[4/6] 部署 Tempo..."
kubectl apply -f tempo-configmap.yaml
kubectl apply -f tempo-pvc.yaml
kubectl apply -f tempo-deployment.yaml
kubectl apply -f tempo-service.yaml

# 部署 OTel Collector
echo "[5/6] 部署 OTel Collector..."
kubectl apply -f otel-collector-configmap.yaml
kubectl apply -f otel-collector-deployment.yaml
kubectl apply -f otel-collector-service.yaml

# 部署 Grafana
echo "[6/6] 部署 Grafana..."
kubectl apply -f grafana-secret.yaml
kubectl apply -f grafana-configmap.yaml
kubectl apply -f grafana-dashboards-provider.yaml
kubectl apply -f grafana-dashboard-configmap.yaml
kubectl apply -f grafana-pvc.yaml
kubectl apply -f grafana-deployment.yaml
kubectl apply -f grafana-service.yaml

echo ""
echo "等待所有 Pod 就绪..."
kubectl wait --for=condition=ready pod -l app=prometheus     -n $NAMESPACE --timeout=120s || true
kubectl wait --for=condition=ready pod -l app=loki           -n $NAMESPACE --timeout=120s || true
kubectl wait --for=condition=ready pod -l app=tempo          -n $NAMESPACE --timeout=180s || true
kubectl wait --for=condition=ready pod -l app=otel-collector -n $NAMESPACE --timeout=120s || true
kubectl wait --for=condition=ready pod -l app=grafana        -n $NAMESPACE --timeout=120s || true

echo ""
echo "=========================================="
echo "部署完成"
echo "=========================================="
echo ""
echo "查看 Pod 状态:"
kubectl get pods -n $NAMESPACE
echo ""
echo "查看 Service:"
kubectl get svc -n $NAMESPACE
echo ""
echo "访问服务（端口转发）:"
echo "  Prometheus:   kubectl port-forward -n $NAMESPACE svc/prometheus 9090:9090"
echo "  Grafana:      kubectl port-forward -n $NAMESPACE svc/grafana 3000:3000"
echo "  Loki:         kubectl port-forward -n $NAMESPACE svc/loki 3100:3100"
echo "  Tempo:        kubectl port-forward -n $NAMESPACE svc/tempo 3200:3200"
echo "  OTel gRPC:    kubectl port-forward -n $NAMESPACE svc/otel-collector 4317:4317"
echo ""
echo "NodePort 访问（如果已创建）:"
echo "  Prometheus: http://<node-ip>:30090"
echo "  Grafana:    http://<node-ip>:30300"
echo ""
echo "默认 Grafana 用户名/密码: admin/admin"
echo ""
echo "提示: OTel Collector 通过 hostPath 读取 /home/yr/log"
echo "      如日志在特定节点，请在 otel-collector-deployment.yaml 中配置 nodeSelector"
echo ""
echo "删除部署: ./undeploy.sh"
echo "=========================================="
