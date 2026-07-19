# Day 9 - 熔断器实现详解

## 熔断器原理

### 为什么需要熔断器？

假设 book-service 宕机，没有熔断器时：

```
用户请求 → order-service → book-service (超时 5s)
用户请求 → order-service → book-service (超时 5s)
用户请求 → order-service → book-service (超时 5s)
... 100 个请求同时进来 ...
order-service 的 100 个线程全部阻塞 5s
→ 系统完全不可用
```

有了熔断器：

```
用户请求 → order-service → book-service (失败 1)
用户请求 → order-service → book-service (失败 2)
用户请求 → order-service → book-service (失败 3)
用户请求 → order-service → book-service (失败 4)
用户请求 → order-service → book-service (失败 5)
用户请求 → order-service → 熔断器 OPEN → 直接返回错误 (0ms)
```

### 三态模型详解

```
┌─────────────────────────────────────────────────────────────┐
│                      Circuit Breaker                        │
├─────────────┬─────────────────┬─────────────────────────────┤
│   CLOSED    │      OPEN       │         HALF-OPEN           │
├─────────────┼─────────────────┼─────────────────────────────┤
│ 放行所有请求 │ 拒绝所有请求     │ 允许少量请求通过              │
│ 统计失败率   │ 返回快速失败     │ 测试下游是否恢复              │
│ 失败率>阈值  │ 超时后→HALF-OPEN│ 成功→CLOSED                │
│ →OPEN       │                 │ 失败→OPEN                   │
└─────────────┴─────────────────┴─────────────────────────────┘
```

## 代码实现

### 核心结构

```go
// internal/resilience/circuit_breaker.go

package resilience

import (
    "github.com/sony/gobreaker/v2"
    "go.uber.org/zap"
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
    Name        string                              // 熔断器名称
    MaxRequests uint32                              // Half-Open 允许的并发数
    Interval    time.Duration                       // Closed 状态清零间隔
    Timeout     time.Duration                       // Open → Half-Open 等待时间
    ReadyToTrip func(counts gobreaker.Counts) bool  // 触发熔断的条件
}
```

### 创建熔断器

```go
func NewCircuitBreaker(cfg CircuitBreakerConfig, logger *zap.Logger) *gobreaker.CircuitBreaker[any] {
    return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name:        cfg.Name,
        MaxRequests: cfg.MaxRequests,     // Half-Open 时允许 3 个请求通过
        Interval:    cfg.Interval,        // 每 30s 清零计数器
        Timeout:     cfg.Timeout,         // Open 10s 后进入 Half-Open
        ReadyToTrip: cfg.ReadyToTrip,     // 连续失败 > 5 次触发熔断
        OnStateChange: func(name string, from, to gobreaker.State) {
            // 记录日志 + Prometheus 指标
            logger.Info("circuit breaker state changed",
                zap.String("name", name),
                zap.String("from", from.String()),
                zap.String("to", to.String()),
            )
        },
    })
}
```

### gRPC 拦截器

```go
func CircuitBreakerInterceptor(cb *gobreaker.CircuitBreaker[any]) grpc.UnaryClientInterceptor {
    return func(
        ctx context.Context,
        method string,
        req, reply any,
        cc *grpc.ClientConn,
        invoker grpc.UnaryInvoker,
        opts ...grpc.CallOption,
    ) error {
        // 通过熔断器执行调用
        _, err := cb.Execute(func() (any, error) {
            // 如果熔断器 Open，这里不会被执行
            innerErr := invoker(ctx, method, req, reply, cc, opts...)
            return nil, innerErr
        })

        if err != nil {
            // 如果是熔断器打开错误，包装为 gRPC Unavailable
            if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
                return status.Error(codes.Unavailable, "circuit breaker is open: "+err.Error())
            }
            return err
        }
        return nil
    }
}
```

**执行流程**：

