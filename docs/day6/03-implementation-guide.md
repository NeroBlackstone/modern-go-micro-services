# Traefik 实现指南

## 架构概览

```
客户端 ──→ Traefik (:80) ──→ user-service (:8081)
                          ──→ order-service (:8080)
                          ──→ (book-service 仅 gRPC，不通过 Traefik)
```

## compose.yml 配置详解

### Traefik 服务配置

```yaml
traefik:
  image: traefik:v3.3
  container_name: bookstore-traefik
  command:
    - "--providers.docker=true"              # 启用 Docker 提供者
    - "--providers.docker.exposedbydefault=false"  # 不自动暴露所有容器
    - "--entrypoints.web.address=:80"        # 监听 80 端口
    - "--api.dashboard=true"                 # 启用 Dashboard
    - "--log.level=INFO"                     # 日志级别
  ports:
    - "80:80"       # HTTP 入口
    - "8080:8080"   # Dashboard
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro  # 只读挂载 Docker Socket
```

**关键配置说明：**

| 配置 | 作用 |
|------|------|
| `providers.docker=true` | 告诉 Traefik 从 Docker 发现服务 |
| `exposedbydefault=false` | 只处理带 `traefik.enable=true` 的容器 |
| `entrypoints.web.address=:80` | 定义 HTTP 入口点 |
| `docker.sock:ro` | 只读访问 Docker，Traefik 不能创建/删除容器 |

### 服务标签配置

**user-service 标签：**
```yaml
labels:
  - "traefik.enable=true"                    # 启用 Traefik 发现
  - "traefik.http.routers.user.rule=PathPrefix(`/api/v1/auth`) || PathPrefix(`/api/v1/user`)"
  - "traefik.http.routers.user.entrypoints=web"
  - "traefik.http.services.user.loadbalancer.server.port=8081"
```

**order-service 标签：**
```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.order.rule=PathPrefix(`/api/v1/orders`) || PathPrefix(`/api/v1/reviews`)"
  - "traefik.http.routers.order.entrypoints=web"
  - "traefik.http.services.order.loadbalancer.server.port=8080"
```

**标签命名规则：**
```
traefik.http.routers.<router-name>.<property>
traefik.http.services.<service-name>.<property>
```

## 端口变更说明

| 服务 | 之前 | 之后 | 原因 |
|------|------|------|------|
| Traefik | 无 | :80 (HTTP), :8080 (Dashboard) | 新增入口 |
| user-service | :9091, :8081 | :9091 | HTTP 通过 Traefik 暴露 |
| order-service | :8080 | 无 host 映射 | HTTP 通过 Traefik 暴露 |
| book-service | :9092 | :9092 | 不变，gRPC 内部通信 |

**注意：** 服务在容器内部仍然监听原来的端口（8080/8081），只是不再映射到 host。Traefik 通过 Docker 内部网络访问这些端口。

## 路由映射表

```
请求路径                              → 目标服务
─────────────────────────────────────────────────
/api/v1/auth/register               → user-service:8081
/api/v1/auth/login                  → user-service:8081
/api/v1/user/profile                → user-service:8081
/api/v1/orders                      → order-service:8080
/api/v1/orders/:id                  → order-service:8080
/api/v1/orders/:id/status           → order-service:8080
/api/v1/reviews/book/:book_id       → order-service:8080
/api/v1/reviews                     → order-service:8080
```

## 启动和验证

### 1. 启动所有服务

```bash
docker compose up -d
```

### 2. 检查 Traefik 是否发现服务

```bash
# 查看 Traefik 日志
docker logs bookstore-traefik

# 访问 Dashboard
curl http://localhost:8080/api/http/routers
```

### 3. 测试路由

```bash
# 测试 user-service 路由（应转发到 user-service）
curl http://localhost/api/v1/auth/login

# 测试 order-service 路由（应转发到 order-service）
curl http://localhost/api/v1/orders

# 测试 Traefik Dashboard
open http://localhost:8080/dashboard/
```

### 4. 验证 gRPC 不受影响

```bash
# gRPC 端口仍然可以直接访问
grpcurl -plaintext localhost:9091 list
grpcurl -plaintext localhost:9092 list
```

## 常见问题

### Q: 端口冲突怎么办？

如果 Traefik 的 8080 端口和 order-service 冲突，order-service 已经移除了 host 端口映射，不会冲突。

### Q: 如何添加新服务的路由？

只需在新服务的 compose 配置中添加 Traefik 标签即可，无需重启 Traefik：

```yaml
new-service:
  labels:
    - "traefik.enable=true"
    - "traefik.http.routers.new.rule=PathPrefix(`/api/v1/new`)"
    - "traefik.http.routers.new.entrypoints=web"
    - "traefik.http.services.new.loadbalancer.server.port=9000"
```

### Q: 如何添加限流中间件？

```yaml
labels:
  - "traefik.http.middlewares.user-rateLimit.ratelimit.average=100"
  - "traefik.http.middlewares.user-rateLimit.ratelimit.burst=50"
  - "traefik.http.routers.user.middlewares=user-rateLimit"
```

## 后续扩展方向

1. **HTTPS** — 配置 TLS 证书，添加 443 入口点
2. **限流** — 为公开 API 添加速率限制
3. **CORS** — 配置跨域资源共享
4. **请求重写** — 去除路径前缀后转发
5. **健康检查** — 配置后端服务健康检查
