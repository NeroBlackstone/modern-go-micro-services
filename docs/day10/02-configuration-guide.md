# Day 10 - Ory 配置详解

## Docker Compose 配置

### 基础设施服务

在 `compose.yml` 中新增以下服务：

```yaml
services:
  # Ory Kratos PostgreSQL（独立数据库）
  postgres-kratos:
    image: postgres:18-alpine
    environment:
      POSTGRES_USER: kratos
      POSTGRES_PASSWORD: kratos_secret
      POSTGRES_DB: kratos

  # Kratos 数据库迁移（一次性执行）
  kratos-migrate:
    image: oryd/kratos:v26.2.0
    command: migrate sql -e --yes
    environment:
      - DSN=postgres://kratos:kratos_secret@postgres-kratos:5432/kratos?sslmode=disable

  # Ory Kratos 身份管理服务器
  kratos:
    image: oryd/kratos:v26.2.0
    command: serve -c /etc/config/oathkeeper/kratos.yml --dev
    ports:
      - "4433:4433"  # 公共 API
      - "4434:4434"  # 管理 API

  # Ory Oathkeeper API 认证代理
  oathkeeper:
    image: oryd/oathkeeper:v26.2.0
    command: serve -c /etc/config/oathkeeper/oathkeeper.yml
    ports:
      - "4455:4455"  # API + Decision API
```

### Traefik ForwardAuth 配置

```yaml
traefik:
  labels:
    # ForwardAuth 中间件：转发到 Oathkeeper 验证认证
    - "traefik.http.middlewares.oathkeeper-auth.forwardauth.address=http://oathkeeper:4455/decisions"
    - "traefik.http.middlewares.oathkeeper-auth.forwardauth.trustForwardHeader=true"
    - "traefik.http.middlewares.oathkeeper-auth.forwardauth.authResponseHeaders=Authorization"
```

## Kratos 配置

### 主配置文件 (`configs/kratos/kratos.yml`)

```yaml
# 数据库连接
dsn: "postgres://kratos:kratos_secret@postgres-kratos:5432/kratos?sslmode=disable"

# 自服务流程
selfservice:
  default_browser_return_url: http://localhost:4455/

  flows:
    login:
      ui_url: http://localhost:4455/login
      lifespan: 720h  # 30 天会话有效期

    registration:
      ui_url: http://localhost:4455/registration
      enabled: true

    settings:
      ui_url: http://localhost:4455/settings

    verification:
      enabled: true
      ui_url: http://localhost:4455/verification

    recovery:
      enabled: true
      ui_url: http://localhost:4455/recovery

  methods:
    password:
      enabled: true

# 身份配置
identity:
  schemas:
    - id: default
      url: file:///etc/config/kratos/identity.schema.json

# 服务器配置
serve:
  public:
    port: 4433
    host: 0.0.0.0
    cors:
      enabled: true
      allowed_origins:
        - http://localhost:4455
        - http://localhost:80
      allowed_methods:
        - GET
        - POST
        - PUT
        - DELETE
        - PATCH
      allowed_headers:
        - Authorization
        - Content-Type
        - X-Session-Token
      allow_credentials: true
  admin:
    port: 4434
    host: 0.0.0.0

# 日志配置
log:
  level: info
  format: json

# 版本（必填）
version: v0.13.0
```

### 身份 Schema (`configs/kratos/identity.schema.json`)

```json
{
  "$id": "https://bookstore.example.com/identity.schema.json",
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "traits": {
      "type": "object",
      "properties": {
        "email": {
          "type": "string",
          "format": "email",
          "title": "电子邮箱"
        },
        "username": {
          "type": "string",
          "title": "用户名",
          "minLength": 2,
          "maxLength": 50
        }
      },
      "required": ["email", "username"]
    }
  }
}
```

## Oathkeeper 配置

### 主配置文件 (`configs/oathkeeper/oathkeeper.yml`)

