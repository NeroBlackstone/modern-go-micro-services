# Gin Middleware：中间件机制

## 是什么

中间件是拦截 HTTP 请求的函数，在请求到达 Handler **之前**（或之后）执行。典型用途：日志记录、认证鉴权、跨域处理、panic 恢复。

```
请求 → Recovery → Logger → CORS → JWTAuth → Handler → 返回
```

Gin 中所有中间件和 Handler 的签名统一为 `func(c *gin.Context)`，所以中间件可以在任意位置插入链条。

---

## 创建中间件

中间件本质是**返回闭包**，闭包捕获外部依赖（logger、config 等）：

```go
// internal/middleware/logger.go
func Logger(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path

        c.Next()  // 执行后续中间件和 Handler，返回后可拿到响应信息

        status := c.Writer.Status()
        latency := time.Since(start)
        logger.Info("request",
            zap.Int("status", status),
            zap.String("path", path),
            zap.Duration("latency", latency),
        )
    }
}
```

`c.Next()` 之前 = 请求阶段（读取请求信息）；`c.Next()` 之后 = 响应阶段（可拿到状态码、耗时等）。

---

## 三种应用方式

粒度从大到小：

### 1. 全局中间件 — `engine.Use()`

对**所有请求**生效（`internal/handler/router.go`）：

```go
r.engine.Use(middleware.Recovery(r.logger))
r.engine.Use(middleware.Logger(r.logger))
r.engine.Use(middleware.CORS())
```

### 2. 路由组中间件 — `group.Use()`

对**某个路由组**生效：

```go
orders := v1.Group("/orders")
orders.Use(middleware.JWTAuth(&r.cfg.JWT))
{
    orders.POST("", r.orderHandler.CreateOrder)
    orders.GET("", r.orderHandler.ListOrders)
}
```

### 3. 单条路由中间件 — 作为路由参数

只对**一条路由**生效：

```go
books.GET("", r.bookHandler.ListBooks)                                     // 公开
books.POST("", middleware.JWTAuth(&r.cfg.JWT), r.bookHandler.CreateBook)   // 需认证
```

路由注册函数签名是 `handlers ...HandlerFunc`，中间件和 Handler 类型相同，按参数顺序依次执行。

---

## 链条控制：Next / Abort

| 方法 | 作用 |
|---|---|
| `c.Next()` | 传递给下一环节，后续执行完后返回到 `c.Next()` 之后 |
| `c.Abort()` | 中断链条，不再执行后续中间件和 Handler |
| `c.AbortWithStatus(code)` | 中断并设置 HTTP 状态码 |
| `c.AbortWithStatusJSON(code, obj)` | 中断并返回 JSON |

**认证失败时中断**（`internal/middleware/jwt.go`）：

```go
func JWTAuth(cfg *config.JWTConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            response.Unauthorized(c, "missing authorization header")
            c.Abort()  // Handler 不会执行
            return
        }
        // ... 解析 Token ...
        c.Set("user_id", claims.UserID)
        c.Next()  // 认证通过，继续
    }
}
```

**CORS 预检请求直接返回**（`internal/middleware/cors.go`）：

```go
func CORS() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        // ...
        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(204)  // 不进入业务逻辑
            return
        }
        c.Next()
    }
}
```

**panic 恢复**（`internal/middleware/logger.go`）：

```go
func Recovery(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if err := recover(); err != nil {
                c.AbortWithStatusJSON(500, gin.H{
                    "code": 500, "message": "internal server error",
                })
            }
        }()
        c.Next()
    }
}
```

---

## 中间件间传值

中间件通过 `c.Set()` 写入数据，Handler 通过 `c.Get()` 读取（`internal/middleware/jwt.go`）：

```go
// 中间件写入
c.Set("user_id", claims.UserID)

// Handler 读取
func GetCurrentUserID(c *gin.Context) (uint, bool) {
    userID, exists := c.Get("user_id")  // 返回 any，需类型断言
    if !exists {
        response.Unauthorized(c, "user not authenticated")
        return 0, false
    }
    id, ok := userID.(uint)
    return id, ok
}
```

---

## 执行顺序

以 `POST /api/v1/books` 为例，完整链条：

```
1. Recovery       ← engine.Use()   全局，最先注册
2. Logger         ← engine.Use()   全局
3. CORS           ← engine.Use()   全局
4. JWTAuth        ← 路由级参数      单条路由
5. CreateBook                          Handler

请求方向:  1 → 2 → 3 → 4 → 5
响应方向:  5 → 4 → 3 → 2 → 1
```

Logger 中间件在 `c.Next()` 前记录开始时间，`c.Next()` 返回后记录耗时——这时所有后续环节（包括 Handler）都已执行完毕。

---

**文档版本**: v1.0
**创建时间**: 2026-06-16
**关联文档**: [phase-1.2 Gin Context](./phase-1.2-gin-context.md)
