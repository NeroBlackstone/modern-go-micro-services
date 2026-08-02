# Day 12: 全局架构总览

## 组件拓扑

```
                                    ┌─────────────────────────────────────────────┐
                                    │            请求流转全景图                    │
                                    └─────────────────────────────────────────────┘

┌──────────┐      ┌─────────────────────────────────────────────────────────────────────────────┐
│  Client   │─────▶                            Traefik (:80)                                   │
│ (Browser) │      │  ┌──────────────────────────────────────────────────────────────────────┐  │
│           │      │  │                     请求处理流程                                      │  │
│           │      │  │                                                                      │  │
│           │      │  │  1. 收到请求                                                          │  │
│           │      │  │       │                                                               │  │
│           │      │  │       ▼                                                               │  │
│           │      │  │  2. ForwardAuth 中间件 ──────────────────────┐                        │  │
│           │      │  │       │                                     │                        │  │
│           │      │  │       │ 3. 调用 Oathkeeper /decisions       │                        │  │
│           │      │  │       │    (认证+授权)                      │                        │  │
│           │      │  │       │                                     ▼                        │  │
│           │      │  │       │                              ┌──────────────┐                │  │
│           │      │  │       │                              │  Oathkeeper  │                │  │
│           │      │  │       │                              │  (4455)      │                │  │
│           │      │  │       │                              │              │                │  │
│           │      │  │       │                              │ Authenticator│                │  │
│           │      │  │       │                              │     ↓        │                │  │
│           │      │  │       │                              │ Authorizer   │                │  │
│           │      │  │       │                              │ (keto_engine │                │  │
│           │      │  │       │                              │  _acp_ory)   │                │  │
│           │      │  │       │                              │     ↓        │                │  │
│           │      │  │       │                              │ Mutator      │                │  │
│           │      │  │       │                              │ (生成 JWT)   │                │  │
│           │      │  │       │                              └──────┬───────┘                │  │
│           │      │  │       │                                     │                        │  │
│           │      │  │       │◀──────────── 返回 200 + Headers ────┘                        │  │
│           │      │  │       │                                                              │  │
│           │      │  │       ▼                                                              │  │
│           │      │  │  4. 路径路由                                                          │  │
│           │      │  │       │                                                               │  │
│           │      │  │       ├─ /api/v1/orders/* ────▶ order-service (:8080)                 │  │
│           │      │  │       ├─ /api/v1/admin/* ─────▶ admin-service (:8084)                 │  │
│           │      │  │       └─ /api/v1/reviews/* ──▶ order-service (:8080)                  │  │
│           │      │  └──────────────────────────────────────────────────────────────────────┘  │
│           │      └─────────────────────────────────────────────────────────────────────────────┘
└──────────┘
```

### 简化流转

```
┌────────┐    1. HTTP Request     ┌──────────┐    2. ForwardAuth     ┌────────────┐
│ Client │───────────────────────▶│ Traefik  │──────────────────────▶│ Oathkeeper │
│        │                        │   (:80)  │                       │   (4455)   │
└────────┘                        └────┬─────┘                       └─────┬──────┘
                                       │                                  │
                                       │    3. 返回 200 + Headers         │
                                       │◀─────────────────────────────────┘
                                       │
                                       │ 4. 路由转发 + 注入 Headers
                                       │    X-User-ID: user-123
                                       │    X-User-Email: alice@example.com
                                       ▼
                                ┌─────────────────┐
                                │ Backend Service │
                                │  (8080)         │
                                └─────────────────┘
```

### Oathkeeper 内部管道

