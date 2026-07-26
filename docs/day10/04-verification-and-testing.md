# Day 10 - 验证与测试

## 启动服务

### 1. 启动 Ory 基础设施

```bash
# 启动 Kratos 和 Oathkeeper
docker compose up -d postgres-kratos kratos-migrate kratos oathkeeper

# 检查服务状态
docker compose ps | grep -E "kratos|oathkeeper"
```

### 2. 验证 Kratos 服务

```bash
# 检查 Kratos 健康状态
curl http://localhost:4433/health/alive

# 检查 Kratos 就绪状态
curl http://localhost:4433/health/ready
```

### 3. 验证 Oathkeeper 服务

```bash
# 检查 Oathkeeper 健康状态
curl http://localhost:4455/health/alive
```

## 测试注册流程

### 测试用户注册

```bash
# 注册新用户
curl -X POST http://localhost:4433/self-service/registration/api \
  -H "Content-Type: application/json" \
  -d '{
    "traits": {
      "email": "testuser@example.com",
      "username": "testuser"
    },
    "password": "securepassword123"
  }'
```

**预期响应**：
```json
{
  "id": "uuid-of-identity",
  "traits": {
    "email": "testuser@example.com",
    "username": "testuser"
  },
  "session": {
    "id": "session-id",
    "expires_at": "2024-01-15T00:00:00Z"
  }
}
```

### 测试重复注册

```bash
# 尝试用相同邮箱注册
curl -X POST http://localhost:4433/self-service/registration/api \
  -H "Content-Type: application/json" \
  -d '{
    "traits": {
      "email": "testuser@example.com",
      "username": "anotheruser"
    },
    "password": "anotherpassword123"
  }'
```

**预期响应**：
```json
{
  "error": {
    "code": 400,
    "message": "an identity with this email address already exists"
  }
}
```

## 测试登录流程

### 测试用户登录

```bash
# 使用邮箱和密码登录
curl -X POST http://localhost:4433/self-service/login/api \
  -H "Content-Type: application/json" \
  -d '{
    "identifier": "testuser@example.com",
    "password": "securepassword123"
  }'
```

**预期响应**：
```json
{
  "session_token": "session-token-value",
  "session": {
    "id": "session-id",
    "active": true,
    "identity": {
      "id": "uuid-of-identity",
      "traits": {
        "email": "testuser@example.com",
        "username": "testuser"
      }
    }
  }
}
```

### 测试错误密码

```bash
# 使用错误密码登录
curl -X POST http://localhost:4433/self-service/login/api \
  -H "Content-Type: application/json" \
  -d '{
    "identifier": "testuser@example.com",
    "password": "wrongpassword"
  }'
```

**预期响应**：
```json
{
  "error": {
    "code": 400,
    "message": "credentials are invalid"
  }
}
```

## 测试 API 访问

### 测试公开 API（无需认证）

```bash
# 访问图书评论列表（公开 API）
curl -X GET http://localhost/api/v1/reviews/book/1
```

**预期**：返回 200 OK 和评论列表

### 测试受保护 API（需要认证）

```bash
# 尝试不带认证访问订单 API
curl -X GET http://localhost/api/v1/orders
```

**预期**：返回 401 Unauthorized

### 测试带认证访问

```bash
# 使用 session token 访问订单 API
curl -X GET http://localhost/api/v1/orders \
  -H "Cookie: ory_kratos_session=session-token-value"
```

**预期**：返回 200 OK 和订单列表

## 测试 Oathkeeper Decision API

### 测试公开路由

```bash
# 测试公开路由的 Decision API
curl -X GET http://localhost:4455/decisions/api/v1/auth/register
```

**预期**：返回 200 OK（允许访问）

### 测试受保护路由

```bash
# 测试受保护路由的 Decision API（无 session）
curl -X GET http://localhost:4455/decisions/api/v1/orders
```

**预期**：返回 401 Unauthorized（拒绝访问）

### 测试带 Session 的 Decision API

```bash
# 测试受保护路由的 Decision API（带 session）
curl -X GET http://localhost:4455/decisions/api/v1/orders \
  -H "Cookie: ory_kratos_session=session-token-value"
```

**预期**：返回 200 OK（允许访问）

## 测试用户信息

### 获取 Session 信息

```bash
# 使用 session token 获取用户信息
curl -X GET http://localhost:4433/sessions/whoami \
  -H "Cookie: ory_kratos_session=session-token-value"
```

**预期响应**：
```json
{
  "id": "session-id",
  "active": true,
  "identity": {
    "id": "uuid-of-identity",
    "traits": {
      "email": "testuser@example.com",
      "username": "testuser"
    }
  }
}
```

### 获取用户列表（管理 API）

```bash
# 获取所有用户列表
curl -X GET http://localhost:4434/admin/identities
```

**预期响应**：
```json
[
  {
    "id": "uuid-of-identity",
    "traits": {
      "email": "testuser@example.com",
      "username": "testuser"
    }
  }
]
```

## 常见问题

### 1. Kratos 启动失败

**检查日志**：
```bash
docker compose logs kratos
```

**常见原因**：
- 数据库连接失败
- 配置文件错误
- 端口被占用

### 2. Oathkeeper 验证失败

**检查日志**：
```bash
docker compose logs oathkeeper
```

**常见原因**：
- Kratos 服务不可用
- 访问规则配置错误
- Session 过期

### 3. CORS 错误

**检查配置**：
```yaml
# Kratos CORS 配置
serve:
  public:
    cors:
      enabled: true
      allow_origins:
        - http://localhost:4455
        - http://localhost:80
```

### 4. Cookie 问题

**检查 Cookie 配置**：
```yaml
security:
  cookie:
    secure: false  # 开发环境
    name: ory_kratos_session
    same_site: Lax
```

## 验证清单

- [ ] Kratos 服务启动成功
- [ ] Oathkeeper 服务启动成功
- [ ] 用户注册成功
- [ ] 用户登录成功
- [ ] 公开 API 访问正常
- [ ] 受保护 API 访问正常
- [ ] 未认证访问被拒绝
- [ ] Session 过期处理正常
- [ ] 社交登录配置正确（可选）