```
请求进入
  → cb.Execute()
    → cb.state == Closed?
      → Yes: 允许调用 invoker()
        → 调用成功: 重置失败计数
        → 调用失败: 失败计数 + 1
          → ReadyToTrip 返回 true?
            → Yes: 状态 → Open
      → No: cb.state == Open?
        → Yes: 返回 ErrOpenState（不调用 invoker）
      → No: cb.state == Half-Open?
        → Yes: 允许 MaxRequests 个调用通过
          → 成功: 状态 → Closed
          → 失败: 状态 → Open
  ← 返回结果
```

## 配置参数详解

### MaxRequests（Half-Open 并发数）

```go
MaxRequests: 3,  // Half-Open 状态下只允许 3 个请求通过
```

**为什么不是 1？**
- 允许多个请求可以更准确地判断下游是否恢复
- 如果只允许 1 个，可能因为偶然因素误判

### Interval（清零间隔）

```go
Interval: 30 * time.Second,  // 每 30s 清零一次计数器
```

**作用**：防止历史失败影响当前判断
- 如果 5 分钟前有 10 次失败，之后都成功了
- 不清零的话，新来的失败很容易触发熔断

### Timeout（Open 持续时间）

```go
Timeout: 10 * time.Second,  // Open 状态持续 10s 后进入 Half-Open
```

**为什么是 10s？**
- 太短：下游还没恢复就尝试调用，可能再次失败
- 太长：下游已恢复但用户还得等 10s

### ReadyToTrip（触发条件）

```go
ReadyToTrip: func(counts gobreaker.Counts) bool {
    return counts.ConsecutiveFailures > 5  // 连续失败 > 5 次触发
},
```

**触发策略选择**：

| 策略 | 适用场景 | 示例 |
|------|----------|------|
| 连续失败 | 对可用性敏感 | 连续 5 次失败就熔断 |
| 失败率 | 对准确性敏感 | 失败率 > 50% 且请求数 > 20 才熔断 |
| 慢调用率 | 延迟敏感 | 慢调用 > 50% 熔断 |

## 演示场景

### 场景 1：book-service 宕机

```bash
# 1. 停止 book-service
docker stop bookstore-book-service

# 2. 连续创建订单（触发熔断）
for i in {1..10}; do
  curl -X POST http://localhost/api/v1/orders \
    -H "Authorization: Bearer <token>" \
    -H "Content-Type: application/json" \
    -d '{"items":[{"book_id":1,"quantity":1}]}'
done

# 3. 观察日志
# 前 5 次：book-service 连接失败（超时）
# 第 6 次起：circuit breaker state changed: closed → open
# 之后：直接返回 "circuit breaker is open"

# 4. 重启 book-service
docker start bookstore-book-service

# 5. 等待 10s 后再次请求
# 观察：circuit breaker state changed: open → half-open → closed
```

### 场景 2：Prometheus 指标

```bash
# 查询熔断器状态变化
curl http://localhost:9090/api/v1/query?query=circuit_breaker_state_changes_total

# 查询结果示例
{
  "breaker": "book-service",
  "from": "closed",
  "to": "open"
}
```

### 场景 3：Grafana Dashboard

在 Grafana 中查看熔断器面板：
- 状态变化时间线
- 当前状态（Closed/Open/Half-Open）
- 状态变化次数

## 与其他模式的协作

```
请求进入
  → CircuitBreaker (检查是否熔断)
    → Open → 直接返回错误
    → Closed/Half-Open → 继续
      → Retry (失败时重试)
        → 重试次数用完仍失败 → 熔断器记录失败
```

**关键点**：重试时，每次失败都会被熔断器统计。如果 3 次重试都失败，相当于 3 次失败计数。

## 总结

| 概念 | 说明 |
|------|------|
| **Closed** | 正常放行，统计失败率 |
| **Open** | 拒绝所有请求，快速失败 |
| **Half-Open** | 尝试恢复，允许少量请求 |
| **ReadyToTrip** | 触发熔断的条件（连续失败/失败率） |
| **Timeout** | Open 持续时间 |
| **MaxRequests** | Half-Open 允许的并发数 |
