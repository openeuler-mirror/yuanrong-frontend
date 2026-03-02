#!/bin/bash

# 启动完整监控栈（裸进程模式）
# 服务: Prometheus / Grafana / Loki / Tempo / OTel Collector
# 使用方法: ./start-monitoring.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export MONITORING_DIR="$SCRIPT_DIR"

PROMETHEUS_DIR="$MONITORING_DIR/prometheus"
GRAFANA_DIR="$MONITORING_DIR/grafana"
LOKI_DIR="$MONITORING_DIR/loki"
TEMPO_DIR="$MONITORING_DIR/tempo"
OTEL_DIR="$MONITORING_DIR/otelcol"
BIN_DIR="$MONITORING_DIR/bin"

# ── 版本（需要更新时只改这里）────────────────────────────────────────────
PROMETHEUS_VERSION="2.51.2"
LOKI_VERSION="3.3.2"
TEMPO_VERSION="2.7.1"
OTELCOL_VERSION="0.120.0"
GRAFANA_VERSION="11.4.0"
GRAFANA_HOME="$BIN_DIR/grafana-v${GRAFANA_VERSION}"

# ── 创建数据/工具目录 ────────────────────────────────────────────────────
mkdir -p "$BIN_DIR"
mkdir -p "$PROMETHEUS_DIR/data" "$PROMETHEUS_DIR/logs"
mkdir -p "$GRAFANA_DIR/data" "$GRAFANA_DIR/logs"
mkdir -p "$GRAFANA_DIR/provisioning/datasources"
mkdir -p "$GRAFANA_DIR/provisioning/dashboards"
mkdir -p "$LOKI_DIR/data/chunks" "$LOKI_DIR/data/rules"
mkdir -p "$LOKI_DIR/data/storage" "$LOKI_DIR/data/compactor" "$LOKI_DIR/logs"
mkdir -p "$TEMPO_DIR/data/traces" "$TEMPO_DIR/data/wal"
mkdir -p "$TEMPO_DIR/data/generator/wal" "$TEMPO_DIR/logs"
mkdir -p "$OTEL_DIR/logs"

export PATH="$BIN_DIR:$PATH"

# ── 自动下载单文件二进制 ─────────────────────────────────────────────────
# 用法: _ensure_bin <目标名> <下载URL> [包内二进制名]
_ensure_bin() {
    local name="$1"
    local url="$2"
    local arc_name="${3:-$name}"

    command -v "$name" &>/dev/null && return 0

    echo "  下载 $name ..."
    local tmp
    tmp=$(mktemp -d)
    trap "rm -rf '$tmp'" RETURN

    if ! curl -fsSL --retry 3 -o "$tmp/arc" "$url"; then
        echo "错误: 下载 $name 失败，URL: $url"
        exit 1
    fi

    case "$url" in
        *.tar.gz|*.tgz) tar -xzf "$tmp/arc" -C "$tmp" ;;
        *.zip)          unzip -q  "$tmp/arc" -d "$tmp" ;;
    esac

    local bin
    bin=$(find "$tmp" -name "$arc_name" -type f | head -1)
    if [[ -z "$bin" ]]; then
        echo "错误: 在下载包内找不到 $arc_name"
        exit 1
    fi

    cp "$bin" "$BIN_DIR/$name"
    chmod +x "$BIN_DIR/$name"
    echo "  $name 已安装: $BIN_DIR/$name"
}

# ── 自动下载 Grafana（需保留完整目录以提供 public/ conf/ 等资源）─────────
_ensure_grafana() {
    command -v grafana        &>/dev/null && return 0
    command -v grafana-server &>/dev/null && return 0
    [[ -x "$GRAFANA_HOME/bin/grafana" ]]  && return 0

    echo "  下载 Grafana ${GRAFANA_VERSION} ..."
    local url="https://dl.grafana.com/oss/release/grafana-${GRAFANA_VERSION}.linux-amd64.tar.gz"
    local tmp
    tmp=$(mktemp -d)
    trap "rm -rf '$tmp'" RETURN

    if ! curl -fsSL --retry 3 -o "$tmp/grafana.tar.gz" "$url"; then
        echo "错误: 下载 Grafana 失败，URL: $url"
        exit 1
    fi
    tar -xzf "$tmp/grafana.tar.gz" -C "$BIN_DIR"
    echo "  Grafana 已安装: $GRAFANA_HOME"
}

# ── 检查/下载依赖 ─────────────────────────────────────────────────────────
echo "检查依赖..."
_ensure_bin prometheus \
    "https://github.com/prometheus/prometheus/releases/download/v${PROMETHEUS_VERSION}/prometheus-${PROMETHEUS_VERSION}.linux-amd64.tar.gz"

_ensure_bin loki \
    "https://github.com/grafana/loki/releases/download/v${LOKI_VERSION}/loki-linux-amd64.zip" \
    "loki-linux-amd64"

_ensure_bin tempo \
    "https://github.com/grafana/tempo/releases/download/v${TEMPO_VERSION}/tempo_${TEMPO_VERSION}_linux_amd64.tar.gz"

_ensure_bin otelcol-contrib \
    "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTELCOL_VERSION}/otelcol-contrib_${OTELCOL_VERSION}_linux_amd64.tar.gz"

_ensure_grafana
echo ""

