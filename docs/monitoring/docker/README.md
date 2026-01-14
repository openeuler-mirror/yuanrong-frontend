# Docker 部署 Prometheus 和 Grafana 监控

本文档介绍如何使用 Docker 和 Docker Compose 部署 Prometheus 和 Grafana 来监控 Yuanrong Frontend 服务。

## 前置要求

- Docker 20.10 或更高版本
- Docker Compose 2.0 或更高版本（或 Docker 内置的 `docker compose` 命令）

### 安装 Docker

**Ubuntu/Debian:**
```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
```

**CentOS/RHEL:**
```bash
sudo yum install -y docker
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER
```

安装完成后，需要重新登录或执行 `newgrp docker` 使权限生效。

## 快速开始

### 1. 配置 Prometheus

编辑 `prometheus.yml`，根据你的 Frontend 服务位置修改 targets：

**如果 Frontend 服务在宿主机上运行:**
```yaml
- targets: ['host.docker.internal:8888']  # 使用 host.docker.internal 访问宿主机
```

**如果 Frontend 服务在 Docker 网络中:**
```yaml
- targets: ['frontend-service:8888']  # 使用服务名或容器名
```

**如果 Frontend 服务在其他机器上:**
```yaml
- targets: ['192.168.1.100:8888']  # 使用实际 IP 地址
```

### 2. 修复权限（首次运行或遇到权限问题时）

Prometheus 容器以 `nobody` 用户（UID 65534）运行，需要确保数据目录有正确的权限：

```bash
cd docs/monitoring/docker
./fix-permissions.sh
```

或者手动修复：
```bash
sudo chown -R 65534:65534 prometheus-data
sudo chmod -R 755 prometheus-data
```

### 3. 启动服务

```bash
cd docs/monitoring/docker
chmod +x *.sh
./start.sh
```

**注意**: `start.sh` 脚本会自动尝试修复权限，但如果遇到权限问题，请手动运行 `fix-permissions.sh`。

### 4. 访问服务

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000
  - 用户名: `admin`
  - 密码: `admin`（首次登录会要求修改）

### 4. 停止服务

```bash
./stop.sh
```

## 目录结构

```
docker/
├── docker-compose.yml          # Docker Compose 配置文件
├── prometheus.yml              # Prometheus 配置
├── start.sh                    # 启动脚本
├── stop.sh                     # 停止脚本
├── README.md                   # 本文档
├── prometheus-data/            # Prometheus 数据目录（自动创建）
├── grafana-data/               # Grafana 数据目录（自动创建）
├── grafana-provisioning/       # Grafana 自动配置
│   ├── datasources/           # 数据源配置
│   │   └── prometheus.yml
│   └── dashboards/            # 仪表板配置
│       └── default.yml
└── grafana-dashboards/        # Grafana 仪表板 JSON 文件
    └── yuanrong-frontend.json
```

## 配置说明

### Docker Compose 配置

`docker-compose.yml` 包含两个服务：

1. **Prometheus**
   - 端口: 9090
   - 数据卷: `./prometheus-data`
   - 配置文件: `./prometheus.yml`

2. **Grafana**
   - 端口: 3000
   - 数据卷: `./grafana-data`
   - 自动配置数据源和仪表板

### 网络配置

- 使用 Docker bridge 网络 `monitoring`
- Prometheus 和 Grafana 在同一网络中，可以互相访问
- Grafana 通过服务名 `prometheus:9090` 访问 Prometheus

### 数据持久化

- Prometheus 数据保存在 `./prometheus-data`
- Grafana 数据保存在 `./grafana-data`
- 删除容器不会删除数据（除非使用 `docker-compose down -v`）

## 常用命令

### 查看服务状态

```bash
docker-compose ps
# 或
docker compose ps
```

### 查看日志

```bash
# 查看所有服务日志
docker-compose logs -f

# 查看 Prometheus 日志
docker logs -f prometheus

# 查看 Grafana 日志
docker logs -f grafana
```

### 重启服务

