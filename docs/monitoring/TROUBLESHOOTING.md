# Grafana 指标显示问题排查指南

## 问题：直接访问 Metrics 端点能采集到指标，但 Grafana 上没有显示

### 排查步骤

#### 1. 检查 Prometheus 是否抓取到指标

**方法一：通过 Prometheus Web UI**

1. 访问 http://localhost:9090
2. 点击 "Graph" 菜单
3. 输入查询：`function_invocations_total`
4. 点击 "Execute"
5. 查看是否有结果

**方法二：通过 API**

```bash
curl 'http://localhost:9090/api/v1/query?query=function_invocations_total'
```

**方法三：检查 Targets 状态**

1. 访问 http://localhost:9090/targets
2. 查看 `yuanrong-frontend` 目标状态
3. 确认状态为 "UP"（绿色）
4. 如果有错误，查看 "Error" 列

#### 2. 检查 Grafana 数据源连接

**方法一：通过 Grafana UI**

1. 登录 Grafana (http://localhost:3000)
2. 进入 Configuration -> Data Sources
3. 点击 "Prometheus" 数据源
4. 点击 "Save & Test" 按钮
5. 查看是否显示 "Data source is working"

**方法二：通过 API**

```bash
# 测试数据源连接（需要先获取 API Key 或使用基本认证）
curl -u admin:admin 'http://localhost:3000/api/datasources/proxy/1/api/v1/query?query=up'
```

#### 3. 检查查询语句

在 Grafana 中测试查询：

1. 进入 Grafana
2. 点击左侧菜单 "Explore"（或 "+" -> "Explore"）
3. 选择 "Prometheus" 数据源
4. 输入查询：`function_invocations_total`
5. 点击 "Run query"
6. 查看是否有数据

#### 4. 检查时间范围

1. 在 Grafana 右上角检查时间范围
2. 确保时间范围包含有数据的时间段
3. 尝试选择 "Last 5 minutes" 或 "Last 1 hour"

#### 5. 检查指标名称和标签

**验证指标是否存在：**

```bash
# 在 Prometheus 中查询所有指标名称
curl 'http://localhost:9090/api/v1/label/__name__/values' | grep function

# 查询特定指标的所有标签值
curl 'http://localhost:9090/api/v1/label/function_name/values'
```

**在 Grafana Explore 中测试：**

1. 进入 Explore
2. 输入：`{__name__=~"function.*"}`
3. 查看所有 function 相关的指标

#### 6. 检查仪表板配置

**验证仪表板查询：**

1. 打开仪表板
2. 点击面板标题 -> "Edit"
3. 查看查询语句是否正确
4. 检查数据源是否选择正确
5. 查看是否有错误提示

#### 7. 检查 Prometheus 配置

**验证抓取配置：**

```bash
# 查看 Prometheus 配置
docker exec prometheus cat /etc/prometheus/prometheus.yml

# 验证配置语法
docker exec prometheus promtool check config /etc/prometheus/prometheus.yml
```

**检查抓取日志：**

```bash
# 查看 Prometheus 日志
docker logs prometheus | grep -i error
docker logs prometheus | grep -i "yuanrong-frontend"
```

#### 8. 检查网络连接

**从 Prometheus 容器测试连接：**

```bash
# 测试是否能访问 Frontend metrics 端点
docker exec prometheus wget -O- http://host.docker.internal:8888/metrics

# 或使用容器 IP（如果已连接网络）
docker exec prometheus wget -O- http://172.17.0.2:8888/metrics
```

## 常见问题和解决方案

### 问题 1: Prometheus 显示 "no data"

**原因：**
- Prometheus 没有抓取到指标
- 时间范围内没有数据
- 查询语句错误

**解决：**
1. 检查 Targets 状态
2. 确认指标端点可访问
3. 等待一段时间让 Prometheus 收集数据
4. 检查查询语句语法

### 问题 2: Grafana 显示 "No data"

**原因：**
- 数据源未连接
- 查询语句错误
- 时间范围不对
- Prometheus 中没有数据

**解决：**
1. 测试数据源连接
2. 在 Explore 中测试查询
3. 检查时间范围
4. 验证 Prometheus 中有数据

### 问题 3: 数据源测试失败

**原因：**
- Prometheus 服务未运行
- 网络连接问题
- URL 配置错误

**解决：**
```bash
# 检查 Prometheus 是否运行
docker ps | grep prometheus

# 测试连接
curl http://localhost:9090/-/healthy

# 检查 Grafana 配置中的 URL
# 在 Docker 中应该使用: http://prometheus:9090
# 在宿主机访问应该使用: http://localhost:9090
```

### 问题 4: 查询返回空结果但指标存在

**原因：**
- 标签不匹配
- 时间范围问题
- 指标名称错误

**解决：**
```bash
# 查看所有指标
curl 'http://localhost:9090/api/v1/label/__name__/values'

# 查看指标的标签值
curl 'http://localhost:9090/api/v1/series?match[]=function_invocations_total'
```

## 快速诊断脚本

创建诊断脚本帮助排查：

```bash
#!/bin/bash
# diagnose-grafana.sh

echo "=== 诊断 Grafana 指标显示问题 ==="
echo ""

# 1. 检查 Prometheus 是否运行
echo "1. 检查 Prometheus 状态..."
if docker ps | grep -q prometheus; then
    echo "   ✓ Prometheus 容器运行中"
else
    echo "   ✗ Prometheus 容器未运行"
    exit 1
fi

# 2. 检查 Prometheus 健康状态
echo ""
echo "2. 检查 Prometheus 健康状态..."
if curl -s http://localhost:9090/-/healthy > /dev/null; then
    echo "   ✓ Prometheus 健康检查通过"
else
    echo "   ✗ Prometheus 健康检查失败"
fi

# 3. 检查 Targets
echo ""
echo "3. 检查 Prometheus Targets..."
TARGETS=$(curl -s 'http://localhost:9090/api/v1/targets' | jq -r '.data.activeTargets[] | "\(.labels.job): \(.health)"' 2>/dev/null)
if [ -n "$TARGETS" ]; then
    echo "   Targets 状态:"
    echo "$TARGETS" | sed 's/^/   /'
else
    echo "   ⚠ 无法获取 Targets 信息"
fi

# 4. 检查指标是否存在
echo ""
echo "4. 检查指标是否存在..."
METRICS=$(curl -s 'http://localhost:9090/api/v1/query?query=function_invocations_total' | jq -r '.data.result | length' 2>/dev/null)
if [ "$METRICS" != "0" ] && [ -n "$METRICS" ]; then
    echo "   ✓ 找到 $METRICS 个 function_invocations_total 时间序列"
else
    echo "   ✗ 未找到 function_invocations_total 指标"
fi

# 5. 检查 Grafana 是否运行
echo ""
echo "5. 检查 Grafana 状态..."
if docker ps | grep -q grafana; then
    echo "   ✓ Grafana 容器运行中"
else
    echo "   ✗ Grafana 容器未运行"
fi

# 6. 检查 Grafana 健康状态
echo ""
echo "6. 检查 Grafana 健康状态..."
if curl -s http://localhost:3000/api/health > /dev/null; then
    echo "   ✓ Grafana 健康检查通过"
else
    echo "   ✗ Grafana 健康检查失败"
fi

# 7. 测试数据源连接
echo ""
echo "7. 测试 Grafana 数据源连接..."
DS_TEST=$(curl -s -u admin:admin 'http://localhost:3000/api/datasources/proxy/1/api/v1/query?query=up' 2>/dev/null | jq -r '.status' 2>/dev/null)
if [ "$DS_TEST" = "success" ]; then
    echo "   ✓ Grafana 可以连接到 Prometheus"
else
    echo "   ✗ Grafana 无法连接到 Prometheus"
fi

echo ""
echo "=== 诊断完成 ==="
```

## 验证步骤清单

- [ ] Prometheus 容器运行正常
- [ ] Prometheus Web UI 可访问 (http://localhost:9090)
- [ ] Targets 状态为 "UP"
- [ ] 在 Prometheus 中能查询到指标
- [ ] Grafana 容器运行正常
- [ ] Grafana Web UI 可访问 (http://localhost:3000)
- [ ] 数据源连接测试通过
- [ ] 在 Grafana Explore 中能查询到数据
- [ ] 仪表板时间范围正确
- [ ] 仪表板查询语句正确
