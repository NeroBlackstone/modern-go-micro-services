# Gin Context：请求上下文

## 是什么

`gin.Context` 是 Gin 框架中每个 HTTP 请求的核心载体。每次请求进来时创建一个实例，贯穿请求的完整生命周期。它封装了请求读取、响应构建和中间件间数据传递的能力。

```go
func (h *UserHandler) Register(c *gin.Context) {
    // c 就是这次请求的上下文
}
```

---

## 读取请求信息

| 场景 | 方法 | 说明 |
|---|---|---|
| JSON 请求体 | `c.ShouldBindJSON(&v)` | POST/PUT 时绑定到结构体 |
| 查询参数 | `c.ShouldBindQuery(&v)` | GET 列表的分页/筛选 |
| URL 路径参数 | `c.Param("id")` | `:id` 占位符的值，返回 string |
| 请求头 | `c.GetHeader("Authorization")` | 读取指定 Header |
| 底层请求 | `c.Request` | 原生 `*http.Request` |

**示例 — 绑定 JSON 请求体**（`internal/handler/user_handler.go`）：

```go
func (h *UserHandler) Register(c *gin.Context) {
    var req model.RegisterRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, err.Error())
        return
    }
    result, err := h.userSvc.Register(&req)
    // ...
}
```

**示例 — 获取路径参数**（`internal/handler/book_handler.go`）：

```go
// 路由: GET /api/v1/books/:id
id, err := strconv.ParseUint(c.Param("id"), 10, 32)
```

**示例 — 绑定查询参数**（`internal/handler/book_handler.go`）：

```go
// 请求: GET /api/v1/books?page=1&page_size=10&keyword=golang
var query model.BookQuery
c.ShouldBindQuery(&query)
// 对应结构体用 form 标签: Page int `form:"page"`
```

---

## 构建响应

| 操作 | 方法 |
|---|---|
| 返回 JSON | `c.JSON(200, data)` |
| 设置响应头 | `c.Header("key", "value")` |
| 中断请求链 | `c.Abort()` |
| 中断并返回状态码 | `c.AbortWithStatus(401)` |
| 中断并返回 JSON | `c.AbortWithStatusJSON(500, obj)` |

项目中不直接调用 `c.JSON()`，而是通过 `pkg/response` 封装：

```go
response.Success(c, user)        // 200 + {code:0, message:"success", data:...}
response.BadRequest(c, err.Error())  // 400
response.Unauthorized(c, "...")      // 401
```

---

## 中间件间传值 — `Set` / `Get`

`gin.Context` 可以在中间件和 Handler 之间传递数据，这是它最强大的能力。

**中间件存值**（`internal/middleware/jwt.go`）：

```go
// JWT 中间件解析 Token 后，将用户信息存入 Context
c.Set("user_id", claims.UserID)
c.Set("email", claims.Email)
c.Next()  // 继续执行后续中间件和 Handler
```

**Handler 取值**：

```go
// GetCurrentUserID 从 Context 中取出中间件存入的 user_id
func GetCurrentUserID(c *gin.Context) (uint, bool) {
    userID, exists := c.Get("user_id")  // 返回 interface{}，需要类型断言
    if !exists {
        response.Unauthorized(c, "user not authenticated")
        return 0, false
    }
    id, ok := userID.(uint)
    return id, ok
}
```

---

## 中间件链控制 — `Next` / `Abort`

```
请求 → CORS → Logger → JWT → Handler → 返回
              c.Next()  c.Next()  c.Next()
```

- **`c.Next()`**：传递控制给下一环，后续环节执行完后返回到 `c.Next()` 之后
- **`c.Abort()`**：中断，不再执行后续中间件和 Handler

**场景 — JWT 认证失败**：调用 `c.Abort()` 后 Handler 不会执行。

**场景 — CORS OPTIONS 预检**：`c.AbortWithStatus(204)` 直接返回，不进入业务逻辑。

**场景 — Logger 中间件**：`c.Next()` 前记录开始时间，返回后记录耗时和状态码。

---

## 速查表

| 操作 | 方法 |
|---|---|
| 绑定 JSON | `c.ShouldBindJSON(&v)` |
| 绑定查询参数 | `c.ShouldBindQuery(&v)` |
| URL 路径参数 | `c.Param("name")` |
| 读取请求头 | `c.GetHeader("Name")` |
| 读取底层请求 | `c.Request` |
| 设置 Context 值 | `c.Set(key, val)` |
| 读取 Context 值 | `c.Get(key)` |
| 设置响应头 | `c.Header(k, v)` |
| 返回 JSON | `c.JSON(code, obj)` |
| 中断请求链 | `c.Abort()` |
| 传递控制 | `c.Next()` |

---

**文档版本**: v1.0
**创建时间**: 2026-06-16
**关联文档**: [phase-1.1 项目结构](./phase-1.1-project-structure.md)
