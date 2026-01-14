#!/bin/bash

# 修复 Prometheus 数据目录权限的脚本
# 使用方法: ./fix-permissions.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "修复 Prometheus 数据目录权限..."

# Prometheus 容器使用 nobody 用户 (UID 65534)
PROMETHEUS_UID=65534
PROMETHEUS_GID=65534

# 创建数据目录（如果不存在）
mkdir -p prometheus-data

# 设置正确的权限
if [ "$(id -u)" = "0" ]; then
    # 如果以 root 运行，直接修改权限
    chown -R ${PROMETHEUS_UID}:${PROMETHEUS_GID} prometheus-data
    chmod -R 755 prometheus-data
    echo "权限已修复（root 用户）"
else
    # 如果不是 root，使用 sudo
    if command -v sudo &> /dev/null; then
        sudo chown -R ${PROMETHEUS_UID}:${PROMETHEUS_GID} prometheus-data
        sudo chmod -R 755 prometheus-data
        echo "权限已修复（使用 sudo）"
    else
        echo "警告: 需要 root 权限来修复权限"
        echo "请运行: sudo chown -R ${PROMETHEUS_UID}:${PROMETHEUS_GID} prometheus-data"
        echo "或使用: docker-compose down && docker-compose up -d"
    fi
fi

echo "完成"
