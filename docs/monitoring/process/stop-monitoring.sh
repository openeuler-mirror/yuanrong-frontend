#!/bin/bash

# 停止完整监控栈（裸进程模式）
# 使用方法: ./stop-monitoring.sh

# 用精确进程名（-x）而非命令行模式（-f），避免 pkill 误杀调用方自身
_stop() {
    local name="$1"
    if pkill -x "$name" 2>/dev/null; then
        echo "  $name 已停止"
    else
        echo "  $name 未运行"
    fi
}

# 若 SIGTERM 后进程仍在，强制 SIGKILL
_stop_hard() {
    local name="$1"
    _stop "$name"
    sleep 1
    pkill -9 -x "$name" 2>/dev/null || true
}

echo "停止监控栈..."

_stop grafana
_stop prometheus
_stop otelcol-contrib
_stop_hard tempo
_stop_hard loki

echo ""
echo "监控栈已停止"
