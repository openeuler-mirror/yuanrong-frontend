# 手动查询 Prometheus 指标指南

本文档介绍如何手动查询 Prometheus 指标，包括通过 Web UI、HTTP API 和命令行等方式。

## 方法一：通过 Prometheus Web UI 查询（推荐）

### 1. 访问 Prometheus Web UI

- **进程部署**: http://localhost:9090
- **Docker 部署**: http://localhost:9090
- **Kubernetes 部署**: 
  - 端口转发: `kubectl port-forward -n monitoring svc/prometheus 9090:9090`
  - 然后访问: http://localhost:9090

### 2. 使用 Graph 页面查询

1. 点击顶部菜单 "Graph"
2. 在查询框中输入 PromQL 查询语句
3. 点击 "Execute" 或按 Enter
4. 查看结果（表格或图形）

### 3. 常用查询示例

**查询所有函数调用总数**
```
function_invocations_total
```

**查询特定函数的调用次数**
```
function_invocations_total{function_name="your-function-name"}
```

**查询函数调用速率（每秒请求数）**
```
sum(rate(function_invocations_total[5m])) by (function_name)
```

**查询平均响应时间**
```
sum(rate(function_invocation_duration_seconds_sum[5m])) / sum(rate(function_invocation_duration_seconds_count[5m])) by (function_name)
```

**查询 P95 响应时间**
```
histogram_quantile(0.95, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))
```

**查询错误率**
```
sum(rate(function_invocations_total{http_code=~"5.."}[5m])) / sum(rate(function_invocations_total[5m]))
```

**查询特定 HTTP 状态码的调用次数**
```
function_invocations_total{http_code="200"}
```

## 方法二：通过 HTTP API 查询

### 1. 即时查询（Instant Query）

查询当前时刻的指标值：

```bash
# 查询所有函数调用总数
curl 'http://localhost:9090/api/v1/query?query=function_invocations_total'

# 查询特定函数
curl 'http://localhost:9090/api/v1/query?query=function_invocations_total{function_name="your-function"}'

# URL 编码版本（如果包含特殊字符）
curl -G 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=function_invocations_total{function_name="your-function"}'
```

### 2. 范围查询（Range Query）

查询一段时间内的指标值：

```bash
# 查询过去 1 小时的数据，每 15 秒一个点
curl 'http://localhost:9090/api/v1/query_range?query=rate(function_invocations_total[5m])&start=2026-01-14T10:00:00Z&end=2026-01-14T11:00:00Z&step=15s'

# 使用相对时间
curl -G 'http://localhost:9090/api/v1/query_range' \
  --data-urlencode 'query=rate(function_invocations_total[5m])' \
  --data-urlencode 'start='$(date -d '1 hour ago' +%s) \
  --data-urlencode 'end='$(date +%s) \
  --data-urlencode 'step=15s'
```

### 3. 查询元数据

**列出所有指标**
```bash
curl 'http://localhost:9090/api/v1/label/__name__/values'
```

**查询指标的标签值**
```bash
# 查询 function_name 的所有值
curl 'http://localhost:9090/api/v1/label/function_name/values'

# 查询 http_code 的所有值
curl 'http://localhost:9090/api/v1/label/http_code/values'
```

**查询指标的所有标签**
```bash
curl 'http://localhost:9090/api/v1/labels'
```

## 方法三：直接访问 Metrics 端点

### 1. 访问 Frontend 服务的 Metrics 端点

```bash
# 获取所有指标（原始格式）
curl http://localhost:8888/metrics

# 过滤特定指标
curl http://localhost:8888/metrics | grep function_invocations_total

# 只查看 Counter 指标
curl http://localhost:8888/metrics | grep "^function_invocations_total"

# 只查看 Histogram 指标
curl http://localhost:8888/metrics | grep "function_invocation_duration_seconds"
```

### 2. 格式化输出

```bash
# 使用 jq 格式化 JSON 响应（如果 API 返回 JSON）
curl -s 'http://localhost:9090/api/v1/query?query=function_invocations_total' | jq

# 使用 grep 和 awk 处理文本
curl -s http://localhost:8888/metrics | grep function_invocations_total | awk '{print $1, $2}'
```

## 方法四：使用 PromQL 命令行工具

### 安装 promtool（Prometheus 工具）

```bash
# 下载 Prometheus
wget https://github.com/prometheus/prometheus/releases/download/v2.45.0/prometheus-2.45.0.linux-amd64.tar.gz
tar xvfz prometheus-2.45.0.linux-amd64.tar.gz
cd prometheus-2.45.0.linux-amd64
```

### 使用 promtool 查询

```bash
# 注意: promtool 主要用于配置验证，不支持直接查询
# 但可以用于测试查询语法
promtool query instant http://localhost:9090 'function_invocations_total'
```

## 方法五：使用 curl 脚本查询

### 创建查询脚本

```bash
#!/bin/bash
# query-metrics.sh

PROMETHEUS_URL="http://localhost:9090"
QUERY="$1"

if [ -z "$QUERY" ]; then
    echo "Usage: $0 '<promql-query>'"
    echo "Example: $0 'function_invocations_total'"
    exit 1
fi

# URL 编码查询
ENCODED_QUERY=$(printf '%s' "$QUERY" | jq -sRr @uri)

# 执行查询
curl -s -G "${PROMETHEUS_URL}/api/v1/query" \
  --data-urlencode "query=${QUERY}" | jq '.'
```

