#!/bin/bash

# 启动 Prometheus 和 Grafana 监控服务的脚本
# 使用方法: ./start-monitoring.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MONITORING_DIR="$SCRIPT_DIR"
PROMETHEUS_DIR="$MONITORING_DIR/prometheus"
GRAFANA_DIR="$MONITORING_DIR/grafana"

# 创建必要的目录
mkdir -p "$PROMETHEUS_DIR/data"
mkdir -p "$GRAFANA_DIR/data"
mkdir -p "$GRAFANA_DIR/provisioning/datasources"
mkdir -p "$GRAFANA_DIR/provisioning/dashboards"

# 检查 Prometheus 是否已安装
if ! command -v prometheus &> /dev/null; then
    echo "错误: Prometheus 未安装"
    echo "请访问 https://prometheus.io/download/ 下载并安装 Prometheus"
    echo "或者使用以下命令安装:"
    echo "  wget https://github.com/prometheus/prometheus/releases/download/v2.45.0/prometheus-2.45.0.linux-amd64.tar.gz"
    echo "  tar xvfz prometheus-2.45.0.linux-amd64.tar.gz"
    echo "  sudo mv prometheus-2.45.0.linux-amd64/prometheus /usr/local/bin/"
    exit 1
fi

# 检查 Grafana 是否已安装
if ! command -v grafana-server &> /dev/null; then
    echo "错误: Grafana 未安装"
    echo "请访问 https://grafana.com/grafana/download 下载并安装 Grafana"
    echo "或者使用以下命令安装:"
    echo "  wget https://dl.grafana.com/oss/release/grafana-10.0.0.linux-amd64.tar.gz"
    echo "  tar xvfz grafana-10.0.0.linux-amd64.tar.gz"
    echo "  sudo mv grafana-10.0.0.linux-amd64/bin/grafana-server /usr/local/bin/"
    exit 1
fi

# 复制配置文件
echo "配置 Prometheus..."
cp "$MONITORING_DIR/prometheus.yml" "$PROMETHEUS_DIR/prometheus.yml"

echo "配置 Grafana..."
# 创建 Grafana 数据源配置
cat > "$GRAFANA_DIR/provisioning/datasources/prometheus.yml" <<EOF
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://localhost:9090
    isDefault: true
    editable: true
    jsonData:
      timeInterval: "10s"
      httpMethod: "POST"
EOF

# 创建 Grafana 仪表板配置
cat > "$GRAFANA_DIR/provisioning/dashboards/default.yml" <<EOF
apiVersion: 1

providers:
  - name: 'Default'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: $GRAFANA_DIR/provisioning/dashboards
EOF

# 复制仪表板 JSON
cp "$MONITORING_DIR/grafana-dashboard.json" "$GRAFANA_DIR/provisioning/dashboards/yuanrong-frontend.json"

# 启动 Prometheus
echo "启动 Prometheus..."
PROMETHEUS_PID=$(pgrep -f "prometheus.*prometheus.yml" || echo "")
if [ -n "$PROMETHEUS_PID" ]; then
    echo "Prometheus 已在运行 (PID: $PROMETHEUS_PID)"
else
    nohup prometheus \
        --config.file="$PROMETHEUS_DIR/prometheus.yml" \
        --storage.tsdb.path="$PROMETHEUS_DIR/data" \
        --web.console.libraries=/usr/share/prometheus/console_libraries \
        --web.console.templates=/usr/share/prometheus/consoles \
        --web.listen-address=:9090 \
        --web.enable-lifecycle \
        > "$PROMETHEUS_DIR/prometheus.log" 2>&1 &
    echo "Prometheus 已启动 (PID: $!)"
    echo "访问地址: http://localhost:9090"
fi

# 等待 Prometheus 启动
sleep 2

# 启动 Grafana
echo "启动 Grafana..."
GRAFANA_PID=$(pgrep -f grafana-server || echo "")
if [ -n "$GRAFANA_PID" ]; then
    echo "Grafana 已在运行 (PID: $GRAFANA_PID)"
else
    export GF_PATHS_DATA="$GRAFANA_DIR/data"
    export GF_PATHS_LOGS="$GRAFANA_DIR/logs"
    export GF_PATHS_PROVISIONING="$GRAFANA_DIR/provisioning"
    export GF_SERVER_HTTP_PORT=3000
    
    mkdir -p "$GRAFANA_DIR/logs"
    
    nohup grafana-server \
        --homepath=/usr/share/grafana \
        --config=/etc/grafana/grafana.ini \
        > "$GRAFANA_DIR/grafana.log" 2>&1 &
    echo "Grafana 已启动 (PID: $!)"
    echo "访问地址: http://localhost:3000"
    echo "默认用户名/密码: admin/admin"
fi

echo ""
echo "=========================================="
echo "监控服务已启动"
echo "=========================================="
echo "Prometheus: http://localhost:9090"
echo "Grafana:    http://localhost:3000"
echo ""
echo "停止服务: ./stop-monitoring.sh"
echo "查看日志:"
echo "  Prometheus: tail -f $PROMETHEUS_DIR/prometheus.log"
echo "  Grafana:    tail -f $GRAFANA_DIR/grafana.log"
echo "=========================================="
