# Day 9 - 重试与退避策略详解

## 为什么需要重试？

瞬时故障是微服务中最常见的问题：

```
网络抖动：  请求 → 超时 → 重试 → 成功
GC 停顿：   请求 → 503 → 重试 → 成功
服务重启：  请求 → 连接拒绝 → 重试 → 成功
```

**重试不是万能的**：
- 持久故障（数据库宕机）：重试没用，只会加重负担
- 过载：重试会让情况更糟

**正确的做法**：只重试可重试的错误，并且有退避策略。

## 重试策略

### 1. 固定间隔重试

```
请求失败 → 等待 100ms → 重试
请求失败 → 等待 100ms → 重试
请求失败 → 等待 100ms → 重试
```

**问题**：所有客户端同时重试，形成重试风暴。

### 2. 指数退避

```
请求失败 → 等待 100ms → 重试
请求失败 → 等待 200ms → 重试
请求失败 → 等待 400ms → 重试
```

**改进**：等待时间指数增长，给下游更多恢复时间。

### 3. 指数退避 + 抖动（推荐）

```
请求失败 → 等待 100ms + random(0, 10ms) → 重试
请求失败 → 等待 200ms + random(0, 20ms) → 重试
请求失败 → 等待 400ms + random(0, 40ms) → 重试
```

**最佳**：抖动让重试时间分散，避免同步重试。

## 代码实现

### 配置结构

```go
// internal/resilience/retry.go

type RetryConfig struct {
    MaxRetries        int            // 最大重试次数
    InitialBackoff    time.Duration  // 初始退避时间
    MaxBackoff        time.Duration  // 最大退避上限
    BackoffMultiplier float64        // 退避倍数
    RetryableStatuses []codes.Code   // 可重试的 gRPC 状态码
}

func DefaultRetryConfig(logger *zap.Logger) RetryConfig {
    return RetryConfig{
        MaxRetries:        3,
        InitialBackoff:    100 * time.Millisecond,
        MaxBackoff:        2 * time.Second,
        BackoffMultiplier: 2.0,
        RetryableStatuses: []codes.Code{
            codes.Unavailable,       // 服务不可用（最常见）
            codes.DeadlineExceeded,  // 超时
            codes.ResourceExhausted, // 资源耗尽（限流）
            codes.Aborted,           // 操作被中止
        },
    }
}
```

### 退避计算

```go
func (cfg RetryConfig) calculateBackoff(attempt int) time.Duration {
    // 指数退避：initialBackoff * multiplier^attempt
    backoff := float64(cfg.InitialBackoff) * math.Pow(cfg.BackoffMultiplier, float64(attempt))

    // 限制最大退避
    if backoff > float64(cfg.MaxBackoff) {
        backoff = float64(cfg.MaxBackoff)
    }

    // 添加 jitter：[0, backoff*0.1]
    jitter := rand.Float64() * backoff * 0.1
    return time.Duration(backoff + jitter)
}
```

**计算示例**：

| attempt | base | jitter 范围 | 实际等待 |
|---------|------|-------------|----------|
| 0 | 100ms | [0, 10ms] | ~105ms |
| 1 | 200ms | [0, 20ms] | ~210ms |
| 2 | 400ms | [0, 40ms] | ~420ms |
| 3 | 800ms | [0, 80ms] | ~830ms |
| 4 | 1600ms | [0, 160ms] | ~1680ms |
| 5 | 3200ms | [0, 320ms] | 2000ms (被 cap) |

### 拦截器实现

```go
func RetryInterceptor(cfg RetryConfig) grpc.UnaryClientInterceptor {
    return func(
        ctx context.Context,
        method string,
        req, reply any,
        cc *grpc.ClientConn,
        invoker grpc.UnaryInvoker,
        opts ...grpc.CallOption,
    ) error {
        var lastErr error

        for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
            // 第一次调用不需要等待
            if attempt > 0 {
                backoff := cfg.calculateBackoff(attempt - 1)
                // 等待退避时间，但尊重 context 取消
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case <-time.After(backoff):
                }
            }

            lastErr = invoker(ctx, method, req, reply, cc, opts...)
            if lastErr == nil {
                return nil  // 成功，直接返回
            }

            // 如果错误不可重试，直接返回
            if !cfg.isRetryable(lastErr) {
                return lastErr
            }
        }

        return lastErr  // 所有重试都失败
    }
}
```

**执行流程**：

```
attempt=0: 调用 invoker()
  → 成功: 返回 nil
  → 失败: 检查是否可重试
    → 不可重试: 返回错误
    → 可重试: 继续

attempt=1: 等待 100ms + jitter
  → 调用 invoker()
  → ...

attempt=2: 等待 200ms + jitter
  → 调用 invoker()
  → ...

attempt=3: 等待 400ms + jitter
  → 调用 invoker()
  → 失败: 返回错误
```

## 可重试的错误

### gRPC 状态码分类

