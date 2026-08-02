# Day 11 - OAuth2 完整流程演练

## 前置条件

1. 启动所有服务：
   ```bash
   docker compose up -d
   ```

2. 等待服务就绪：
   ```bash
   docker compose ps
   ```

3. 创建 OAuth2 Client：
   ```bash
   ./scripts/create-hydra-client.sh
   ```

## 流程一：完整 Authorization Code 流程

### 步骤 1：访问客户端应用

打开浏览器访问 `http://localhost:8082`

```
┌─────────────────────────────────────────┐
│  OAuth2 Client Demo                     │
│                                         │
│  当前状态：❌ 未登录                     │
│                                         │
│  [使用 OAuth2 登录]                     │
│                                         │
└─────────────────────────────────────────┘
```

### 步骤 2：点击登录按钮

点击「使用 OAuth2 登录」，浏览器重定向到 Hydra：

```
URL 变化：
http://localhost:8082/
  → http://localhost:4444/oauth2/auth?
      response_type=code
      &client_id=bookstore-web-client
      &redirect_uri=http://localhost:8082/callback
      &scope=openid+offline+email+profile
      &state=xxx
```

### 步骤 3：Hydra 检查登录状态

Hydra 发现用户未登录，重定向到 Login Provider：

```
http://localhost:3001/login?login_challenge=xxx
```

### 步骤 4：Login Provider 检查 Kratos Session

Login Provider 检查是否有有效的 Kratos Session：

```go
// 检查 Kratos Session
cookie, err := r.Cookie("ory_kratos_session")
if err != nil {
    // 没有 Session，重定向到 Kratos 登录页
    redirectToLogin(w, r, challenge)
    return
}
```

### 步骤 5：Kratos 登录页面

浏览器重定向到 Kratos 登录页：

```
http://localhost:4433/self-service/login/browser?return_to=...
```

用户输入邮箱和密码登录。

### 步骤 6：登录成功

Kratos 验证通过，设置 Session Cookie，重定向回 Login Provider：

```
http://localhost:3001/login?login_challenge=xxx
```

### 步骤 7：Login Provider 接受登录

Login Provider 调用 Hydra Admin API 接受登录：

```go
// 接受登录请求
body := map[string]any{
    "subject":  session.Identity.ID,
    "remember": true,
}

http.Post(
    "http://hydra:4445/admin/oauth2/auth/requests/login/accept?login_challenge=xxx",
    "application/json",
    strings.NewReader(string(data)),
)
```

### 步骤 8：Hydra 请求 Consent

Hydra 重定向到 Consent Provider：

```
http://localhost:3001/consent?consent_challenge=xxx
```

### 步骤 9：Consent Provider 自动同意

开发环境下，Consent Provider 自动批准：

```go
// 自动同意（开发环境）
acceptConsent(w, r, challenge, subject, requestedScope)
```

### 步骤 10：返回 Authorization Code

Hydra 重定向回客户端，携带 authorization_code：

```
http://localhost:8082/callback?
    code=xxx
    &state=xxx
```

### 步骤 11：用 Code 换取 Token

客户端用 code 换取 access_token：

```go
// Token 交换请求
data := url.Values{}
data.Set("grant_type", "authorization_code")
data.Set("code", code)
data.Set("redirect_uri", redirectURI)

// 使用 Basic Auth 认证
auth := base64.StdEncoding.EncodeToString(clientID + ":" + clientSecret)
req.Header.Set("Authorization", "Basic "+auth)

// POST /oauth2/token
resp, _ := http.Post("http://localhost:4444/oauth2/token", ...)
```

### 步骤 12：获取 Token 响应

```json
{
    "access_token": "eyJhbGciOiJSUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 3600,
    "refresh_token": "ory_rt_xxx...",
    "scope": "openid offline email profile",
    "id_token": "eyJhbGciOiJSUzI1NiIs..."
}
```

### 步骤 13：显示用户信息

客户端解析 ID Token，显示用户信息：

```
┌─────────────────────────────────────────┐
│  OAuth2 Client Demo                     │
│                                         │
│  当前状态：✅ 已登录                     │
│                                         │
│  Access Token: eyJhbGciOiJSUzI1NiIs... │
│  Token 过期时间: 2026-07-19 15:00:00    │
│                                         │
│  [查看用户信息] [测试 API 调用] [登出]   │
│                                         │
└─────────────────────────────────────────┘
```

## 流程二：访问受保护 API

### 使用 Access Token

点击「测试 API 调用」，客户端用 access_token 调用 API：

