# Day 9 - 限流详解：令牌桶与 Traefik 中间件

## 为什么需要限流？

### 过载场景

```
正常流量：100 QPS → 系统正常处理
突发流量：1000 QPS → 系统过载 → 响应变慢 → 超时 → 熔断 → 用户体验差
恶意攻击：10000 QPS → 系统崩溃
```

### 限流的作用

1. **保护服务**：防止过载导致崩溃
2. **公平性**：防止一个客户端占用所有资源
3. **成本控制**：限制资源消耗
4. **安全防护**：抵御 DDoS 攻击

## 限流算法

### 1. 令牌桶（Token Bucket）

**原理**：
- 桶以固定速率 `R` 产生令牌
- 桶最多容纳 `B` 个令牌（突发容量）
- 每个请求需要 1 个令牌
- 没有令牌时请求被拒绝

```
Rate = 10/s, Burst = 5

时间 0s: 桶满 (5 个令牌)
  请求 1-5: 全部通过（消耗 5 个令牌）
  请求 6: 拒绝（没有令牌）

时间 0.1s: 桶产生 1 个令牌
  请求 7: 通过

时间 0.2s: 桶产生 1 个令牌
  请求 8: 通过
```

**特点**：允许突发流量（最多 `Burst` 个请求同时通过）。

### 2. 漏桶（Leaky Bucket）

**原理**：
- 请求进入桶（队列）
- 桶以固定速率 `R` 处理请求
- 桶满时请求被拒绝

```
Rate = 10/s, Burst = 5

时间 0s: 5 个请求进入桶
  处理速率: 10/s
  每 0.1s 处理 1 个请求

时间 0.5s: 5 个请求全部处理完
```

**特点**：平滑流量，不允许突发。

### 3. 滑动窗口（Sliding Window）

**原理**：
- 统计过去 `N` 秒内的请求数
- 超过阈值时拒绝

```
Window = 1s, Limit = 10

时间 0-1s: 8 个请求 → 全部通过
时间 1-2s: 5 个请求 → 前 5 个通过，后 3 个拒绝
```

**特点**：精确但实现复杂。

### 算法对比

| 算法 | 突发流量 | 平滑性 | 实现复杂度 | 适用场景 |
|------|----------|--------|------------|----------|
| 令牌桶 | ✅ 允许 | 中 | 简单 | API 限流（推荐） |
| 漏桶 | ❌ 不允许 | 高 | 简单 | 流量整形 |
| 滑动窗口 | 中 | 高 | 复杂 | 精确计数 |

## gRPC 层限流实现

### 令牌桶实现

```go
// internal/resilience/rate_limiter.go

type RateLimiterConfig struct {
    Rate  float64  // 每秒令牌数
    Burst int      // 桶容量
}

type GRPCRateLimiter struct {
    limiter *rate.Limiter
}

func NewGRPCRateLimiter(cfg RateLimiterConfig) *GRPCRateLimiter {
    return &GRPCRateLimiter{
        limiter: rate.NewLimiter(rate.Limit(cfg.Rate), cfg.Burst),
    }
}

func (rl *GRPCRateLimiter) Allow() bool {
    return rl.limiter.Allow()
}
```

### gRPC 拦截器

```go
func RateLimiterInterceptor(rl *GRPCRateLimiter) grpc.UnaryClientInterceptor {
    return func(
        ctx context.Context,
        method string,
        req, reply any,
        cc *grpc.ClientConn,
        invoker grpc.UnaryInvoker,
        opts ...grpc.CallOption,
    ) error {
        if !rl.Allow() {
            // 记录限流指标
            metrics.RecordRateLimitRejection(ctx, "grpc", method)
            return status.Error(codes.ResourceExhausted, "rate limit exceeded")
        }
        return invoker(ctx, method, req, reply, cc, opts...)
    }
}
```

### 配置

```go
// book-service: 每秒 100 请求，突发 50
bookLimiter := resilience.NewGRPCRateLimiter(resilience.RateLimiterConfig{
    Rate:  100,
    Burst: 50,
})

// user-service: 每秒 50 请求，突发 20
userLimiter := resilience.NewGRPCRateLimiter(resilience.RateLimiterConfig{
    Rate:  50,
    Burst: 20,
})
```

## HTTP 层限流实现（Gin 中间件）

### Per-IP 限流

```go
// internal/middleware/rate_limiter.go

type IPRateLimiter struct {
    limiters sync.Map  // map[string]*rate.Limiter
    rate     rate.Limit
    burst    int
}

func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
    return &IPRateLimiter{
        rate:  rate.Limit(rps),
        burst: burst,
    }
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
    if limiter, ok := i.limiters.Load(ip); ok {
        return limiter.(*rate.Limiter)
    }

    limiter := rate.NewLimiter(i.rate, i.burst)
    i.limiters.Store(ip, limiter)

    // 定期清理（防止内存泄漏）
    go func() {
        time.Sleep(5 * time.Minute)
        i.limiters.Delete(ip)
    }()

    return limiter
}
```

