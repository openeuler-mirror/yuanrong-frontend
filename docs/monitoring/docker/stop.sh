#!/bin/bash

# 停止 Prometheus 和 Grafana 监控服务
# 使用方法: ./stop.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "停止 Prometheus 和 Grafana 容器..."

# 使用 docker-compose 或 docker compose
if command -v docker-compose &> /dev/null; then
    docker-compose down
else
    docker compose down
fi

echo "监控服务已停止"

# 可选: 删除数据卷（取消注释以启用）
# echo "删除数据卷..."
# docker volume rm monitoring_prometheus-data monitoring_grafana-data 2>/dev/null || true
