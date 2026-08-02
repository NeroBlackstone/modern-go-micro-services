# Day 11 - 测试和验证指南

## 快速开始

### 1. 启动所有服务

```bash
# 启动基础服务
docker compose up -d postgres-kratos postgres-hydra redis consul

# 等待数据库就绪
docker compose ps

# 启动 Ory 服务
docker compose up -d kratos-migrate hydra-migrate
docker compose up -d kratos hydra hydra-login-consent

# 启动应用服务
docker compose up -d user-service book-service order-service

# 启动 OAuth 客户端
docker compose up -d oauth-client-demo
```

### 2. 创建 OAuth2 Client

```bash
./scripts/create-hydra-client.sh
```

### 3. 注册测试用户

通过 Kratos 注册一个测试用户：

```bash
# 方法 1：通过 Kratos API
curl -X POST http://localhost:4433/self-service/registration/flows \
  -H "Content-Type: application/json" \
  -d '{
    "method": "password",
    "password": "testpass123",
    "traits": {
      "email": "test@example.com",
      "username": "testuser"
    }
  }'

# 方法 2：通过浏览器
# 访问 http://localhost:4433/self-service/registration/browser
```

### 4. 测试 OAuth2 流程

1. 访问 `http://localhost:8082`
2. 点击「使用 OAuth2 登录」
3. 在 Kratos 登录页面输入测试用户凭证
4. 完成授权流程
5. 查看获取的 Token

## 端到端测试

### 测试 1：完整 Authorization Code 流程

```bash
# 1. 发起授权请求
curl -v "http://localhost:4444/oauth2/auth?response_type=code&client_id=bookstore-web-client&redirect_uri=http://localhost:8082/callback&scope=openid+offline+email+profile&state=test123"

# 2. 跟随重定向完成登录和 consent

# 3. 检查回调 URL 中的 code
# http://localhost:8082/callback?code=xxx&state=test123
```

### 测试 2：用 Code 换取 Token

```bash
# 获取 authorization code（从回调 URL 中提取）
CODE="your-authorization-code"

# 交换 token
curl -X POST http://localhost:4444/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "Authorization: Basic $(echo -n 'bookstore-web-client:change-me-in-production' | base64)" \
  -d "grant_type=authorization_code&code=$CODE&redirect_uri=http://localhost:8082/callback"
```

预期响应：

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

### 测试 3：访问受保护 API

```bash
# 使用 access_token 调用 API
ACCESS_TOKEN="your-access-token"

curl -v http://localhost:80/api/v1/user/profile \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

### 测试 4：Token 内省

```bash
# 验证 token 有效性
curl -X POST http://localhost:4444/oauth2/introspect \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=$ACCESS_TOKEN"
```

预期响应：

```json
{
    "active": true,
    "scope": "openid offline email profile",
    "client_id": "bookstore-web-client",
    "sub": "user-id-123",
    "iss": "http://localhost:4444/",
    "exp": 1690000000,
    "iat": 1689996400,
    "token_type": "Bearer"
}
```

### 测试 5：Refresh Token 续期

```bash
# 使用 refresh_token 获取新的 access_token
REFRESH_TOKEN="your-refresh-token"

curl -X POST http://localhost:4444/oauth2/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "Authorization: Basic $(echo -n 'bookstore-web-client:change-me-in-production' | base64)" \
  -d "grant_type=refresh_token&refresh_token=$REFRESH_TOKEN"
```

### 测试 6：OpenID Connect Discovery

```bash
# 获取 OIDC 配置
curl http://localhost:4444/.well-known/openid-configuration

# 获取 JWKS
curl http://localhost:4444/jwks.json

# 获取用户信息
curl http://localhost:4444/userinfo \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

## 管理 API 测试

### 列出所有 OAuth2 Client

```bash
curl http://localhost:4445/admin/clients
```

### 创建新的 Client

```bash
curl -X POST http://localhost:4445/admin/clients \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "mobile-app",
    "grant_types": ["authorization_code", "refresh_token"],
    "response_types": ["code"],
    "scope": "openid email profile",
    "redirect_uris": ["myapp://callback"]
  }'
```

### 删除 Client

```bash
curl -X DELETE http://localhost:4445/admin/clients/bookstore-web-client
```

### 列出活跃的登录请求

```bash
curl http://localhost:4445/admin/oauth2/auth/requests/login
```

### 列出活跃的 consent 请求

```bash
curl http://localhost:4445/admin/oauth2/auth/requests/consent
```

## 监控和日志

### 查看服务状态

```bash
docker compose ps
```

