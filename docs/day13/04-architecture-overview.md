# Day 13 - 服务间认证：架构总览

## 完整认证架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Day 13 后的完整认证架构                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────┐                                                               │
│  │  Client   │──── Kratos Session Cookie                                    │
│  │ (Browser) │                                                              │
│  └────┬─────┘                                                               │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Traefik (:80)                               │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │  ForwardAuth → Oathkeeper Decision API                       │   │    │
│  │  │  ├─ cookie_session: 验证 Kratos Session                      │   │    │
│  │  │  ├─ keto_engine_acp_ory: 检查权限 (Order:read)               │   │    │
│  │  │  └─ id_token mutator: 生成 JWT (HMAC 签名)                   │   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  │                              │                                      │    │
│  │                              │ 返回: Authorization: Bearer <jwt>   │    │
│  └──────────────────────────────┼──────────────────────────────────────┘    │
│                                 │                                           │
│                                 ▼                                           │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    order-service (:8080)                             │   │
│  │                                                                      │   │
│  │  HTTP Handler                                                        │   │
│  │  ├─ JWTAuth middleware: 验证 Oathkeeper JWT                         │   │
│  │  │   └─ 提取 user_id, email, username                               │   │
│  │  │   └─ 注入 caller_user, caller_email 到 context                   │   │
│  │  │                                                                   │   │
│  │  └─ 调用 orderSvc.Create(ctx, ...)                                  │   │
│  │                                                                      │   │
│  │  gRPC Client 拦截器链:                                               │   │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌──────┐ │   │
│  │  │ Tracing │──▶│ Service │──▶│Circuit  │──▶│  Retry  │──▶│ Rate │ │   │
│  │  │         │   │  Auth   │   │ Breaker │   │         │   │Limiter│ │   │
│  │  └─────────┘   └────┬────┘   └─────────┘   └─────────┘   └──────┘ │   │
│  │                      │                                               │   │
│  │                      │ 从 context 提取用户信息                       │   │
│  │                      │ 签发 service JWT (HMAC-SHA256)                │   │
│  │                      │ 放入 gRPC metadata: authorization            │   │
│  └──────────────────────┼───────────────────────────────────────────────┘   │
│                         │                                                   │
│                         │ gRPC + metadata: { authorization: "Bearer xxx" } │
│                         ▼                                                   │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    book-service (:9092)                              │   │
│  │                                                                      │   │
│  │  gRPC Server 拦截器链:                                               │   │
│  │  ┌─────────┐   ┌─────────┐                                          │   │
│  │  │ Tracing │──▶│ Service │──▶ Handler                                │   │
│  │  │         │   │  Auth   │                                           │   │
│  │  └─────────┘   └────┬────┘                                           │   │
│  │                      │                                               │   │
│  │                      │ 从 metadata 提取 JWT                         │   │
│  │                      │ 验证签名 + aud 匹配                          │   │
│  │                      │ 提取 caller_user, caller_email               │   │
│  │                      │ 注入到 context                                │   │
│  │                                                                      │   │
│  │  Handler 处理请求，可使用 ctx.Value(caller_user) 获取调用方信息      │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 两层认证对比

| 层级 | 认证机制 | 签发方 | 验证方 | 用途 |
|------|----------|--------|--------|------|
| **外部请求** | Oathkeeper JWT (HMAC) | Oathkeeper | 后端服务 | 验证用户身份 |
| **服务间调用** | Service JWT (HMAC) | order-service | book-service | 验证服务身份 + 用户传播 |

```
外部认证（Day 10-12）:
  Client → Traefik → Oathkeeper → JWT → 后端服务验证

服务间认证（Day 13）:
  order-service → Service JWT → book-service验证
                └─ 包含原始用户信息
```

## Token 流转示例

以 "用户 alice 创建订单" 为例：

