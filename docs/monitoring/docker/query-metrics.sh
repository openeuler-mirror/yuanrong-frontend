#!/bin/bash

# Prometheus 指标查询脚本
# 使用方法: ./query-metrics.sh '<promql-query>'
# 示例: ./query-metrics.sh 'function_invocations_total'

set -e

PROMETHEUS_URL="${PROMETHEUS_URL:-http://localhost:9090}"
QUERY="$1"

if [ -z "$QUERY" ]; then
    echo "用法: $0 '<promql-query>'"
    echo ""
    echo "示例:"
    echo "  $0 'function_invocations_total'"
    echo "  $0 'rate(function_invocations_total[5m])'"
    echo "  $0 'function_invocations_total{function_name=\"my-function\"}'"
    echo ""
    echo "环境变量:"
    echo "  PROMETHEUS_URL: Prometheus 地址（默认: http://localhost:9090）"
    exit 1
fi

# 检查 jq 是否安装
if ! command -v jq &> /dev/null; then
    echo "警告: jq 未安装，将显示原始 JSON 输出"
    echo "安装: sudo apt-get install jq 或 sudo yum install jq"
    echo ""
    USE_JQ=false
else
    USE_JQ=true
fi

echo "查询: $QUERY"
echo "Prometheus: $PROMETHEUS_URL"
echo ""

# 执行查询
RESPONSE=$(curl -s -G "${PROMETHEUS_URL}/api/v1/query" \
  --data-urlencode "query=${QUERY}")

# 检查响应状态
STATUS=$(echo "$RESPONSE" | $([ "$USE_JQ" = true ] && echo "jq -r .status" || echo "grep -o '\"status\":\"[^\"]*\"' | cut -d'\"' -f4"))

if [ "$STATUS" != "success" ]; then
    echo "错误: 查询失败"
    if [ "$USE_JQ" = true ]; then
        echo "$RESPONSE" | jq '.'
    else
        echo "$RESPONSE"
    fi
    exit 1
fi

# 显示结果
if [ "$USE_JQ" = true ]; then
    echo "结果:"
    echo "$RESPONSE" | jq '.data.result[] | {
        metric: .metric,
        value: .value[1],
        timestamp: .value[0]
    }'
    
    # 统计结果数量
    COUNT=$(echo "$RESPONSE" | jq '.data.result | length')
    echo ""
    echo "共 $COUNT 个结果"
else
    echo "$RESPONSE"
fi
