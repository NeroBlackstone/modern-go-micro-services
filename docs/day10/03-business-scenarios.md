# Day 10 - 认证业务场景

## 场景 1：用户注册

### 业务流程
1. 用户访问注册页面
2. 输入邮箱、用户名、密码
3. Kratos 验证输入并创建 identity
4. 返回 session token（自动登录）

### API 调用

```bash
# 通过 Kratos 公共 API 注册
curl -X POST http://localhost:4433/self-service/registration/api \
  -H "Content-Type: application/json" \
  -d '{
    "traits": {
      "email": "user@example.com",
      "username": "testuser"
    },
    "password": "securepassword123"
  }'
```

### 响应示例
```json
{
  "id": "uuid-of-identity",
  "traits": {
    "email": "user@example.com",
    "username": "testuser"
  },
  "session": {
    "id": "session-id",
    "expires_at": "2024-01-15T00:00:00Z"
  }
}
```

## 场景 2：用户登录

### 业务流程
1. 用户访问登录页面
2. 输入邮箱和密码
3. Kratos 验证凭证
4. 颁发 session token（cookie）

### API 调用

```bash
# 通过 Kratos 公共 API 登录
curl -X POST http://localhost:4433/self-service/login/api \
  -H "Content-Type: application/json" \
  -d '{
    "identifier": "user@example.com",
    "password": "securepassword123"
  }'
```

### 响应示例
```json
{
  "session_token": "session-token-value",
  "session": {
    "id": "session-id",
    "active": true,
    "expires_at": "2024-01-15T00:00:00Z",
    "identity": {
      "id": "uuid-of-identity",
      "traits": {
        "email": "user@example.com",
        "username": "testuser"
      }
    }
  }
}
```

## 场景 3：访问受保护 API

### 业务流程
1. 用户携带 session token 访问 API
2. Traefik 转发到 Oathkeeper
3. Oathkeeper 验证 session
4. Oathkeeper 使用 JWT 变换器注入用户信息
5. 后端服务读取 Authorization header

### API 调用

```bash
# 使用 session token 访问受保护的订单 API
curl -X GET http://localhost/api/v1/orders \
  -H "Cookie: ory_kratos_session=session-token-value"
```

### 响应示例
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [...],
    "total": 10
  }
}
```

### 后端服务收到的请求
```http
GET /api/v1/orders HTTP/1.1
Host: order-service:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
X-User-ID: uuid-of-identity
X-User-Email: user@example.com
X-User-Username: testuser
```

## 场景 4：公开 API 访问

### 业务流程
1. 用户无需登录即可访问
2. Oathkeeper 使用 noop 认证器（不验证）
3. 直接转发到后端服务

### API 调用

```bash
# 无需认证，直接访问图书列表
curl -X GET http://localhost/api/v1/reviews/book/1
```

### 访问规则
```json
{
  "id": "public-review-routes",
  "match": {
    "url": "<http://localhost:4455/api/v1/reviews/book/**>",
    "methods": ["GET"]
  },
  "authenticators": [{ "handler": "noop" }],
  "authorizers": [{ "handler": "allow" }],
  "mutators": [{ "handler": "noop" }]
}
```

## 场景 5：社交登录（Google）

### 业务流程
1. 用户点击"使用 Google 登录"
2. 重定向到 Google OAuth2 授权页面
3. 用户授权后，Google 回调到 Kratos
4. Kratos 使用 Jsonnet 映射用户信息
5. 创建或关联 identity，颁发 session token

### 配置要点

```yaml
# Kratos 配置
selfservice:
  methods:
    oidc:
      enabled: true
      config:
        providers:
          - id: google
            provider: google
            client_id: YOUR_GOOGLE_CLIENT_ID
            client_secret: YOUR_GOOGLE_CLIENT_SECRET
            mapper_url: file:///etc/config/kratos/oidc.google.jsonnet
```

### Jsonnet 映射 (`oidc.google.jsonnet`)
```jsonnet
function(ctx) {
  email: ctx.raw.email,
  username: ctx.raw.email,
}
```

## 场景 6：账户恢复（忘记密码）

### 业务流程
1. 用户点击"忘记密码"
2. 输入注册邮箱
3. Kratos 发送恢复邮件
4. 用户点击邮件中的链接
5. 重置密码

### 配置要点
```yaml
selfservice:
  flows:
    recovery:
      enabled: true
      ui_url: http://localhost:4455/recovery
```

## 场景 7：会话验证

### 业务流程
1. 后端服务收到请求
2. 从 Authorization header 提取 JWT
3. 验证 JWT 签名和有效期
4. 提取用户信息

### Go 代码示例

```go
type OathkeeperClaims struct {
    Sub      string `json:"sub"`
    Email    string `json:"email"`
    Username string `json:"username"`
    jwt.RegisteredClaims
}

func JWTAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        parts := strings.SplitN(authHeader, " ", 2)

        token, err := jwt.ParseWithClaims(parts[1], &OathkeeperClaims{},
            func(token *jwt.Token) (any, error) {
                return []byte(cfg.JWT.Secret), nil
            })

        claims := token.Claims.(*OathkeeperClaims)
        c.Set("user_id", claims.Sub)
        c.Set("email", claims.Email)
        c.Set("username", claims.Username)

        c.Next()
    }
}
```

## 场景 8：会话过期处理

### 业务流程
1. Session token 过期
2. Oathkeeper 验证失败
3. 返回 401 Unauthorized
4. 客户端重定向到登录页面

### 配置要点
```yaml
selfservice:
  flows:
    login:
      lifespan: 720h  # 30 天
```

## 总结

| 场景 | 认证方式 | 访问规则 | 变换器 |
|------|---------|---------|--------|
| 用户注册 | noop | `/api/v1/auth/**` | noop |
| 用户登录 | noop | `/api/v1/auth/**` | noop |
| 受保护 API | cookie_session | `/api/v1/orders/**` | jwt |
| 公开 API | noop | `/api/v1/reviews/book/**` | noop |
| 社交登录 | OIDC | 通过 Kratos | - |
| 账户恢复 | email | 通过 Kratos | - |
