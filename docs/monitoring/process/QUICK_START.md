# 快速开始指南

## 一、安装 Prometheus 和 Grafana

### 使用包管理器安装（推荐）

**Ubuntu/Debian:**
```bash
# 安装 Prometheus
sudo apt-get update
sudo apt-get install -y prometheus

# 安装 Grafana
sudo apt-get install -y software-properties-common
sudo add-apt-repository "deb https://packages.grafana.com/oss/deb stable main"
wget -q -O - https://packages.grafana.com/gpg.key | sudo apt-key add -
sudo apt-get update
sudo apt-get install -y grafana
```

**CentOS/RHEL:**
```bash
# 安装 Prometheus
sudo yum install -y prometheus

# 安装 Grafana
sudo yum install -y grafana
```

### 手动安装

参考 `README.md` 中的安装说明。

## 二、配置和启动

### 1. 修改 Prometheus 配置

编辑 `prometheus.yml`，将 `targets` 中的端口改为你的 Frontend 服务实际端口：

```yaml
- targets: ['localhost:8888']  # 改为实际端口
```

### 2. 启动服务

**使用脚本（推荐）:**
```bash
cd docs/monitoring
./start-monitoring.sh
```

**手动启动:**
```bash
# 启动 Prometheus
prometheus --config.file=prometheus.yml --storage.tsdb.path=./data --web.listen-address=:9090 &

# 启动 Grafana
grafana-server --config=/etc/grafana/grafana.ini --homepath=/usr/share/grafana &
```

## 三、配置 Grafana

### 1. 登录 Grafana

访问 http://localhost:3000
- 用户名: `admin`
- 密码: `admin`（首次登录会要求修改）

### 2. 添加 Prometheus 数据源

1. 点击左侧菜单 "Configuration" -> "Data Sources"
2. 点击 "Add data source"
3. 选择 "Prometheus"
4. URL 填写: `http://localhost:9090`
5. 点击 "Save & Test"

### 3. 创建仪表板

#### 方法一：导入 JSON（推荐）

1. 点击左侧菜单 "Dashboards" -> "Import"
2. 点击 "Upload JSON file"
3. 选择 `grafana-dashboard.json` 文件
4. 选择 Prometheus 数据源
5. 点击 "Import"

#### 方法二：手动创建面板

**面板 1: 函数调用速率**

1. 点击 "Create" -> "Dashboard" -> "Add visualization"
2. 查询语句:
   ```
   sum(rate(function_invocations_total[5m])) by (function_name)
   ```
3. 图例: `{{function_name}}`
4. 标题: "Function Invocation Rate"

**面板 2: 响应时间 P95**

1. 添加新面板
2. 查询语句:
   ```
   histogram_quantile(0.95, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))
   ```
3. 图例: `p95 - {{function_name}}`
4. 标题: "Response Time P95"

**面板 3: HTTP 状态码分布**

1. 添加新面板，选择 "Pie chart" 类型
2. 查询语句:
   ```
   sum(function_invocations_total) by (http_code)
   ```
3. 标题: "HTTP Status Code Distribution"

**面板 4: 错误率**

1. 添加新面板
2. 查询语句:
   ```
   sum(rate(function_invocations_total{http_code=~"5.."}[5m])) / sum(rate(function_invocations_total[5m]))
   ```
3. 标题: "Error Rate"
4. Y 轴单位: Percent (0.0-1.0)

## 四、验证

### 1. 验证指标端点

访问 http://localhost:8888/metrics（替换为实际端口），应该能看到：
- `function_invocations_total`
- `function_invocation_duration_seconds_bucket`
- `function_invocation_duration_seconds_sum`
- `function_invocation_duration_seconds_count`

### 2. 验证 Prometheus 抓取

访问 http://localhost:9090/targets，确认 `yuanrong-frontend` 目标状态为 "UP"

### 3. 验证 Grafana 查询

在 Grafana 的 Explore 页面，输入以下查询验证：
```
function_invocations_total
```

## 五、常用查询示例

### 查看所有函数调用总数
```
sum(function_invocations_total) by (function_name)
```

### 查看特定函数的调用次数
```
function_invocations_total{function_name="your-function-name"}
```

### 查看平均响应时间
```
sum(rate(function_invocation_duration_seconds_sum[5m])) / sum(rate(function_invocation_duration_seconds_count[5m])) by (function_name)
```

### 查看 P99 响应时间
```
histogram_quantile(0.99, sum(rate(function_invocation_duration_seconds_bucket[5m])) by (le, function_name))
```

### 查看 5xx 错误数量
```
sum(function_invocations_total{http_code=~"5.."})
```

## 六、停止服务

```bash
./stop-monitoring.sh
```

或手动停止:
```bash
pkill prometheus
pkill grafana-server
```
