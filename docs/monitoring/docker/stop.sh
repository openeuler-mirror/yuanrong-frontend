#!/bin/bash

# 停止完整监控服务栈
# 包含: Loki, Tempo, OTel Collector, Prometheus, Grafana
# 使用方法: ./stop.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "停止监控服务栈 (Loki / Tempo / OTel Collector / Prometheus / Grafana)..."

# 使用 docker-compose 或 docker compose
if command -v docker-compose &> /dev/null; then
    docker-compose down
else
    docker compose down
fi

echo "监控服务已停止"

# 可选: 删除持久化数据（取消注释以启用，操作不可逆）
# echo "删除持久化数据..."
# rm -rf prometheus-data grafana-data loki-data tempo-data