### Gin 中间件

```go
func RateLimit(rps float64, burst int) gin.HandlerFunc {
    limiter := resilience.NewIPRateLimiter(rps, burst)

    return func(c *gin.Context) {
        ip := c.ClientIP()
        l := limiter.GetLimiter(ip)

        if !l.Allow() {
            c.Header("Retry-After", "1")
            c.AbortWithStatusJSON(429, gin.H{
                "code":    429,
                "message": "rate limit exceeded, please retry later",
            })
            return
        }

        c.Next()
    }
}
```

## Traefik 网关层限流

### 配置

```yaml
# compose.yml
labels:
  # 限流中间件：每秒 100 请求，突发 50
  - "traefik.http.middlewares.order-ratelimit.ratelimit.average=100"
  - "traefik.http.middlewares.order-ratelimit.ratelimit.burst=50"
  - "traefik.http.middlewares.order-ratelimit.ratelimit.period=1s"
  # 应用中间件
  - "traefik.http.routers.order.middlewares=order-ratelimit"
```

### Traefik 限流参数

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `average` | 平均速率（每秒请求数） | 100 |
| `burst` | 突发容量 | 50 |
| `period` | 时间窗口 | 1s |
| `sourceCriterion.ipStrategy.depth` | IP 提取深度 | 1 |

### Traefik vs 应用层限流

| 特性 | Traefik 网关 | 应用层（Gin） |
|------|-------------|---------------|
| 位置 | 入口处 | 服务内部 |
| 性能 | 高（C++ 实现） | 中（Go 实现） |
| 粒度 | 全局 | Per-service |
| 配置 | YAML 标签 | 代码 |
| 适用场景 | 全局限流 | 精细控制 |

**推荐**：
- **Traefik**：全局限流，保护整个系统
- **应用层**：精细控制，保护特定服务或端点

## 多层限流策略

```
用户请求
  → Traefik (全局限流: 1000 QPS)
    → order-service (应用层限流: 100 QPS per IP)
      → gRPC → book-service (gRPC 层限流: 100 QPS)
```

**为什么多层？**
1. **Traefik**：防止恶意流量进入系统
2. **应用层**：防止某个客户端占用所有资源
3. **gRPC 层**：防止服务间调用过载

## 演示场景

### 场景 1：触发 gRPC 限流

```bash
# 1. 设置低限流（每秒 5 请求）
bookLimiter := resilience.NewGRPCRateLimiter(resilience.RateLimiterConfig{
    Rate:  5,
    Burst: 2,
})

# 2. 快速发送 10 个订单请求
for i in {1..10}; do
  curl -X POST http://localhost/api/v1/orders \
    -H "Authorization: Bearer <token>" \
    -H "Content-Type: application/json" \
    -d '{"items":[{"book_id":1,"quantity":1}]}' &
done

# 3. 观察结果
# 前 2 个请求：通过（突发）
# 第 3-7 个请求：通过（正常速率）
# 第 8-10 个请求：429 Too Many Requests
```

### 场景 2：触发 Traefik 限流

```bash
# 1. 使用 wrk 发送高并发
wrk -t4 -c100 -d10s http://localhost/api/v1/orders

# 2. 观察 Traefik 日志
# 部分请求返回 429

# 3. 查看 Prometheus 指标
curl http://localhost:9090/api/v1/query?query=rate_limit_rejected_total
```

### 场景 3：Per-IP 限流

```bash
# 1. 客户端 A 发送 5 个请求
for i in {1..5}; do
  curl http://localhost/api/v1/auth/login ...
done

# 2. 客户端 B 发送 5 个请求（不同 IP）
for i in {1..5}; do
  curl http://localhost/api/v1/auth/login ...
done  # 全部通过

# 3. 客户端 A 再发送 1 个请求
curl http://localhost/api/v1/auth/login ...  # 429
```

## 限流响应

### HTTP 429 响应

```json
{
    "code": 429,
    "message": "rate limit exceeded, please retry later"
}
```

### Headers

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
Content-Type: application/json
```

**`Retry-After`**：告诉客户端多久后可以重试（秒）。

### gRPC ResourceExhausted

```go
status.Error(codes.ResourceExhausted, "rate limit exceeded")
```

客户端收到 `codes.ResourceExhausted`，可以根据退避策略重试。

## 总结

| 概念 | 说明 |
|------|------|
| **令牌桶** | 允许突发的限流算法 |
| **漏桶** | 平滑流量的限流算法 |
| **Rate** | 每秒令牌数（QPS 上限） |
| **Burst** | 桶容量（突发上限） |
| **Per-IP** | 按客户端 IP 限流 |
| **多层限流** | Traefik + 应用层 + gRPC 层 |
| **Retry-After** | 告诉客户端重试时间 |