```
                    Oathkeeper 内部管道
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  Incoming Request                                           │
│       │                                                     │
│       ▼                                                     │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │Authenticator│───▶│ Authorizer  │───▶│  Mutator    │     │
│  │             │    │             │    │             │     │
│  │ • cookie_   │    │ • allow     │    │ • id_token  │     │
│  │   session   │    │ • deny      │    │ • header    │     │
│  │ • noop      │    │ • keto_     │    │ • noop      │     │
│  │             │    │   engine_   │    │             │     │
│  │             │    │   acp_ory   │    │             │     │
│  └─────────────┘    └─────────────┘    └─────────────┘     │
│       │                  │                  │               │
│       ▼                  ▼                  ▼               │
│  验证身份          检查权限          注入用户信息           │
│  (Kratos)         (Keto)            到 Headers             │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## 服务职责

| 组件 | 职责 | 端口 |
|------|------|------|
| **Traefik** | 反向代理/负载均衡 + ForwardAuth 中间件 | 80/8080 |
| **Kratos** | 身份管理（注册/登录/Session） | 4433/4434 |
| **Hydra** | OAuth2/OIDC Provider（Token 签发与验证） | 4444/4445 |
| **Keto** | 权限控制（Zanzibar 关系模型） | 4466/4467/4468 |
| **Oathkeeper** | API 认证代理（Authenticator → Authorizer → Mutator） | 4455/4456 |
| **hydra-login-consent** | 胶水：连接 Hydra ↔ Kratos（Login/Consent/Logout） | 3001 |
| **webhook** | 胶水：注册后自动将用户添加到 `Group:users` | 8083 |
| **book-service** | 图书业务（gRPC :9092 + Metrics :9093） | - |
| **order-service** | 订单/书评业务（HTTP :8080，通过 Traefik 暴露） | - |
| **admin-service** | 管理 API（HTTP :8084，通过 Traefik 暴露，需 `Order:manage` 权限） | - |

## 核心流程

### 1. API 请求认证（Session/Cookie）

```
Client ──▶ Traefik (80) ──ForwardAuth──▶ Oathkeeper ──▶ Kratos (验证Session)
              │                            │
              │◀───────────────────────────│ 返回 identity.id + traits
              │
              │ 路由转发 + 注入 Headers
              ▼
         Backend Service (X-User-ID, X-User-Email, Authorization)
```

### 2. 授权检查（Keto）

```
Oathkeeper cookie_session 验证身份 → 得到 subject (用户 ID)
  → keto_engine_acp_ory 调用 Keto Check API
  → Keto 自动解析 subject sets 和传递关系
  → 通过: 转发 / 拒绝: 403

用户 API:     Order:read  → order-service  (/api/v1/orders/*)
管理 API:     Order:manage → admin-service  (/api/v1/admin/orders/*)
```

### 3. OAuth2 授权码流程

```
1. Client ──▶ Hydra (/oauth2/auth)
2. Hydra ──▶ hydra-login-consent (/login?login_challenge=xxx)
3. hydra-login-consent 检查 Kratos Session
   ├─ 有 Session → 接受登录 → 返回 Hydra
   └─ 无 Session → 重定向 Kratos 登录 → 登录后返回
4. hydra-login-consent ──▶ Hydra (接受 login)
5. Hydra ──▶ hydra-login-consent (/consent?consent_challenge=xxx)
6. hydra-login-consent ──▶ Hydra (自动批准 consent，开发环境)
7. Hydra 返回 authorization_code 给 Client
```

### 4. 注册后自动授权

```
用户注册 → Kratos webhook → webhook 服务 → Keto Write API (PUT)
                                              ↓
                                    用户添加到 Group:users
                                              ↓
                                    自动获得基础查看权限
```

## 配置文件索引

| 文件 | 作用 |
|------|------|
| `compose.yml` | Docker Compose 服务编排 |
| `Dockerfile` | 多阶段构建（6 个 target） |
| `configs/kratos/kratos.yml` | Kratos 身份管理配置 |
| `configs/kratos/identity.schema.json` | 用户 traits 定义 |
| `configs/hydra/hydra.yml` | Hydra OAuth2 配置 |
| `configs/keto/keto.yml` | Keto 服务配置 |
| `configs/keto/namespaces.keto.ts` | OPL 命名空间定义 |
| `configs/keto/relation-tuples/*.json` | 权限数据（JSON） |
| `configs/oathkeeper/oathkeeper.yml` | Oathkeeper 代理配置 |
| `configs/oathkeeper/rules.json` | API 访问规则 |

## 端口规划

| 端口 | 服务 | 用途 |
|------|------|------|
| 80 | Traefik | HTTP 入口 |
| 8080 | Traefik | Dashboard |
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

## FAQ

**Q: 为什么需要 hydra-login-consent？**
Hydra 不直接处理用户界面，需要外部 Provider 处理 Login Challenge、Consent Challenge。这个 Go 服务连接 Hydra 和 Kratos。

**Q: 为什么不需要单独的 keto-authorizer 服务？**
Oathkeeper v26 内置了 `keto_engine_acp_ory` authorizer，直接调用 Keto Check API，无需额外中间层。

**Q: Kratos 和 Hydra 的区别？**
- Kratos: 处理用户凭证，管理 Session（谁登录了）
- Hydra: 处理 OAuth2 流程，颁发 Access Token（授权第三方访问）

**Q: 如何表示"所有用户都有权限"？**
使用 Group 命名空间：
```json
{"namespace":"Group","object":"users","relation":"members","subject_id":"alice"}
{"namespace":"Order","object":"*","relation":"viewers",
 "subject_set":{"namespace":"Group","object":"users","relation":"members"}}
```
