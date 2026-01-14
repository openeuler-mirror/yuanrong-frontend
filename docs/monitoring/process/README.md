# Prometheus 和 Grafana 监控部署指南

本文档介绍如何使用进程方式部署 Prometheus 和 Grafana，并配置它们来监控 Yuanrong Frontend 服务的指标。

## 前置要求

### 1. 安装 Prometheus

```bash
# 下载 Prometheus
wget https://github.com/prometheus/prometheus/releases/download/v2.45.0/prometheus-2.45.0.linux-amd64.tar.gz

# 解压
tar xvfz prometheus-2.45.0.linux-amd64.tar.gz

# 安装到系统路径（可选）
sudo mv prometheus-2.45.0.linux-amd64/prometheus /usr/local/bin/
sudo mv prometheus-2.45.0.linux-amd64/promtool /usr/local/bin/

# 或者添加到 PATH
export PATH=$PATH:$(pwd)/prometheus-2.45.0.linux-amd64
```

### 2. 安装 Grafana

```bash
# 下载 Grafana
wget https://dl.grafana.com/oss/release/grafana-10.0.0.linux-amd64.tar.gz

# 解压
tar xvfz grafana-10.0.0.linux-amd64.tar.gz

# 安装到系统路径（可选）
sudo mv grafana-10.0.0.linux-amd64/bin/grafana-server /usr/local/bin/
sudo mv grafana-10.0.0.linux-amd64/bin/grafana-cli /usr/local/bin/

# 或者添加到 PATH
export PATH=$PATH:$(pwd)/grafana-10.0.0.linux-amd64/bin
```

## 快速开始

### 方法一：使用启动脚本（推荐）

```bash
# 进入监控目录
cd docs/monitoring

# 赋予执行权限
chmod +x start-monitoring.sh stop-monitoring.sh

# 启动监控服务
./start-monitoring.sh

# 停止监控服务
./stop-monitoring.sh
```

### 方法二：手动启动

#### 1. 配置 Prometheus

编辑 `prometheus.yml`，确保 `targets` 中的端口与你的 Frontend 服务端口一致：

```yaml
scrape_configs:
  - job_name: 'yuanrong-frontend'
    static_configs:
      - targets: ['localhost:8888']  # 修改为实际的服务端口
```

#### 2. 启动 Prometheus

```bash
prometheus \
  --config.file=prometheus.yml \
  --storage.tsdb.path=./prometheus/data \
  --web.listen-address=:9090 \
  --web.enable-lifecycle
```

访问 http://localhost:9090 验证 Prometheus 是否正常运行。

#### 3. 启动 Grafana

```bash
export GF_PATHS_DATA=./grafana/data
export GF_PATHS_LOGS=./grafana/logs
export GF_PATHS_PROVISIONING=./grafana/provisioning
export GF_SERVER_HTTP_PORT=3000

grafana-server \
  --homepath=/usr/share/grafana \
  --config=/etc/grafana/grafana.ini
```

访问 http://localhost:3000，使用默认用户名/密码 `admin/admin` 登录。

## 配置说明

### Prometheus 配置

`prometheus.yml` 文件包含以下主要配置：

- **scrape_interval**: 指标抓取间隔（默认 15 秒）
- **scrape_configs**: 定义要抓取的目标服务
  - `job_name`: 任务名称
  - `targets`: 服务地址和端口
  - `metrics_path`: 指标路径（默认为 `/metrics`）

### Grafana 配置

#### 1. 添加数据源

1. 登录 Grafana
2. 进入 Configuration -> Data Sources
3. 点击 "Add data source"
4. 选择 "Prometheus"
5. 配置 URL: `http://localhost:9090`
6. 点击 "Save & Test"

或者使用自动配置（如果使用启动脚本，会自动配置）：

配置文件位于 `grafana/provisioning/datasources/prometheus.yml`

#### 2. 导入仪表板

**方法一：使用 JSON 文件导入**

1. 进入 Dashboards -> Import
2. 上传 `grafana-dashboard.json` 文件
3. 选择 Prometheus 数据源
4. 点击 "Import"

**方法二：手动创建面板**

在 Grafana 中创建以下面板：

1. **Function Invocations Total**
   - 查询: `sum(rate(function_invocations_total[5m])) by (function_name, http_code)`
   - 类型: Graph

2. **Function Invocation Duration**
   - 查询: `histogram_quantile(0.95, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))`
   - 类型: Graph

3. **HTTP Status Code Distribution**
   - 查询: `sum(function_invocations_total) by (http_code)`
   - 类型: Pie Chart

4. **Error Rate**
   - 查询: `sum(rate(function_invocations_total{http_code=~"5.."}[5m])) / sum(rate(function_invocations_total[5m]))`
   - 类型: Graph

## 可用的指标

### Counter 指标

- `function_invocations_total`: 函数调用总数
  - 标签: `function_name`, `http_code`

### Histogram 指标

- `function_invocation_duration_seconds`: 函数调用耗时（秒）
  - 标签: `function_name`
  - 自动生成: `_bucket`, `_sum`, `_count`

## 常用 Prometheus 查询

### 查询函数调用速率

```promql
sum(rate(function_invocations_total[5m])) by (function_name)
```

### 查询平均响应时间

```promql
sum(rate(function_invocation_duration_seconds_sum[5m])) / sum(rate(function_invocation_duration_seconds_count[5m])) by (function_name)
```

### 查询 P95 响应时间

```promql
histogram_quantile(0.95, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))
```

### 查询错误率

```promql
sum(rate(function_invocations_total{http_code=~"5.."}[5m])) / sum(rate(function_invocations_total[5m]))
```

### 查询特定函数的调用次数

```promql
function_invocations_total{function_name="your-function-name"}
```

## 故障排查

### Prometheus 无法抓取指标

1. 检查 Frontend 服务是否运行
2. 验证端口是否正确（默认 8888）
3. 访问 `http://localhost:8888/metrics` 确认指标端点可访问
4. 检查 Prometheus 日志: `tail -f prometheus/prometheus.log`

### Grafana 无法连接 Prometheus

1. 确认 Prometheus 正在运行（访问 http://localhost:9090）
2. 检查数据源配置中的 URL 是否正确
3. 检查防火墙设置

### 指标不显示

1. 确认指标已注册（检查 `/metrics` 端点）
2. 确认 Prometheus 正在抓取（在 Prometheus UI 的 Status -> Targets 中查看）
3. 等待一段时间让 Prometheus 收集数据

## 生产环境建议

1. **使用 systemd 服务**: 将 Prometheus 和 Grafana 配置为 systemd 服务，确保自动启动
2. **配置持久化存储**: 使用持久化存储保存 Prometheus 数据
3. **设置告警规则**: 在 Prometheus 中配置告警规则，使用 Alertmanager 发送通知
4. **配置反向代理**: 使用 Nginx 等反向代理保护 Grafana
5. **定期备份**: 定期备份 Grafana 仪表板和 Prometheus 数据

## 参考资源

- [Prometheus 官方文档](https://prometheus.io/docs/)
- [Grafana 官方文档](https://grafana.com/docs/)
- [PromQL 查询语言](https://prometheus.io/docs/prometheus/latest/querying/basics/)
