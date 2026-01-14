#!/bin/bash

# Prometheus 范围查询脚本
# 使用方法: ./query-range.sh '<promql-query>' [start] [end] [step]
# 示例: ./query-range.sh 'rate(function_invocations_total[5m])' '1h' 'now' '15s'

set -e

PROMETHEUS_URL="${PROMETHEUS_URL:-http://localhost:9090}"
QUERY="$1"
START="${2:-1h}"  # 默认 1 小时前
END="${3:-now}"   # 默认现在
STEP="${4:-15s}"  # 默认 15 秒

if [ -z "$QUERY" ]; then
    echo "用法: $0 '<promql-query>' [start] [end] [step]"
    echo ""
    echo "参数:"
    echo "  query: PromQL 查询语句（必需）"
    echo "  start: 开始时间（默认: 1h，支持相对时间如 '1h' 或绝对时间如 '2026-01-14T10:00:00Z'）"
    echo "  end:   结束时间（默认: now）"
    echo "  step:  步长（默认: 15s）"
    echo ""
    echo "示例:"
    echo "  $0 'rate(function_invocations_total[5m])'"
    echo "  $0 'rate(function_invocations_total[5m])' '2h' 'now' '30s'"
    echo "  $0 'rate(function_invocations_total[5m])' '2026-01-14T10:00:00Z' '2026-01-14T11:00:00Z' '1m'"
    exit 1
fi

# 检查 jq 是否安装
if ! command -v jq &> /dev/null; then
    USE_JQ=false
else
    USE_JQ=true
fi

# 转换相对时间为时间戳
if [[ "$START" =~ ^[0-9]+[smhd]$ ]]; then
    # 相对时间，转换为时间戳
    START_TS=$(date -d "$START ago" +%s 2>/dev/null || date -v-${START} +%s 2>/dev/null || echo "$START")
else
    # 绝对时间或已经是时间戳
    START_TS=$(date -d "$START" +%s 2>/dev/null || echo "$START")
fi

if [ "$END" = "now" ]; then
    END_TS=$(date +%s)
else
    END_TS=$(date -d "$END" +%s 2>/dev/null || echo "$END")
fi

echo "查询: $QUERY"
echo "时间范围: $START -> $END (步长: $STEP)"
echo "Prometheus: $PROMETHEUS_URL"
echo ""

# 执行查询
RESPONSE=$(curl -s -G "${PROMETHEUS_URL}/api/v1/query_range" \
  --data-urlencode "query=${QUERY}" \
  --data-urlencode "start=${START_TS}" \
  --data-urlencode "end=${END_TS}" \
  --data-urlencode "step=${STEP}")

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
    echo "结果（前 10 个数据点）:"
    echo "$RESPONSE" | jq '.data.result[] | {
        metric: .metric,
        values: .values[0:10]
    }'
    
    # 统计
    RESULT_COUNT=$(echo "$RESPONSE" | jq '.data.result | length')
    echo ""
    echo "共 $RESULT_COUNT 个时间序列"
    
    # 保存完整结果到文件
    OUTPUT_FILE="metrics-range-$(date +%Y%m%d-%H%M%S).json"
    echo "$RESPONSE" > "$OUTPUT_FILE"
    echo "完整结果已保存到: $OUTPUT_FILE"
else
    echo "$RESPONSE"
fi
