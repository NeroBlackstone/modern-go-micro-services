# Day 13 - 服务间认证：代码实现详解

## 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        请求完整流程                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Client                                                         │
│    │  HTTP Request + Kratos Cookie                              │
│    ▼                                                            │
│  Traefik (:80)                                                  │
│    │  ForwardAuth → Oathkeeper → 验证 Session                   │
│    │  返回 JWT in Authorization header                          │
│    ▼                                                            │
│  order-service (:8080)                                          │
│    │  JWTAuth middleware: 验证 Oathkeeper JWT                    │
│    │  提取 user_id, email, username                             │
│    │  注入 caller_user, caller_email 到 context                 │
│    │                                                            │
│    │  gRPC 拦截器链:                                            │
│    │  Tracing → ServiceAuth → CircuitBreaker → Retry → Limiter  │
│    │  ServiceAuth: 从 context 提取用户信息                       │
│    │               签发 service JWT (HMAC-SHA256)               │
│    │               放入 gRPC metadata: authorization            │
│    ▼                                                            │
│  book-service (:9092)                                           │
│    │  gRPC 拦截器链:                                            │
│    │  Tracing → ServiceAuth                                     │
│    │  ServiceAuth: 从 metadata 提取 JWT                         │
│    │               验证签名 + aud 匹配                          │
│    │               提取 caller 信息注入 context                 │
│    ▼                                                            │
│  Handler 处理请求                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 核心代码

### 1. 配置结构 (`internal/serviceauth/config.go`)

```go
type Config struct {
    SharedSecret  string        // HMAC 共享密钥
    ServiceName   string        // 当前服务名称（iss）
    TargetService string        // 目标服务名称（aud）
    TTL           time.Duration // Token 有效期
}
```

两个服务共享同一个 `SharedSecret`，但 `ServiceName` 和 `TargetService` 不同：

| 服务 | ServiceName | TargetService |
|------|-------------|---------------|
| order-service | `order-service` | `book-service` |
| book-service | `book-service` | - (不签发) |

### 2. JWT 签发与验证 (`internal/serviceauth/token.go`)

**签发 Token：**

```go
func GenerateToken(cfg *Config, callerUser, callerEmail string) (string, error) {
    claims := ServiceClaims{
        Iss:         cfg.ServiceName,   // 签发者
        Aud:         cfg.TargetService, // 接收者
        Sub:         cfg.ServiceName,   // 调用方
        CallerUser:  callerUser,        // 原始用户 ID
        CallerEmail: callerEmail,       // 原始用户邮箱
        RegisteredClaims: jwt.RegisteredClaims{
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(cfg.TTL)),
        },
    }
    return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.SharedSecret))
}
```

**验证 Token：**

```go
func ValidateToken(cfg *Config, tokenString string) (*ServiceClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &ServiceClaims{}, func(token *jwt.Token) (any, error) {
        return []byte(cfg.SharedSecret), nil  // 使用共享密钥验证
    })
    claims := token.Claims.(*ServiceClaims)
    if claims.Aud != cfg.ServiceName {  // 验证目标服务匹配
        return nil, fmt.Errorf("audience mismatch")
    }
    return claims, nil
}
```

### 3. gRPC 拦截器 (`internal/serviceauth/interceptor.go`)

**客户端拦截器（order-service 使用）：**

```go
func UnaryClientInterceptor(cfg *Config, logger *zap.Logger) grpc.UnaryClientInterceptor {
    return func(ctx context.Context, method string, req, reply any,
        cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

        // 1. 从 context 提取原始用户信息
        callerUser := ctx.Value(CallerUserKey).(string)
        callerEmail := ctx.Value(CallerEmailKey).(string)

        // 2. 签发服务间 JWT
        token, _ := GenerateToken(cfg, callerUser, callerEmail)

        // 3. 放入 gRPC metadata
        md := metadata.New(nil)
        md.Set("authorization", "Bearer "+token)
        ctx = metadata.NewOutgoingContext(ctx, md)

        // 4. 继续调用
        return invoker(ctx, method, req, reply, cc, opts...)
    }
}
```

**服务端拦截器（book-service 使用）：**

```go
func UnaryServerInterceptor(cfg *Config, logger *zap.Logger) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler) (any, error) {

        // 1. 从 metadata 提取 JWT
        md, _ := metadata.FromIncomingContext(ctx)
        authHeader := md.Get("authorization")[0]

        // 2. 验证 JWT
        claims, err := ValidateToken(cfg, tokenString)

        // 3. 将调用方信息注入 context
        ctx = context.WithValue(ctx, CallerServiceKey, claims.Sub)
        ctx = context.WithValue(ctx, CallerUserKey, claims.CallerUser)

        return handler(ctx, req)
    }
}
```

### 4. Context 传播

**关键点**：order-service 的 JWTAuth 中间件需要将用户信息注入到请求 context 中：

```go
// order_handler.go - JWTAuth middleware
ctx := context.WithValue(c.Request.Context(), serviceauth.CallerUserKey, userIDStr)
ctx = context.WithValue(ctx, serviceauth.CallerEmailKey, claims.Email)
c.Request = c.Request.WithContext(ctx)
```

这样当 order-service 调用 book-service 时，客户端拦截器可以从 context 中提取用户信息，放入 service JWT。

### 5. 配置文件

**order-service.yaml：**

```yaml
service_auth:
  shared_secret: "${SERVICE_AUTH_SECRET}"
  service_name: "order-service"
  target_service: "book-service"
  ttl: "5m"
```

**book-service.yaml：**

```yaml
service_auth:
  shared_secret: "${SERVICE_AUTH_SECRET}"
  service_name: "book-service"
```

**compose.yml：**

```yaml
environment:
  - SERVICE_AUTH_SECRET=${SERVICE_AUTH_SECRET:-super-secret-key-change-in-production}
```

## 拦截器链顺序

### order-service（客户端）

```
Tracing → ServiceAuth → CircuitBreaker → Retry → RateLimiter
   │          │              │              │           │
   ▼          ▼              ▼              ▼           ▼
 创建Span   签发JWT       检查状态      失败重试    限流控制
```

### book-service（服务端）

```
Tracing → ServiceAuth → Handler
   │          │              │
   ▼          ▼              ▼
 提取Span   验证JWT      处理请求
            提取caller
```

## 代码修改清单

| 文件 | 修改内容 |
|------|----------|
| `internal/serviceauth/config.go` | **新建** 配置结构 |
| `internal/serviceauth/token.go` | **新建** JWT 签发/验证 |
| `internal/serviceauth/interceptor.go` | **新建** gRPC 拦截器 |
| `internal/order/client/book_client.go` | 方法签名添加 `context.Context` |
| `internal/order/config/config.go` | 添加 `ServiceAuthConfig` |
| `internal/order/handler/order_handler.go` | JWTAuth 注入 caller 到 context |
| `configs/order-service.yaml` | 添加 `service_auth` 配置 |
| `cmd/order-service/main.go` | 添加 serviceauth 拦截器 |
| `internal/book/config/config.go` | 添加 `ServiceAuthConfig` |
| `internal/book/server/server.go` | 添加 serviceauth 拦截器 |
| `configs/book-service.yaml` | 添加 `service_auth` 配置 |
| `cmd/book-service/main.go` | 加载配置创建拦截器 |
| `compose.yml` | 添加 `SERVICE_AUTH_SECRET` 环境变量 |

## 下一步

在下一个文档中，我们将介绍如何测试和调试服务间认证。
