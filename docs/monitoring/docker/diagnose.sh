#!/bin/bash

# Grafana 指标显示问题诊断脚本
# 使用方法: ./diagnose.sh

set -e

PROMETHEUS_URL="${PROMETHEUS_URL:-http://localhost:9090}"
GRAFANA_URL="${GRAFANA_URL:-http://localhost:3000}"
GRAFANA_USER="${GRAFANA_USER:-admin}"
GRAFANA_PASS="${GRAFANA_PASS:-admin}"

echo "=========================================="
echo "Grafana 指标显示问题诊断"
echo "=========================================="
echo ""

# 检查依赖
if ! command -v jq &> /dev/null; then
    echo "警告: jq 未安装，部分功能可能无法使用"
    echo "安装: sudo apt-get install jq 或 sudo yum install jq"
    USE_JQ=false
else
    USE_JQ=true
fi

# 1. 检查 Prometheus 容器
echo "1. 检查 Prometheus 容器状态..."
if docker ps --format "{{.Names}}" | grep -q "^prometheus$"; then
    echo "   ✓ Prometheus 容器运行中"
    PROMETHEUS_IP=$(docker inspect prometheus --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -1)
    echo "   IP 地址: $PROMETHEUS_IP"
else
    echo "   ✗ Prometheus 容器未运行"
    echo "   请运行: docker-compose up -d prometheus"
    exit 1
fi

# 2. 检查 Prometheus 健康状态
echo ""
echo "2. 检查 Prometheus 健康状态..."
if curl -s -f "${PROMETHEUS_URL}/-/healthy" > /dev/null 2>&1; then
    echo "   ✓ Prometheus 健康检查通过"
else
    echo "   ✗ Prometheus 健康检查失败"
    echo "   请检查: curl ${PROMETHEUS_URL}/-/healthy"
fi

# 3. 检查 Targets
echo ""
echo "3. 检查 Prometheus Targets..."
if [ "$USE_JQ" = true ]; then
    TARGETS=$(curl -s "${PROMETHEUS_URL}/api/v1/targets" 2>/dev/null)
    if [ -n "$TARGETS" ]; then
        echo "$TARGETS" | jq -r '.data.activeTargets[] | "   \(.labels.job // "unknown"): \(.health) - \(.lastError // "OK")"' 2>/dev/null || echo "   ⚠ 无法解析 Targets"
        
        # 检查 yuanrong-frontend target
        FRONTEND_TARGET=$(echo "$TARGETS" | jq -r '.data.activeTargets[] | select(.labels.job=="yuanrong-frontend") | .health' 2>/dev/null)
        if [ "$FRONTEND_TARGET" = "up" ]; then
            echo "   ✓ yuanrong-frontend target 状态: UP"
        elif [ -n "$FRONTEND_TARGET" ]; then
            echo "   ✗ yuanrong-frontend target 状态: $FRONTEND_TARGET"
            ERROR=$(echo "$TARGETS" | jq -r '.data.activeTargets[] | select(.labels.job=="yuanrong-frontend") | .lastError' 2>/dev/null)
            if [ -n "$ERROR" ] && [ "$ERROR" != "null" ]; then
                echo "   错误信息: $ERROR"
            fi
        else
            echo "   ⚠ 未找到 yuanrong-frontend target"
        fi
    else
        echo "   ✗ 无法获取 Targets 信息"
    fi
else
    echo "   ⚠ 需要 jq 来解析 Targets 信息"
fi

# 4. 检查指标是否存在
echo ""
echo "4. 检查指标是否存在..."
METRICS_QUERY=$(curl -s "${PROMETHEUS_URL}/api/v1/query?query=function_invocations_total" 2>/dev/null)
if [ "$USE_JQ" = true ]; then
    METRICS_COUNT=$(echo "$METRICS_QUERY" | jq -r '.data.result | length' 2>/dev/null)
    STATUS=$(echo "$METRICS_QUERY" | jq -r '.status' 2>/dev/null)
    
    if [ "$STATUS" = "success" ]; then
        if [ "$METRICS_COUNT" != "0" ] && [ -n "$METRICS_COUNT" ]; then
            echo "   ✓ 找到 $METRICS_COUNT 个 function_invocations_total 时间序列"
            
            # 显示前几个指标
            echo ""
            echo "   指标示例:"
            echo "$METRICS_QUERY" | jq -r '.data.result[0:3][] | "     \(.metric | to_entries | map("\(.key)=\"\(.value)\"") | join(", ")) = \(.value[1])"' 2>/dev/null
        else
            echo "   ✗ 未找到 function_invocations_total 指标"
            echo "   请检查:"
            echo "   1. Frontend 服务是否运行"
            echo "   2. Metrics 端点是否可访问: curl http://localhost:8888/metrics"
            echo "   3. Prometheus 配置中的 targets 是否正确"
        fi
    else
        ERROR_MSG=$(echo "$METRICS_QUERY" | jq -r '.error // .errorType' 2>/dev/null)
        echo "   ✗ 查询失败: $ERROR_MSG"
    fi
else
    if echo "$METRICS_QUERY" | grep -q '"status":"success"'; then
        echo "   ✓ 查询成功（需要 jq 查看详细信息）"
    else
        echo "   ✗ 查询失败"
    fi
fi

# 5. 检查 Grafana 容器
echo ""
echo "5. 检查 Grafana 容器状态..."
if docker ps --format "{{.Names}}" | grep -q "^grafana$"; then
    echo "   ✓ Grafana 容器运行中"
    GRAFANA_IP=$(docker inspect grafana --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -1)
    echo "   IP 地址: $GRAFANA_IP"