# ── 配置 Grafana 数据源（Prometheus + Loki + Tempo）────────────────────
cat > "$GRAFANA_DIR/provisioning/datasources/datasources.yml" <<EOF
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

  - name: Loki
    type: loki
    access: proxy
    url: http://localhost:3100
    editable: true
    jsonData:
      derivedFields:
        - name: TraceID
          matcherRegex: 'trace_id=(\w+)'
          url: "\${__value.raw}"
          datasourceUid: tempo

  - name: Tempo
    type: tempo
    access: proxy
    url: http://localhost:3200
    editable: true
    uid: tempo
    jsonData:
      httpMethod: GET
      serviceMap:
        datasourceUid: prometheus
EOF

# ── 配置 Grafana 仪表板 provider ─────────────────────────────────────────
cat > "$GRAFANA_DIR/provisioning/dashboards/default.yml" <<EOF
apiVersion: 1

providers:
  - name: Default
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: $GRAFANA_DIR/provisioning/dashboards
EOF

cp "$MONITORING_DIR/grafana-dashboard.json" \
   "$GRAFANA_DIR/provisioning/dashboards/yuanrong-monitoring.json"

# ── 启动 Loki ─────────────────────────────────────────────────────────────
echo "启动 Loki..."
if pgrep -f "loki.*loki-config.yaml" &>/dev/null; then
    echo "  Loki 已在运行"
else
    nohup loki \
        -config.file="$MONITORING_DIR/loki-config.yaml" \
        -config.expand-env=true \
        > "$LOKI_DIR/logs/loki.log" 2>&1 &
    echo "  Loki 已启动 (PID: $!)"
fi

# ── 启动 Tempo ────────────────────────────────────────────────────────────
echo "启动 Tempo..."
if pgrep -f "tempo.*tempo-config.yaml" &>/dev/null; then
    echo "  Tempo 已在运行"
else
    nohup tempo \
        -config.file="$MONITORING_DIR/tempo-config.yaml" \
        > "$TEMPO_DIR/logs/tempo.log" 2>&1 &
    echo "  Tempo 已启动 (PID: $!)"
fi

# ── 启动 OTel Collector ───────────────────────────────────────────────────
echo "启动 OTel Collector..."
if pgrep -f "otelcol.*otel-collector-config.yaml" &>/dev/null; then
    echo "  OTel Collector 已在运行"
else
    nohup otelcol-contrib \
        --config="$MONITORING_DIR/otel-collector-config.yaml" \
        > "$OTEL_DIR/logs/otelcol.log" 2>&1 &
    echo "  OTel Collector 已启动 (PID: $!)"
fi

# ── 启动 Prometheus ───────────────────────────────────────────────────────
echo "启动 Prometheus..."
if pgrep -f "prometheus.*prometheus.yml" &>/dev/null; then
    echo "  Prometheus 已在运行"
else
    nohup prometheus \
        --config.file="$MONITORING_DIR/prometheus.yml" \
        --storage.tsdb.path="$PROMETHEUS_DIR/data" \
        --web.listen-address=:9090 \
        --web.enable-lifecycle \
        --web.enable-remote-write-receiver \
        > "$PROMETHEUS_DIR/logs/prometheus.log" 2>&1 &
    echo "  Prometheus 已启动 (PID: $!)"
fi

sleep 2

# ── 启动 Grafana ──────────────────────────────────────────────────────────
echo "启动 Grafana..."
if pgrep -f "grafana.* server\|grafana-server" &>/dev/null; then
    echo "  Grafana 已在运行"
else
    export GF_PATHS_DATA="$GRAFANA_DIR/data"
    export GF_PATHS_LOGS="$GRAFANA_DIR/logs"
    export GF_PATHS_PROVISIONING="$GRAFANA_DIR/provisioning"
    export GF_SERVER_HTTP_PORT=3000

    if command -v grafana-server &>/dev/null; then
        # 系统包安装：使用系统默认 homepath 和 config
        nohup grafana-server \
            --homepath=/usr/share/grafana \
            --config=/etc/grafana/grafana.ini \
            > "$GRAFANA_DIR/logs/grafana.log" 2>&1 &
    else
        # 本地下载：使用解压目录作为 homepath
        local_bin="$GRAFANA_HOME/bin/grafana"
        [[ ! -x "$local_bin" ]] && local_bin=$(command -v grafana)
        nohup "$local_bin" server \
            --homepath="$GRAFANA_HOME" \
            cfg:paths.data="$GRAFANA_DIR/data" \
            cfg:paths.logs="$GRAFANA_DIR/logs" \
            cfg:paths.provisioning="$GRAFANA_DIR/provisioning" \
            cfg:server.http_port=3000 \
            > "$GRAFANA_DIR/logs/grafana.log" 2>&1 &
    fi
    echo "  Grafana 已启动 (PID: $!)"
fi

echo ""
echo "=========================================="
echo "监控栈已启动"
echo "=========================================="
echo "Prometheus:     http://localhost:9090"
echo "Grafana:        http://localhost:3000  (admin/admin)"
echo "Loki:           http://localhost:3100"
echo "Tempo:          http://localhost:3200"
echo "OTel (OTLP):    grpc://localhost:4317  http://localhost:4318"
echo "OTel self-mon:  http://localhost:8888"
echo ""
echo "日志目录:"
echo "  Loki:         $LOKI_DIR/logs/loki.log"
echo "  Tempo:        $TEMPO_DIR/logs/tempo.log"
echo "  OTel:         $OTEL_DIR/logs/otelcol.log"
echo "  Prometheus:   $PROMETHEUS_DIR/logs/prometheus.log"
echo "  Grafana:      $GRAFANA_DIR/logs/grafana.log"
echo ""
echo "停止服务: ./stop-monitoring.sh"
echo "=========================================="