使用方法：
```bash
chmod +x query-metrics.sh
./query-metrics.sh 'function_invocations_total'
./query-metrics.sh 'rate(function_invocations_total[5m])'
```

## 常用查询示例

### 基础查询

```promql
# 所有指标
function_invocations_total

# 带标签过滤
function_invocations_total{function_name="my-function"}

# 多个标签
function_invocations_total{function_name="my-function", http_code="200"}

# 标签正则匹配
function_invocations_total{function_name=~"my-.*"}
function_invocations_total{http_code=~"2.."}
```

### 聚合查询

```promql
# 按函数名聚合
sum(function_invocations_total) by (function_name)

# 按 HTTP 状态码聚合
sum(function_invocations_total) by (http_code)

# 多维度聚合
sum(function_invocations_total) by (function_name, http_code)
```

### 速率查询

```promql
# 每秒请求数
rate(function_invocations_total[5m])

# 按函数名分组的速率
sum(rate(function_invocations_total[5m])) by (function_name)

# 使用 irate（瞬时速率）
irate(function_invocations_total[5m])
```

### 时间窗口查询

```promql
# 过去 5 分钟的平均速率
avg_over_time(rate(function_invocations_total[5m])[5m:])

# 过去 1 小时的最大值
max_over_time(function_invocations_total[1h])
```

### 分位数查询

```promql
# P50 (中位数)
histogram_quantile(0.50, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))

# P95
histogram_quantile(0.95, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))

# P99
histogram_quantile(0.99, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))
```

### 错误率查询

```promql
# 5xx 错误率
sum(rate(function_invocations_total{http_code=~"5.."}[5m])) / sum(rate(function_invocations_total[5m]))

# 4xx 错误率
sum(rate(function_invocations_total{http_code=~"4.."}[5m])) / sum(rate(function_invocations_total[5m]))

# 非 2xx 错误率
sum(rate(function_invocations_total{http_code!~"2.."}[5m])) / sum(rate(function_invocations_total[5m]))
```

## 查询结果格式

### JSON 格式（API 响应）

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "function_name": "my-function",
          "http_code": "200"
        },
        "value": [1705234561.234, "100"]
      }
    ]
  }
}
```

### 文本格式（Metrics 端点）

```
function_invocations_total{function_name="my-function",http_code="200"} 100
function_invocations_total{function_name="my-function",http_code="500"} 5
```

## 实用技巧

### 1. 使用 jq 处理 JSON 响应

```bash
# 提取值
curl -s 'http://localhost:9090/api/v1/query?query=function_invocations_total' | \
  jq '.data.result[].value[1]'

# 提取标签和值
curl -s 'http://localhost:9090/api/v1/query?query=function_invocations_total' | \
  jq '.data.result[] | {function: .metric.function_name, count: .value[1]}'
```

### 2. 监控查询

```bash
# 持续监控（每 5 秒查询一次）
watch -n 5 'curl -s "http://localhost:9090/api/v1/query?query=sum(function_invocations_total)" | jq ".data.result[].value[1]"'
```

### 3. 保存查询结果

```bash
# 保存为 JSON
curl -s 'http://localhost:9090/api/v1/query?query=function_invocations_total' > metrics.json

# 保存为 CSV
curl -s 'http://localhost:9090/api/v1/query?query=function_invocations_total' | \
  jq -r '.data.result[] | [.metric.function_name, .metric.http_code, .value[1]] | @csv' > metrics.csv
```

### 4. 批量查询

```bash
#!/bin/bash
# batch-query.sh

QUERIES=(
  "sum(function_invocations_total)"
  "sum(rate(function_invocations_total[5m]))"
  "histogram_quantile(0.95, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le))"
)

for query in "${QUERIES[@]}"; do
  echo "Query: $query"
  curl -s -G "http://localhost:9090/api/v1/query" \
    --data-urlencode "query=$query" | jq '.data.result'
  echo ""
done
```

## 故障排查

### 查询返回空结果

1. **检查指标是否存在**
   ```bash
   curl http://localhost:8888/metrics | grep function_invocations_total
   ```

2. **检查 Prometheus 是否抓取到指标**
   - 访问 http://localhost:9090/targets
   - 确认 `yuanrong-frontend` 目标状态为 "UP"

3. **检查时间范围**
   - 确保查询的时间范围内有数据
   - 使用 `[5m]` 等时间窗口时，需要等待足够的时间

### 查询语法错误

1. **验证 PromQL 语法**
   - 在 Prometheus Web UI 的 Graph 页面测试
   - 查看错误提示

2. **检查标签名称**
   ```bash
   # 查看所有标签值
   curl 'http://localhost:9090/api/v1/label/function_name/values'
   ```

## 参考资源

- [PromQL 查询语言文档](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Prometheus HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/)
- [PromQL 函数参考](https://prometheus.io/docs/prometheus/latest/querying/functions/)