else
    echo "   ✗ Grafana 容器未运行"
    echo "   请运行: docker-compose up -d grafana"
    exit 1
fi

# 6. 检查 Grafana 健康状态
echo ""
echo "6. 检查 Grafana 健康状态..."
if curl -s -f "${GRAFANA_URL}/api/health" > /dev/null 2>&1; then
    echo "   ✓ Grafana 健康检查通过"
else
    echo "   ✗ Grafana 健康检查失败"
    echo "   请检查: curl ${GRAFANA_URL}/api/health"
fi

# 7. 测试数据源连接
echo ""
echo "7. 测试 Grafana 数据源连接..."
DS_RESPONSE=$(curl -s -u "${GRAFANA_USER}:${GRAFANA_PASS}" "${GRAFANA_URL}/api/datasources" 2>/dev/null)
if [ "$USE_JQ" = true ]; then
    PROM_DS=$(echo "$DS_RESPONSE" | jq -r '.[] | select(.type=="prometheus") | .id' 2>/dev/null | head -1)
    if [ -n "$PROM_DS" ]; then
        echo "   ✓ 找到 Prometheus 数据源 (ID: $PROM_DS)"
        
        # 测试连接
        DS_TEST=$(curl -s -u "${GRAFANA_USER}:${GRAFANA_PASS}" "${GRAFANA_URL}/api/datasources/proxy/${PROM_DS}/api/v1/query?query=up" 2>/dev/null)
        DS_STATUS=$(echo "$DS_TEST" | jq -r '.status' 2>/dev/null)
        
        if [ "$DS_STATUS" = "success" ]; then
            echo "   ✓ Grafana 可以连接到 Prometheus"
        else
            echo "   ✗ Grafana 无法连接到 Prometheus"
            DS_ERROR=$(echo "$DS_TEST" | jq -r '.error // .errorType' 2>/dev/null)
            if [ -n "$DS_ERROR" ] && [ "$DS_ERROR" != "null" ]; then
                echo "   错误: $DS_ERROR"
            fi
        fi
        
        # 检查数据源 URL
        DS_URL=$(echo "$DS_RESPONSE" | jq -r '.[] | select(.type=="prometheus") | .url' 2>/dev/null | head -1)
        echo "   数据源 URL: $DS_URL"
        if [ "$DS_URL" != "http://prometheus:9090" ]; then
            echo "   ⚠ 警告: 数据源 URL 可能不正确（Docker 网络应使用 http://prometheus:9090）"
        fi
    else
        echo "   ✗ 未找到 Prometheus 数据源"
        echo "   请检查 Grafana 数据源配置"
    fi
else
    echo "   ⚠ 需要 jq 来测试数据源连接"
fi

# 8. 测试 Grafana 查询
echo ""
echo "8. 测试 Grafana 中的查询..."
if [ -n "$PROM_DS" ] && [ "$USE_JQ" = true ]; then
    GRAFANA_QUERY=$(curl -s -u "${GRAFANA_USER}:${GRAFANA_PASS}" \
        "${GRAFANA_URL}/api/datasources/proxy/${PROM_DS}/api/v1/query?query=function_invocations_total" 2>/dev/null)
    GRAFANA_STATUS=$(echo "$GRAFANA_QUERY" | jq -r '.status' 2>/dev/null)
    
    if [ "$GRAFANA_STATUS" = "success" ]; then
        GRAFANA_COUNT=$(echo "$GRAFANA_QUERY" | jq -r '.data.result | length' 2>/dev/null)
        if [ "$GRAFANA_COUNT" != "0" ] && [ -n "$GRAFANA_COUNT" ]; then
            echo "   ✓ Grafana 可以查询到 $GRAFANA_COUNT 个指标"
        else
            echo "   ✗ Grafana 查询成功但无数据"
        fi
    else
        echo "   ✗ Grafana 查询失败"
        GRAFANA_ERROR=$(echo "$GRAFANA_QUERY" | jq -r '.error // .errorType' 2>/dev/null)
        if [ -n "$GRAFANA_ERROR" ] && [ "$GRAFANA_ERROR" != "null" ]; then
            echo "   错误: $GRAFANA_ERROR"
        fi
    fi
fi

# 9. 检查网络连接
echo ""
echo "9. 检查网络连接..."
echo "   从 Prometheus 容器测试 Frontend metrics 端点..."
if docker exec prometheus wget -q -O- http://host.docker.internal:8888/metrics 2>/dev/null | grep -q "function_invocations_total"; then
    echo "   ✓ Prometheus 可以访问 Frontend metrics"
else
    echo "   ✗ Prometheus 无法访问 Frontend metrics"
    echo "   请检查:"
    echo "   1. Frontend 服务是否运行在 8888 端口"
    echo "   2. prometheus.yml 中的 targets 配置是否正确"
    echo "   3. 网络连接是否正常"
fi

echo ""
echo "=========================================="
echo "诊断完成"
echo "=========================================="
echo ""
echo "下一步操作:"
echo "1. 如果 Prometheus 中没有数据，检查 Targets 状态"
echo "2. 如果 Grafana 无法连接 Prometheus，检查数据源 URL"
echo "3. 在 Grafana Explore 中测试查询: function_invocations_total"
echo "4. 检查仪表板的时间范围和查询语句"
