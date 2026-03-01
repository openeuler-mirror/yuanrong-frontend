#!/bin/bash

# 使用 Docker Compose 启动完整监控服务栈
# 包含: Loki, Tempo, OTel Collector, Prometheus, Grafana
# 使用方法: ./start.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 检查 Docker 是否安装
if ! command -v docker &> /dev/null; then
    echo "错误: Docker 未安装"
    echo "请访问 https://docs.docker.com/get-docker/ 安装 Docker"
    exit 1
fi

# 检查 Docker Compose 是否安装
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "错误: Docker Compose 未安装"
    echo "请访问 https://docs.docker.com/compose/install/ 安装 Docker Compose"
    exit 1
fi

# 创建必要的目录
mkdir -p prometheus-data
mkdir -p grafana-data
mkdir -p grafana-provisioning/datasources
mkdir -p grafana-provisioning/dashboards
mkdir -p grafana-dashboards
mkdir -p loki-data
mkdir -p tempo-data

# 修复数据目录权限（各容器均以 root 运行，确保宿主机目录可写）
echo "检查并修复数据目录权限..."
for dir in prometheus-data grafana-data loki-data tempo-data; do
    if [ -d "$dir" ]; then
        chmod -R 755 "$dir" 2>/dev/null || true
    fi
done

# 检查配置文件是否存在
for cfg in docker-compose.yml prometheus.yml loki-config.yaml tempo-config.yaml otel-collector-config.yaml; do
    if [ ! -f "$cfg" ]; then
        echo "错误: $cfg 文件不存在"
        exit 1
    fi
done

# 检查仪表板文件是否存在
if [ ! -f "grafana-dashboards/yuanrong-frontend.json" ]; then
    echo "警告: 仪表板文件不存在，将创建空目录"
    touch grafana-dashboards/.gitkeep
fi

echo "启动监控服务栈 (Loki / Tempo / OTel Collector / Prometheus / Grafana)..."

# 使用 docker-compose 或 docker compose
if command -v docker-compose &> /dev/null; then
    docker-compose up -d
else
    docker compose up -d
fi

echo ""
echo "等待服务启动..."
sleep 5

# 检查服务状态
if command -v docker-compose &> /dev/null; then
    docker-compose ps
else
    docker compose ps
fi

echo ""
echo "=========================================="
echo "监控服务已启动"
echo "=========================================="
echo "Prometheus:    http://localhost:9090"
echo "Grafana:       http://localhost:3001  (admin/admin)"
echo "Loki:          http://localhost:3100"
echo "Tempo:         http://localhost:3200"
echo "OTel gRPC:     localhost:4317"
echo "OTel HTTP:     localhost:4318"
echo "OTel Metrics:  http://localhost:8889"
echo ""
echo "查看日志:"
echo "  docker logs -f prometheus"
echo "  docker logs -f grafana"
echo "  docker logs -f loki"
echo "  docker logs -f tempo"
echo "  docker logs -f otel-collector"
echo ""
echo "停止服务: ./stop.sh"
echo "=========================================="
