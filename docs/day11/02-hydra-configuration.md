# Day 11 - Ory Hydra 配置详解

## 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        OAuth2 流程架构                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐    │
│  │  外部应用    │     │  Ory Hydra   │     │  Ory Kratos  │    │
│  │  (Client)    │────>│  (OAuth2)    │────>│  (Identity)  │    │
│  │  :8082       │     │  :4444/:4445 │     │  :4433/:4434 │    │
│  └──────────────┘     └──────────────┘     └──────────────┘    │
│         │                    │                    │             │
│         │                    │                    │             │
│         ▼                    ▼                    ▼             │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐    │
│  │ Login/Consent│     │  PostgreSQL  │     │  PostgreSQL  │    │
│  │ Server       │     │  (Hydra)     │     │  (Kratos)    │    │
│  │ :3001        │     │  :5436       │     │  :5435       │    │
│  └──────────────┘     └──────────────┘     └──────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Docker Compose 配置

### 新增服务

```yaml
services:
  # Hydra PostgreSQL（独立数据库）
  postgres-hydra:
    image: postgres:18-alpine
    environment:
      POSTGRES_USER: hydra
      POSTGRES_PASSWORD: hydra_secret
      POSTGRES_DB: hydra
    ports:
      - "5436:5432"

  # Hydra 数据库迁移
  hydra-migrate:
    image: oryd/hydra:v26.2.0
    command: migrate sql -e --yes
    environment:
      - DSN=postgres://hydra:hydra_secret@postgres-hydra:5432/hydra?sslmode=disable

  # Ory Hydra - OAuth2 Provider
  hydra:
    image: oryd/hydra:v26.2.0
    command: serve -c /etc/config/hydra/hydra.yml --dev
    ports:
      - "4444:4444"  # 公共 API
      - "4445:4445"  # 管理 API

  # Hydra Login/Consent 服务
  hydra-login-consent:
    build:
      context: .
      dockerfile: Dockerfile
      target: hydra-login-consent
    ports:
      - "3001:3001"

  # OAuth2 客户端演示
  oauth-client-demo:
    build:
      context: .
      dockerfile: Dockerfile
      target: oauth-client-demo
    ports:
      - "8082:8082"
```

## Hydra 配置文件

### 主配置 (`configs/hydra/hydra.yml`)

```yaml
# 数据库连接
dsn: "postgres://hydra:hydra_secret@postgres-hydra:5432/hydra?sslmode=disable"

# 服务配置
serve:
  public:
    port: 4444  # OAuth2 端点
  admin:
    port: 4445  # 管理 API

# URL 配置（v26.x 格式）
urls:
  self:
    issuer: http://localhost:4444  # issuer URL
  login: http://hydra-login-consent:3001/login
  consent: http://hydra-login-consent:3001/consent
  logout: http://hydra-login-consent:3001/logout
  post_logout_redirect: http://localhost:8082/

# OAuth2 配置
oauth2:
  # Access Token 策略
  access_token_strategy: jwt

# 安全配置
security:
  force_https: false

# 系统密钥（生产环境必须更改）
secrets:
  system:
    - this-is-a-very-long-and-secure-secret-key-for-development-only
```

### 端口规划

| 端口 | 服务 | 用途 |
|------|------|------|
| 4444 | Hydra Public | OAuth2 端点（授权、Token、OIDC） |
| 4445 | Hydra Admin | 管理 API（创建 Client 等） |
| 3001 | Login/Consent | 处理登录和授权同意 |
| 8082 | OAuth Client | 演示客户端应用 |

## Hydra 关键端点

### 公共端点（:4444）

```
/oauth2/auth          # 授权端点（发起 OAuth2 流程）
/oauth2/token         # Token 端点（用 code 换 token）
/oauth2/revoke        # 撤销 Token
/oauth2/introspect    # Token 内省（验证 token 有效性）
/oauth2/flush         # 刷新 Token

/.well-known/openid-configuration  # OIDC Discovery
/userinfo             # 用户信息端点
/jwks.json            # JSON Web Key Set
```