### 查看日志

```bash
# Hydra 日志
docker compose logs -f hydra

# Login/Consent 日志
docker compose logs -f hydra-login-consent

# Kratos 日志
docker compose logs -f kratos

# Oathkeeper 日志
docker compose logs -f oathkeeper
```

### 健康检查

```bash
# Hydra 健康检查
curl http://localhost:4444/health/alive
curl http://localhost:4444/health/ready

# Hydra Admin 健康检查
curl http://localhost:4445/health/alive

# Login/Consent 健康检查
curl http://localhost:3001/health
```

## 性能测试

### 使用 hey 进行压力测试

```bash
# 安装 hey
go install github.com/rakyll/hey@latest

# 测试 Token 内省端点
hey -n 1000 -c 10 -m POST \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=$ACCESS_TOKEN" \
  http://localhost:4444/oauth2/introspect

# 测试 OIDC Discovery 端点
hey -n 1000 -c 10 http://localhost:4444/.well-known/openid-configuration
```

## 故障排查

### 问题 1：服务无法启动

```bash
# 检查容器状态
docker compose ps

# 检查日志
docker compose logs hydra

# 检查数据库连接
docker compose exec postgres-hydra psql -U hydra -d hydra -c "\dt"
```

### 问题 2：Token 交换失败

```bash
# 检查 Client 是否存在
curl http://localhost:4445/admin/clients

# 检查 redirect_uri 是否匹配
curl http://localhost:4445/admin/clients/bookstore-web-client | jq .redirect_uris

# 检查 code 是否已使用（code 只能使用一次）
```

### 问题 3：Kratos Session 验证失败

```bash
# 检查 Kratos 是否运行
curl http://localhost:4433/health/alive

# 手动验证 session
curl http://localhost:4433/sessions/whoami \
  -H "Cookie: ory_kratos_session=your-session-token"
```

### 问题 4：CORS 错误

检查 Hydra 和 Login/Consent 服务的 CORS 配置：

```yaml
# hydra.yml
serve:
  public:
    cors:
      enabled: true
      allowed_origins:
        - http://localhost:8082
```

## 自动化测试脚本

### 完整测试脚本

```bash
#!/bin/bash
set -e

echo "=== OAuth2 完整测试 ==="

# 1. 检查服务状态
echo "1. 检查服务状态..."
curl -s http://localhost:4444/health/alive > /dev/null && echo "✅ Hydra 可用" || echo "❌ Hydra 不可用"
curl -s http://localhost:4433/health/alive > /dev/null && echo "✅ Kratos 可用" || echo "❌ Kratos 不可用"
curl -s http://localhost:3001/health > /dev/null && echo "✅ Login/Consent 可用" || echo "❌ Login/Consent 不可用"

# 2. 检查 Client
echo "2. 检查 OAuth2 Client..."
CLIENTS=$(curl -s http://localhost:4445/admin/clients)
if echo "$CLIENTS" | grep -q "bookstore-web-client"; then
    echo "✅ Client 存在"
else
    echo "❌ Client 不存在，请运行 ./scripts/create-hydra-client.sh"
    exit 1
fi

# 3. 检查 OIDC Discovery
echo "3. 检查 OIDC Discovery..."
OIDC_CONFIG=$(curl -s http://localhost:4444/.well-known/openid-configuration)
if echo "$OIDC_CONFIG" | grep -q "authorization_endpoint"; then
    echo "✅ OIDC Discovery 正常"
else
    echo "❌ OIDC Discovery 异常"
fi

# 4. 测试 Token 内省（使用假 token）
echo "4. 测试 Token 内省..."
INTROSPECT=$(curl -s -X POST http://localhost:4444/oauth2/introspect \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "token=invalid-token")
if echo "$INTROSPECT" | grep -q '"active":false'; then
    echo "✅ Token 内省正常（正确拒绝无效 token）"
else
    echo "❌ Token 内省异常"
fi

echo ""
echo "=== 测试完成 ==="
echo ""
echo "下一步："
echo "1. 访问 http://localhost:8082 开始 OAuth2 流程"
echo "2. 查看日志: docker compose logs -f hydra-login-consent"
```

## 安全检查清单

- [ ] 生产环境启用 HTTPS
- [ ] 使用强随机的 Client Secret
- [ ] 启用 PKCE（Proof Key for Code Exchange）
- [ ] 设置合理的 Token 过期时间
- [ ] 启用 Refresh Token 轮换
- [ ] 验证 redirect_uri 白名单
- [ ] 监控异常的 Token 使用
- [ ] 定期轮换签名密钥
