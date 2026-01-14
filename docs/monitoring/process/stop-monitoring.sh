#!/bin/bash

# 停止 Prometheus 和 Grafana 监控服务的脚本

echo "停止 Prometheus..."
pkill -f "prometheus.*prometheus.yml" || echo "Prometheus 未运行"

echo "停止 Grafana..."
pkill -f grafana-server || echo "Grafana 未运行"

echo "监控服务已停止"
