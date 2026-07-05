# Traefik 核心概念

## Traefik 是什么

Traefik 是一个现代云原生 API Gateway，专为容器化环境设计。它能自动发现 Docker 容器并配置路由，无需手动维护配置文件。

## 核心概念

### 1. Entrypoints（入口点）

Entrypoint 是 Traefik 监听的网络端口，是请求进入 Traefik 的起点。

```yaml
# 在 compose.yml 中配置
command:
  - "--entrypoints.web.address=:80"    # HTTP
  - "--entrypoints.websecure.address=:443"  # HTTPS
```

我们项目只用 HTTP 入口（端口 80）。

### 2. Routers（路由器）

Router 根据规则决定请求去往哪个 Service。

```yaml
labels:
  # 路由规则：匹配 URL 路径前缀
  - "traefik.http.routers.user.rule=PathPrefix(`/api/v1/auth`)"
  # 指定使用哪个 Entrypoint
  - "traefik.http.routers.user.entrypoints=web"
```

**匹配规则语法：**
```
PathPrefix(`/api/v1/auth`)     # 路径前缀匹配
Path(`/api/v1/auth/login`)     # 精确路径匹配
Host(`example.com`)            # 域名匹配
Method(`GET`, `POST`)          # HTTP 方法匹配
Headers(`X-Custom`, `value`)   # 请求头匹配
```

多个规则可以用 `&&`（与）或 `||`（或）组合。

### 3. Services（服务）

Service 是实际处理请求的后端服务。

```yaml
labels:
  # 指定后端服务的端口
  - "traefik.http.services.user.loadbalancer.server.port=8081"
```

Traefik 会自动将匹配的请求转发到该端口。

### 4. Middlewares（中间件）

Middleware 在请求到达 Service 之前或响应返回客户端之前执行额外逻辑。

```yaml
labels:
  # 速率限制：每秒 100 个请求，突发 200
  - "traefik.http.middlewares.rate-limit.ratelimit.average=100"
  - "traefik.http.middlewares.rate-limit.ratelimit.burst=200"
  # 将中间件应用到路由
  - "traefik.http.routers.user.middlewares=rate-limit"
```

常用中间件：
- `ratelimit` — 限流
- `retry` — 重试
- `circuitbreaker` — 熔断
- `headers` — CORS 头设置
- `basicauth` — 基础认证
- `stripPrefix` — 去除路径前缀

## Docker 自动发现

Traefik 最强大的特性是 **自动发现 Docker 容器**：

```
1. Traefik 监听 Docker Socket (/var/run/docker.sock)
2. 发现带有 traefik.enable=true 标签的容器
3. 读取容器上的路由标签，自动生成配置
4. 容器启动/停止时，配置自动更新
```

```
┌─────────┐     ┌──────────────┐     ┌──────────┐
│ Docker   │ ──→ │   Traefik    │ ──→ │ Service  │
│ Socket   │     │ (读取标签)    │     │ (转发)   │
└─────────┘     └──────────────┘     └──────────┘
```

## Traefik Dashboard

Traefik 自带一个 Web Dashboard，可以查看：
- 所有已发现的服务
- 路由规则
- 中间件配置
- 健康状态

启动后访问 `http://localhost:8080/dashboard/`

## 请求流转过程

```
1. 客户端发送 GET /api/v1/auth/login
       │
2. 请求到达 Traefik :80 (Entrypoint)
       │
3. Traefik 匹配路由规则:
   PathPrefix(`/api/v1/auth`) → 命中 user-service 路由
       │
4. 转发到 user-service:8081 (Service)
       │
5. user-service 处理请求并返回响应
       │
6. Traefik 将响应返回给客户端
```