```go
// 添加 Authorization header
req.Header.Set("Authorization", "Bearer "+session.AccessToken)

// 调用受保护 API
resp, _ := http.Get("http://localhost:80/api/v1/user/profile")
```

### Oathkeeper 验证

Oathkeeper 通过 `bearer_token` 认证器验证 token：

```json
{
    "id": "oauth2-api-routes",
    "authenticators": [
        {
            "handler": "bearer_token",
            "config": {
                "check_session_url": "http://hydra:4444/oauth2/introspect"
            }
        }
    ]
}
```

### 响应

```json
{
    "api_endpoint": "http://localhost:80/api/v1/user/profile",
    "status_code": 200,
    "response": {
        "code": 200,
        "message": "success",
        "data": {
            "id": 1,
            "email": "user@example.com",
            "username": "testuser"
        }
    }
}
```

## 流程三：Refresh Token 续期

### Access Token 过期

当 access_token 过期（默认 1 小时）：

```json
{
    "error": "invalid_token",
    "error_description": "The access token is expired"
}
```

### 使用 Refresh Token

```go
// Refresh Token 请求
data := url.Values{}
data.Set("grant_type", "refresh_token")
data.Set("refresh_token", session.RefreshToken)

// POST /oauth2/token
resp, _ := http.Post("http://localhost:4444/oauth2/token", ...)
```

### 响应

```json
{
    "access_token": "eyJhbGciOiJSUzI1NiIs...(new)",
    "token_type": "Bearer",
    "expires_in": 3600,
    "refresh_token": "ory_rt_xxx...(new)",
    "scope": "openid offline email profile"
}
```

## 流程四：OpenID Connect

### Discovery 端点

访问 `http://localhost:4444/.well-known/openid-configuration`：

```json
{
    "issuer": "http://localhost:4444/",
    "authorization_endpoint": "http://localhost:4444/oauth2/auth",
    "token_endpoint": "http://localhost:4444/oauth2/token",
    "userinfo_endpoint": "http://localhost:4444/userinfo",
    "jwks_uri": "http://localhost:4444/jwks.json",
    "scopes_supported": ["openid", "offline", "email", "profile"],
    "response_types_supported": ["code", "id_token", "token"],
    "grant_types_supported": ["authorization_code", "refresh_token", "implicit", "client_credentials"],
    "subject_types_supported": ["public"],
    "id_token_signing_alg_values_supported": ["RS256"]
}
```

### ID Token Claims

```json
{
    "iss": "http://localhost:4444/",
    "sub": "user-id-123",
    "aud": "bookstore-web-client",
    "exp": 1690000000,
    "iat": 1689996400,
    "email": "user@example.com",
    "name": "testuser",
    "email_verified": true
}
```

### UserInfo 端点

```bash
curl http://localhost:4444/userinfo \
  -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIs..."
```

响应：

```json
{
    "sub": "user-id-123",
    "email": "user@example.com",
    "name": "testuser",
    "email_verified": true
}
```

## 调试技巧

### 查看 Hydra 日志

```bash
docker compose logs -f hydra
```

### 查看 Login/Consent 日志

```bash
docker compose logs -f hydra-login-consent
```

### 手动调用 Hydra API

```bash
# 列出所有 Client
curl http://localhost:4445/admin/clients

# 获取 Client 详情
curl http://localhost:4445/admin/clients/bookstore-web-client

# 列出所有登录请求
curl http://localhost:4445/admin/oauth2/auth/requests/login

# 列出所有 consent 请求
curl http://localhost:4445/admin/oauth2/auth/requests/consent
```

### Token 内省

```bash
curl -X POST http://localhost:4444/oauth2/introspect \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=eyJhbGciOiJSUzI1NiIs..."
```

## 常见问题

### 1. "invalid_client" 错误

**原因**：Client ID 或 Secret 不正确

**解决**：
```bash
# 检查 Client 是否存在
curl http://localhost:4445/admin/clients

# 重新创建 Client
./scripts/create-hydra-client.sh
```

### 2. "redirect_uri_mismatch" 错误

**原因**：回调 URL 与注册的不一致

**解决**：确保 `redirect_uri` 参数与 Client 配置的 `redirect_uris` 完全一致

### 3. "login_required" 错误

**原因**：用户未登录 Kratos

**解决**：
1. 先通过 Kratos 注册/登录用户
2. 确保 Kratos Session Cookie 存在

### 4. "consent_required" 错误

**原因**：用户未同意授权

**解决**：确保 Consent Provider 正确处理 consent challenge