### 管理端点（:4445）

```
/admin/clients                      # 创建 OAuth2 Client
/admin/clients/{id}                 # 获取/更新/删除 Client
/admin/oauth2/auth/requests/login   # 获取登录请求
/admin/oauth2/auth/requests/consent # 获取 consent 请求
/admin/oauth2/auth/requests/logout  # 获取登出请求
```

## Login/Consent 服务

### 工作原理

Hydra 不直接处理用户登录，而是通过**回调**机制：

1. 用户访问 `/oauth2/auth` → Hydra 重定向到 Login Provider
2. Login Provider 验证用户身份（通过 Kratos Session）
3. 验证通过后，调用 Hydra Admin API 接受登录
4. Hydra 重定向到 Consent Provider
5. Consent Provider 获取用户同意
6. Hydra 签发 authorization_code

### 关键代码

```go
// 处理登录挑战
func handleLogin(w http.ResponseWriter, r *http.Request) {
    challenge := r.URL.Query().Get("login_challenge")

    // 1. 获取 Hydra 登录请求
    loginRequest, _ := getLoginRequest(challenge)

    // 2. 检查 Kratos Session
    session, err := checkKratosSession(r)
    if err != nil {
        // 没有 session，重定向到登录页
        redirectToLogin(w, r, challenge)
        return
    }

    // 3. 接受登录
    acceptLogin(w, r, challenge, session.Identity.ID)
}

// 接受登录请求
func acceptLogin(w http.ResponseWriter, r *http.Request, challenge, subject string) {
    body := map[string]any{
        "subject":  subject,
        "remember": true,
    }

    // 调用 Hydra Admin API
    http.Post(
        fmt.Sprintf("%s/admin/oauth2/auth/requests/login/accept?login_challenge=%s", hydraAdminURL, challenge),
        "application/json",
        strings.NewReader(string(data)),
    )
}
```

## 创建 OAuth2 Client

### 使用管理 API

```bash
# 创建 Client
curl -X POST http://localhost:4445/admin/clients \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "bookstore-web-client",
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "scope": "openid offline email profile",
    "token_endpoint_auth_method": "client_secret_basic",
    "redirect_uris": ["http://localhost:8082/callback"]
  }'
```

### 使用脚本

```bash
./scripts/create-hydra-client.sh
```

## 安全配置

### 开发环境

```yaml
security:
  force_https: false
  cookie:
    secure: false
    http_only: true
```

### 生产环境

```yaml
security:
  force_https: true
  cookie:
    secure: true
    http_only: true
    same_site: Strict
```

## 与 Kratos 集成

### 登录流程

```
1. Hydra 重定向到 Login Provider
2. Login Provider 检查 Kratos Session Cookie
3. 如果无 Session，重定向到 Kratos 登录页
4. 用户在 Kratos 登录
5. Kratos 设置 Session Cookie
6. 回调到 Login Provider
7. Login Provider 验证 Session
8. 调用 Hydra Admin API 接受登录
```

### Session 验证

```go
func checkKratosSession(r *http.Request) (*KratosSession, error) {
    cookie, _ := r.Cookie("ory_kratos_session")

    req, _ := http.NewRequest("GET", "http://kratos:4433/sessions/whoami", nil)
    req.Header.Set("Cookie", fmt.Sprintf("ory_kratos_session=%s", cookie.Value))

    resp, _ := client.Do(req)
    // 解析 session...
}
```

## 配置要点

1. **数据库隔离**：Hydra 使用独立的 PostgreSQL
2. **端口规划**：公共 API 和管理 API 分离
3. **Session 共享**：通过 Kratos Session Cookie 实现单点登录
4. **Token 策略**：开发环境使用 JWT，生产环境建议 Opaque + Introspection
5. **CORS 配置**：允许客户端域名跨域访问