| 状态码 | 含义 | 可重试？ | 原因 |
|--------|------|----------|------|
| `OK` | 成功 | N/A | - |
| `Canceled` | 客户端取消 | ❌ | 客户端主动取消 |
| `Unknown` | 未知错误 | ❌ | 可能是 bug |
| `InvalidArgument` | 参数错误 | ❌ | 客户端问题 |
| `DeadlineExceeded` | 超时 | ✅ | 可能是瞬时故障 |
| `NotFound` | 资源不存在 | ❌ | 业务逻辑 |
| `AlreadyExists` | 资源已存在 | ❌ | 业务逻辑 |
| `PermissionDenied` | 权限不足 | ❌ | 客户端问题 |
| `ResourceExhausted` | 资源耗尽 | ✅ | 可能是瞬时过载 |
| `FailedPrecondition` | 前置条件不满足 | ❌ | 业务逻辑 |
| `Aborted` | 操作被中止 | ✅ | 可能是并发冲突 |
| `OutOfRange` | 超出范围 | ❌ | 客户端问题 |
| `Unimplemented` | 未实现 | ❌ | 服务端问题 |
| `Internal` | 内部错误 | ❌ | 可能是 bug |
| `Unavailable` | 服务不可用 | ✅ | 服务重启/网络问题 |
| `DataLoss` | 数据丢失 | ❌ | 严重错误 |
| `Unauthenticated` | 未认证 | ❌ | 客户端问题 |

### 选择可重试状态码

```go
RetryableStatuses: []codes.Code{
    codes.Unavailable,       // 服务不可用（最常见）
    codes.DeadlineExceeded,  // 超时
    codes.ResourceExhausted, // 资源耗尽
    codes.Aborted,           // 操作被中止
},
```

**不要重试的错误**：
- `InvalidArgument`：客户端参数错误，重试还是会失败
- `NotFound`：资源不存在，重试也不会出现
- `PermissionDenied`：权限不足，重试也没用
- `Internal`：服务端 bug，重试可能加重问题

## 重试风暴问题

### 问题场景

```
100 个客户端同时调用 book-service
  → book-service 宕机
    → 100 个客户端同时重试（100ms 后）
      → 200 个请求同时到达（book-service 刚恢复）
        → book-service 再次过载
          → 更多重试...
```

### 解决方案：Jitter

```go
// 没有 jitter：所有客户端在 100ms 后同时重试
time.After(100ms)  // 100 个请求同时到达

// 有 jitter：重试时间分散
time.After(100ms + random(0, 10ms))
// 客户端 1: 102ms
// 客户端 2: 108ms
// 客户端 3: 105ms
// ... 分散在 100-110ms 范围内
```

### 其他缓解策略

1. **限制最大重试次数**：`MaxRetries: 3`
2. **设置退避上限**：`MaxBackoff: 2s`
3. **只重试特定错误**：`RetryableStatuses`
4. **结合熔断器**：连续失败触发熔断，避免无限重试

## 演示场景

### 场景 1：瞬时故障自动恢复

```bash
# 1. 间歇性故障（用 toxiproxy 模拟）
# book-service 偶尔返回 Unavailable

# 2. 创建订单
curl -X POST http://localhost/api/v1/orders ...

# 3. 观察日志
# 第 1 次调用：failed (Unavailable)
# 第 2 次调用（100ms 后）：failed (Unavailable)
# 第 3 次调用（200ms 后）：success!

# 4. 查看 trace
# 看到 3 个 span，每个间隔递增
```

### 场景 2：不可重试错误直接失败

```bash
# 1. book-service 返回 InvalidArgument（参数错误）

# 2. 创建订单
curl -X POST http://localhost/api/v1/orders ...

# 3. 观察日志
# 第 1 次调用：failed (InvalidArgument)
# 直接返回错误，没有重试
```

### 场景 3：Context 取消

```bash
# 1. 客户端设置短超时（如 500ms）

# 2. book-service 响应慢（如 2s）

# 3. 创建订单
curl -X POST http://localhost/api/v1/orders --max-time 0.5 ...

# 4. 观察日志
# 第 1 次调用：超时
# 尝试等待退避时间，但 context 已取消
# 返回 context.DeadlineExceeded
```

## 重试与熔断器的协作

```
请求进入
  → CircuitBreaker
    → Closed: 允许调用
      → Retry
        → attempt 0: 失败 (Unavailable)
        → attempt 1: 失败 (Unavailable)
        → attempt 2: 失败 (Unavailable)
      → 返回错误给 CircuitBreaker
    → CircuitBreaker: 失败计数 +1
    → 连续失败 > 5 次? → OPEN
  → 后续请求直接被 CircuitBreaker 拒绝
```

**关键点**：每次重试失败都会被熔断器统计。如果 3 次重试都失败，相当于 3 次失败计数。

## 总结

| 概念 | 说明 |
|------|------|
| **MaxRetries** | 最大重试次数（避免无限重试） |
| **InitialBackoff** | 初始等待时间 |
| **BackoffMultiplier** | 退避倍数（通常 2.0） |
| **MaxBackoff** | 最大等待时间上限 |
| **Jitter** | 随机抖动（防止重试风暴） |
| **RetryableStatuses** | 可重试的 gRPC 状态码 |
| **Context** | 尊重 context 取消 |