```yaml
# 服务器配置
serve:
  proxy:
    port: 4456
  api:
    port: 4455

# 访问规则
access_rules:
  repositories:
    - file:///etc/config/oathkeeper/rules.json

# 认证器
authenticators:
  # Cookie 认证：浏览器流程使用（Kratos 设置 ory_kratos_session cookie）
  cookie_session:
    enabled: true
    config:
      check_session_url: http://kratos:4433/sessions/whoami
      preserve_path: true
      subject_from: identity.id
      extra_from: identity.traits
      only:
        - ory_kratos_session
  # Bearer Token 认证：API 客户端使用（ory_st_* session token）
  bearer_token:
    enabled: true
    config:
      check_session_url: http://kratos:4433/sessions/whoami
      preserve_path: true
      subject_from: identity.id
      extra_from: identity.traits
  noop:
    enabled: true

# 授权器
authorizers:
  allow:
    enabled: true
  keto_engine_acp_ory:
    enabled: true
    config:
      base_url: http://keto:4466
      flavor: exact

# 变换器
mutators:
  id_token:
    enabled: true
    config:
      claims: |
        {
          "iss": "https://bookstore.example.com",
          "aud": "bookstore-api",
          "email": "{{ print .Extra.email }}",
          "username": "{{ print .Extra.username }}"
        }
      jwks_url: file:///etc/config/oathkeeper/rsa_private.jwks.json
      issuer_url: http://localhost:4455
  noop:
    enabled: true
```

> **注意**：`jwks_url` 指向包含私钥的 JWKS 文件（用于签名 JWT），而非公钥文件。

### 访问规则 (`configs/oathkeeper/rules.json`)

访问规则中只需指定 handler 名称，认证器和变换器的详细配置统一在 `oathkeeper.yml` 全局定义。只有 per-rule 差异化的配置（如 Keto 授权器的 `required_action`）才在规则中覆盖。

URL 匹配使用正则 `<http://localhost(:[0-9]+)?/...>` 以兼容直连 Oathkeeper（带端口）和通过 Traefik 转发（不带端口）两种场景。

```json
[
  {
    "id": "protected-order-routes",
    "match": {
      "url": "<http://localhost(:[0-9]+)?/api/v1/orders|http://localhost(:[0-9]+)?/api/v1/orders/.*>",
      "methods": ["GET", "POST", "PUT", "DELETE"]
    },
    "authenticators": [
      { "handler": "cookie_session" },
      { "handler": "bearer_token" }
    ],
    "authorizer": {
      "handler": "keto_engine_acp_ory",
      "config": {
        "required_action": "Order:read",
        "required_resource": "Order:*",
        "subject": "{{ .Subject }}"
      }
    },
    "mutators": [{ "handler": "id_token" }]
  },
  {
    "id": "public-review-routes",
    "match": {
      "url": "<http://localhost(:[0-9]+)?/api/v1/reviews/book/.*>",
      "methods": ["GET"]
    },
    "authenticators": [{ "handler": "noop" }],
    "authorizer": { "handler": "allow" },
    "mutators": [{ "handler": "noop" }]
  }
]
```

## 认证架构说明

### Kratos 作为唯一身份提供者

在当前架构中，**Kratos 是唯一的身份认证提供者**，不再使用 Go user-service。

#### 认证流程
1. 用户通过 Kratos 的 Self-Service Flow 完成登录/注册
2. Kratos 创建 Session 并设置 Cookie
3. Oathkeeper 拦截受保护的 API 请求
4. Oathkeeper 验证 Cookie 中的 Session（调用 Kratos `/sessions/whoami`）
5. Oathkeeper 通过 `id_token` mutator 生成 JWT 传递给上游服务
6. 上游服务（如 order-service）验证 JWT 获取用户身份

#### 优势
- **单一身份源**：所有用户数据存储在 Kratos 中，避免数据不一致
- **标准化流程**：使用 Ory 生态的标准认证流程
- **减少维护成本**：无需维护自定义的 user-service

## 配置要点

### 1. 数据库隔离
Kratos 使用独立的 PostgreSQL 数据库，与业务数据库分离。

### 2. 端口规划
- Kratos 公共 API：`4433`
- Kratos 管理 API：`4434`
- Oathkeeper API：`4455`
- Oathkeeper 代理：`4456`

### 3. 安全配置
- 开发环境：Cookie 不使用 Secure 标志
- 生产环境：启用 Secure + HttpOnly + SameSite

### 4. 社交登录
需要在 Google/GitHub 创建 OAuth2 应用，获取 Client ID 和 Secret。
