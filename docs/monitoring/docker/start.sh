#!/bin/bash

# 使用 Docker Compose 启动 Prometheus 和 Grafana 监控服务
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

# 修复 Prometheus 数据目录权限
echo "检查并修复 Prometheus 数据目录权限..."
if [ -d "prometheus-data" ]; then
    # Prometheus 容器使用 nobody 用户 (UID 65534)
    if [ "$(id -u)" = "0" ]; then
        chown -R 65534:65534 prometheus-data 2>/dev/null || true
        chmod -R 755 prometheus-data 2>/dev/null || true
    elif command -v sudo &> /dev/null; then
        sudo chown -R 65534:65534 prometheus-data 2>/dev/null || true
        sudo chmod -R 755 prometheus-data 2>/dev/null || true
    fi
fi

# 检查配置文件是否存在
if [ ! -f "prometheus.yml" ]; then
    echo "错误: prometheus.yml 文件不存在"
    exit 1
fi

if [ ! -f "docker-compose.yml" ]; then
    echo "错误: docker-compose.yml 文件不存在"
    exit 1
fi

# 检查仪表板文件是否存在
if [ ! -f "grafana-dashboards/yuanrong-frontend.json" ]; then
    echo "警告: 仪表板文件不存在，将创建空目录"
    touch grafana-dashboards/.gitkeep
fi

echo "启动 Prometheus 和 Grafana 容器..."

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
echo "Prometheus: http://localhost:9090"
echo "Grafana:    http://localhost:3000"
echo "默认用户名/密码: admin/admin"
echo ""
echo "查看日志:"
echo "  Prometheus: docker logs -f prometheus"
echo "  Grafana:    docker logs -f grafana"
echo ""
echo "停止服务: ./stop.sh"
echo "=========================================="