```
1. alice 浏览器发送请求
   Cookie: ory_kratos_session=xxx

2. Traefik → Oathkeeper 验证
   返回: Authorization: Bearer <oathkeeper-jwt>
   JWT Claims: { sub: "alice-id", email: "alice@example.com" }

3. order-service JWTAuth 验证
   提取: user_id=1, email="alice@example.com"
   注入 context: caller_user="alice-id"

4. order-service → book-service
   Service JWT Claims: {
     iss: "order-service",
     aud: "book-service",
     sub: "order-service",
     caller_user: "alice-id",
     caller_email: "alice@example.com"
   }

5. book-service 验证
   验证通过: caller_user="alice-id"
   可以记录: "alice 通过 order-service 查询了图书"
```

## 安全边界

```
┌─────────────────────────────────────────────────────────────────┐
│                        安全边界                                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  外部网络                                                       │
│  ─────────────────────────────────────────────────────────────  │
│                                                                 │
│  Docker 网络                                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                                                         │   │
│  │  Traefik ←→ Oathkeeper ←→ Kratos                       │   │
│  │      │                                                 │   │
│  │      │ ForwardAuth (HTTP)                              │   │
│  │      ▼                                                 │   │
│  │  order-service ──── gRPC + Service JWT ────▶ book-svc  │   │
│  │                                                         │   │
│  │  ⚠️  Service JWT 仅在网络内有效                         │   │
│  │  ⚠️  共享密钥泄露 = 服务间信任链断裂                     │   │
│  │                                                         │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 组件职责总结

| 组件 | 认证职责 | 端口 |
|------|----------|------|
| **Traefik** | 转发认证请求到 Oathkeeper | 80/8080 |
| **Kratos** | 管理用户身份、Session | 4433/4434 |
| **Oathkeeper** | 验证 Session + 签发 JWT | 4455/4456 |
| **Keto** | 权限检查（Order:read 等） | 4466/4467 |
| **order-service** | 验证 Oathkeeper JWT + 签发 Service JWT | 8080 |
| **book-service** | 验证 Service JWT + 提取调用方信息 | 9092 |

## Day 13 学到了什么

1. **服务间认证的必要性**：没有认证的服务间通信等于"裸奔"
2. **JWT 传播模式**：通过 gRPC metadata 传递 token
3. **gRPC 拦截器链**：认证作为拦截器链的一环，与 tracing/circuit-breaker 协同工作
4. **Context 传播**：用户信息通过 Go context 在调用链中传递
5. **Shared Secret 模式**：简单有效的服务间认证方案

## 未来改进方向

| 改进项 | 说明 |
|--------|------|
| **mTLS** | 添加传输层加密，防止网络嗅探 |
| **Hydra Client Credentials** | 使用 OAuth2 标准流程替代共享密钥 |
| **Token 自动续期** | 客户端缓存 token，过期前自动刷新 |
| **密钥轮换** | 支持密钥热更新，无需重启服务 |
| **审计日志** | 记录所有服务间调用的详细日志 |

## 端口规划（更新）

| 端口 | 服务 | 用途 |
|------|------|------|
| 80 | Traefik | HTTP 入口 |
| 3000 | Grafana | 可视化 |
| 3001 | hydra-login-consent | OAuth2 胶水 |
| 4433 | Kratos | 公共 API |
| 4434 | Kratos | 管理 API |
| 4444 | Hydra | OAuth2 端点 |
| 4445 | Hydra | 管理 API |
| 4455 | Oathkeeper | API + Decision API |
| 4456 | Oathkeeper | Proxy |
| 4466 | Keto | Read API |
| 4467 | Keto | Write API |
| 4468 | Keto | Metrics API |
| 5433-5437 | PostgreSQL | 各服务数据库 |
| 8082 | oauth-client-demo | OAuth2 演示 |
| 8083 | webhook | 注册 webhook |
| 8084 | admin-service | 管理 API |
| 9090 | Prometheus | 指标监控 |
| 9092 | book-service | gRPC |
| 9093 | book-service | Prometheus metrics |
