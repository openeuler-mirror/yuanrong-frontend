# Docker 网络配置说明

## 网络隔离问题

当使用 Docker Compose 启动 Prometheus 和 Grafana 时，它们会创建一个独立的 bridge 网络（默认名称：`<project>_monitoring`），使用 `172.18.0.0/16` 网段。

而普通 Docker 容器默认连接到 `bridge` 网络，使用 `172.17.0.0/16` 网段。

这导致：
- Prometheus/Grafana 在 `172.18.0.x` 网段
- 普通容器在 `172.17.0.x` 网段
- 它们无法直接通信

## 解决方案

### 方案 1: 让普通容器连接到 monitoring 网络（推荐）

如果 Frontend 服务也在 Docker 容器中运行，可以让它连接到同一个网络：

```bash
# 查看 monitoring 网络名称
docker network ls | grep monitoring

# 将 Frontend 容器连接到 monitoring 网络
docker network connect docker_monitoring <frontend-container-name>
```

或者在启动 Frontend 容器时指定网络：

```bash
docker run --network docker_monitoring <frontend-image>
```

### 方案 2: 使用 host.docker.internal（适用于宿主机上的服务）

如果 Frontend 服务在宿主机上运行，Prometheus 配置中已经使用了 `host.docker.internal`：

```yaml
- targets: ['host.docker.internal:8888']
```

这允许容器访问宿主机上的服务。

### 方案 3: 使用默认 bridge 网络

修改 `docker-compose.yml`，让 Prometheus 和 Grafana 使用默认的 bridge 网络：

```yaml
networks:
  monitoring:
    external: true
    name: bridge
```

**注意**: 不推荐此方案，因为会失去网络隔离的优势。

### 方案 4: 使用 host 网络模式

修改 `docker-compose.yml`，使用 host 网络：

```yaml
services:
  prometheus:
    network_mode: host
    # 移除 networks 配置
```

**注意**: 此方案会失去端口映射，直接使用主机网络。

### 方案 5: 配置网络别名

在 `docker-compose.yml` 中为服务配置别名，然后使用别名访问：

```yaml
services:
  prometheus:
    networks:
      monitoring:
        aliases:
          - prometheus
```

## 验证网络连接

### 检查容器网络

```bash
# 查看容器 IP 地址
docker inspect <container-name> | grep IPAddress

# 查看容器所属网络
docker inspect <container-name> | grep -A 10 Networks
```

### 测试网络连通性

```bash
# 在 Prometheus 容器中测试连接
docker exec prometheus ping -c 3 <target-ip>

# 测试 HTTP 连接
docker exec prometheus wget -O- http://<target-ip>:<port>/metrics
```

## 当前配置说明

当前的 `prometheus.yml` 配置使用 `host.docker.internal`，这意味着：

- ✅ 如果 Frontend 在宿主机上运行：可以直接访问
- ❌ 如果 Frontend 在 Docker 容器中（172.17.0.x）：无法访问

如果 Frontend 在 Docker 容器中，需要：
1. 将 Frontend 容器连接到 `docker_monitoring` 网络
2. 修改 `prometheus.yml` 中的 targets 为容器名或 IP

## 推荐配置

### Frontend 在宿主机上

保持当前配置，使用 `host.docker.internal:8888`

### Frontend 在 Docker 容器中

1. 将 Frontend 连接到 monitoring 网络：
   ```bash
   docker network connect docker_monitoring <frontend-container>
   ```

2. 修改 `prometheus.yml`：
   ```yaml
   - targets: ['<frontend-container-name>:8888']
   # 或使用服务发现
   ```

3. 或者使用 Docker Compose 的 service name（如果 Frontend 也在 compose 中）