```bash
docker-compose restart
# 或
docker compose restart
```

### 更新配置

修改配置文件后，需要重启服务：

```bash
# 重启 Prometheus（重新加载配置）
docker-compose restart prometheus

# 或使用 Prometheus 的 reload API
curl -X POST http://localhost:9090/-/reload
```

### 清理数据

```bash
# 停止并删除容器和数据卷
docker-compose down -v
```

## 故障排查

### Prometheus 权限错误

如果看到类似 `permission denied` 的错误：

1. **修复数据目录权限**
   ```bash
   ./fix-permissions.sh
   ```

2. **或者重新创建数据目录**
   ```bash
   docker-compose down
   sudo rm -rf prometheus-data
   mkdir -p prometheus-data
   sudo chown -R 65534:65534 prometheus-data
   docker-compose up -d
   ```

3. **检查容器日志**
   ```bash
   docker logs prometheus
   ```

### Prometheus 无法抓取指标

1. **检查网络连接**
   ```bash
   # 在 Prometheus 容器中测试连接
   docker exec prometheus wget -O- http://host.docker.internal:8888/metrics
   ```

2. **检查配置文件**
   ```bash
   # 验证 Prometheus 配置
   docker exec prometheus promtool check config /etc/prometheus/prometheus.yml
   ```

3. **查看 Prometheus 目标状态**
   - 访问 http://localhost:9090/targets
   - 检查 `yuanrong-frontend` 目标状态

### Grafana 无法连接 Prometheus

1. **检查数据源配置**
   - 登录 Grafana
   - 进入 Configuration -> Data Sources
   - 检查 Prometheus URL 是否为 `http://prometheus:9090`

2. **测试连接**
   - 在数据源配置页面点击 "Save & Test"
   - 查看错误信息

### 容器无法启动

1. **检查端口占用**
   ```bash
   # 检查端口是否被占用
   netstat -tuln | grep -E '9090|3000'
   ```

2. **查看容器日志**
   ```bash
   docker-compose logs
   ```

3. **检查文件权限**
   ```bash
   # 确保目录有正确的权限
   chmod -R 755 .
   ```

## 生产环境建议

1. **使用环境变量文件**
   - 创建 `.env` 文件存储敏感信息
   - 在 `docker-compose.yml` 中使用 `${VARIABLE}` 引用

2. **配置资源限制**
   ```yaml
   services:
     prometheus:
       deploy:
         resources:
           limits:
             cpus: '2'
             memory: 4G
   ```

3. **使用 Docker Secrets**
   - 存储 Grafana 管理员密码
   - 存储其他敏感配置

4. **配置日志驱动**
   ```yaml
   services:
     prometheus:
       logging:
         driver: "json-file"
         options:
           max-size: "10m"
           max-file: "3"
   ```

5. **使用健康检查**
   - 已在 `docker-compose.yml` 中配置
   - 可以添加自动重启策略

6. **定期备份**
   ```bash
   # 备份 Prometheus 数据
   tar -czf prometheus-backup-$(date +%Y%m%d).tar.gz prometheus-data/
   
   # 备份 Grafana 数据
   tar -czf grafana-backup-$(date +%Y%m%d).tar.gz grafana-data/
   ```

## 与进程部署的区别

| 特性 | Docker 部署 | 进程部署 |
|------|------------|---------|
| 安装 | 需要 Docker | 需要手动安装二进制文件 |
| 配置 | docker-compose.yml | 多个配置文件 |
| 启动 | `docker-compose up` | 需要手动启动多个进程 |
| 隔离 | 容器隔离 | 系统进程 |
| 数据持久化 | Docker 卷 | 本地目录 |
| 网络 | Docker 网络 | 本地网络 |

## 参考资源

- [Docker 官方文档](https://docs.docker.com/)
- [Docker Compose 文档](https://docs.docker.com/compose/)
- [Prometheus Docker 镜像](https://hub.docker.com/r/prom/prometheus)
- [Grafana Docker 镜像](https://hub.docker.com/r/grafana/grafana)
