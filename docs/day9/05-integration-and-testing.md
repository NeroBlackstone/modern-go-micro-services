# Day 9 - 集成指南与故障注入验证

## 拦截器链集成

### 拦截器执行顺序

```
请求进入
  → Tracing (创建 span，注入 trace context)
    → CircuitBreaker (检查熔断状态)
      → Retry (失败时重试)
        → RateLimiter (检查是否超限)
          → 实际 RPC 调用
        ← RateLimiter 结果
      ← Retry 结果
    ← CircuitBreaker 结果
  ← Tracing 结果
返回响应
```

**为什么这个顺序？**
1. **Tracing 最外层**：无论成功失败，都需要记录 trace
2. **CircuitBreaker 次之**：如果熔断器 Open，直接失败，不需要重试
3. **Retry 内层**：重试时每个 attempt 都会被 RateLimiter 检查
4. **RateLimiter 最内层**：每次调用（包括重试）都要限流

### 代码集成

```go
// cmd/order-service/main.go

// 1. 创建熔断器
bookCB := resilience.NewCircuitBreaker(
    resilience.CircuitBreakerConfig{
        Name:        "book-service",
        MaxRequests: 3,
        Interval:    30 * time.Second,
        Timeout:     10 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 5
        },
    },
    logger,
)

// 2. 创建重试配置
retryCfg := resilience.DefaultRetryConfig(logger)

// 3. 创建限流器
bookLimiter := resilience.NewGRPCRateLimiter(resilience.RateLimiterConfig{
    Rate:  100,
    Burst: 50,
})

// 4. 创建 gRPC 连接（带拦截器链）
bookConn, err := grpc.NewClient(
    "consul:///book-service",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
    grpc.WithChainUnaryInterceptor(
        tracing.UnaryClientInterceptor(),
        resilience.CircuitBreakerInterceptor(bookCB),
        resilience.RetryInterceptor(retryCfg),
        resilience.RateLimiterInterceptor(bookLimiter),
    ),
)
```

## Prometheus 指标

### 新增指标

```go
// internal/metrics/metrics.go

// 容错指标
CircuitBreakerStateChanges  // circuit_breaker_state_changes_total
RetryAttempts               // retry_attempts_total
RateLimitRejections         // rate_limit_rejected_total
```

### 记录函数

```go
func RecordCircuitBreakerStateChange(ctx context.Context, name, from, to string) {
    CircuitBreakerStateChanges.Add(ctx, 1, otelmetric.WithAttributes(
        attribute.String("breaker", name),
        attribute.String("from", from),
        attribute.String("to", to),
    ))
}

func RecordRetryAttempt(ctx context.Context, method string, attempt int) {
    RetryAttempts.Add(ctx, 1, otelmetric.WithAttributes(
        attribute.String("method", method),
        attribute.Int("attempt", attempt),
    ))
}

func RecordRateLimitRejection(ctx context.Context, layer, method string) {
    RateLimitRejections.Add(ctx, 1, otelmetric.WithAttributes(
        attribute.String("layer", layer),
        attribute.String("method", method),
    ))
}
```

### PromQL 查询

```promql
# 熔断器状态变化
rate(circuit_breaker_state_changes_total[5m])

# 重试次数
rate(retry_attempts_total[5m])

# 限流拒绝
rate(rate_limit_rejected_total[5m])

# 按服务分组的重试次数
sum(rate(retry_attempts_total[5m])) by (method)
```

## Grafana Dashboard

### Dashboard JSON

创建 `configs/grafana/provisioning/dashboards/fault-tolerance.json`，包含：

1. **熔断器状态面板**
   - 状态变化时间线
   - 当前状态（Closed/Open/Half-Open）

2. **重试监控面板**
   - 重试次数趋势
   - 按服务分组的重试分布

3. **限流监控面板**
   - 被限流的请求数
   - 按层级（gRPC/HTTP）分组

4. **延迟面板**
   - P50/P95/P99 延迟
   - 重试对延迟的影响

## 故障注入测试

### 测试 1：熔断器演示

**目标**：验证熔断器在 book-service 宕机时自动打开

```bash
# 步骤 1：正常创建订单
curl -X POST http://localhost/api/v1/orders \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"items":[{"book_id":1,"quantity":1}]}'
# 预期：成功

# 步骤 2：停止 book-service
docker stop bookstore-book-service

# 步骤 3：连续创建订单（触发熔断）
for i in {1..10}; do
  curl -X POST http://localhost/api/v1/orders \
    -H "Authorization: Bearer <token>" \
    -H "Content-Type: application/json" \
    -d '{"items":[{"book_id":1,"quantity":1}]}'
done

# 步骤 4：观察日志
# 前 5 次：book-service 连接失败（超时）
# 第 6 次起：circuit breaker state changed: closed → open
# 之后：直接返回 "circuit breaker is open"

# 步骤 5：重启 book-service
docker start bookstore-book-service

# 步骤 6：等待 10s 后再次请求
# 观察：circuit breaker state changed: open → half-open → closed
```

### 测试 2：重试演示

**目标**：验证重试在瞬时故障时自动恢复

```bash
# 步骤 1：模拟瞬时故障（用 toxiproxy）
# book-service 偶尔返回 Unavailable

# 步骤 2：创建订单
curl -X POST http://localhost/api/v1/orders ...

# 步骤 3：观察日志
# 第 1 次调用：failed (Unavailable)
# 第 2 次调用（100ms 后）：failed (Unavailable)
# 第 3 次调用（200ms 后）：success!

# 步骤 4：查看 trace
# 看到 3 个 span，每个间隔递增
```

### 测试 3：限流演示

**目标**：验证限流在高并发时生效

```bash
# 步骤 1：设置低限流（每秒 5 请求）
# 修改代码或配置

# 步骤 2：快速发送 10 个请求
for i in {1..10}; do
  curl -X POST http://localhost/api/v1/orders \
    -H "Authorization: Bearer <token>" \
    -H "Content-Type: application/json" \
    -d '{"items":[{"book_id":1,"quantity":1}]}' &
done

# 步骤 3：观察结果
# 前 2 个请求：通过（突发）
# 第 3-7 个请求：通过（正常速率）
# 第 8-10 个请求：429 Too Many Requests

# 步骤 4：查看 Prometheus 指标
curl http://localhost:9090/api/v1/query?query=rate_limit_rejected_total
```

### 测试 4：Traefik 限流演示

**目标**：验证 Traefik 网关层限流

```bash
# 步骤 1：使用 wrk 发送高并发
wrk -t4 -c100 -d10s http://localhost/api/v1/orders

# 步骤 2：观察 Traefik 日志
# 部分请求返回 429

# 步骤 3：查看 Prometheus 指标
curl http://localhost:9090/api/v1/query?query=rate_limit_rejected_total
```

### 测试 5：级联故障防护

**目标**：验证容错机制防止级联故障

```bash
# 步骤 1：停止 book-service
docker stop bookstore-book-service

# 步骤 2：持续发送订单请求
while true; do
  curl -X POST http://localhost/api/v1/orders \
    -H "Authorization: Bearer <token>" \
    -H "Content-Type: application/json" \
    -d '{"items":[{"book_id":1,"quantity":1}]}'
  sleep 0.1
done

# 步骤 3：观察 order-service
# 熔断器打开后，请求快速失败（0ms）
# order-service 线程不会被阻塞
# 系统保持响应能力

# 步骤 4：重启 book-service
docker start bookstore-book-service

# 步骤 5：等待 10s 后
# 熔断器 Half-Open → Closed
# 订单创建恢复正常
```

## 监控告警

### Prometheus 告警规则

```yaml
# configs/alert-rules.yml

groups:
  - name: resilience
    rules:
      # 熔断器打开告警
      - alert: CircuitBreakerOpen
        expr: circuit_breaker_state_changes_total{to="open"} > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Circuit breaker {{ $labels.breaker }} is open"
          description: "Circuit breaker {{ $labels.breaker }} has been open for more than 1 minute"

      # 重试率过高告警
      - alert: HighRetryRate
        expr: rate(retry_attempts_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High retry rate for {{ $labels.method }}"
          description: "Retry rate is {{ $value }} per second"

      # 限流拒绝过多告警
      - alert: HighRateLimitRejections
        expr: rate(rate_limit_rejected_total[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High rate limit rejections"
          description: "Rate limit rejection rate is {{ $value }} per second"
```

## 总结

### Day 9 实现的内容

| 层级 | 模式 | 实现 |
|------|------|------|
| **gRPC Client** | 熔断器 | `sony/gobreaker` + 拦截器 |
| **gRPC Client** | 重试 | 指数退避 + 拦截器 |
| **gRPC Client** | 限流 | 令牌桶 + 拦截器 |
| **HTTP 网关** | 限流 | Traefik `rate-limit` 中间件 |
| **HTTP 网关** | 熔断 | Traefik `circuit-breaker` 中间件 |
| **HTTP 网关** | 重试 | Traefik `retry` 中间件 |
| **监控** | 指标 | Prometheus + Grafana |

### 关键文件

| 文件 | 说明 |
|------|------|
| `internal/resilience/circuit_breaker.go` | 熔断器实现 |
| `internal/resilience/retry.go` | 重试实现 |
| `internal/resilience/rate_limiter.go` | 限流实现 |
| `internal/middleware/rate_limiter.go` | HTTP 限流中间件 |
| `internal/metrics/metrics.go` | 容错指标 |
| `cmd/order-service/main.go` | 拦截器链集成 |
| `compose.yml` | Traefik 中间件配置 |

### 验证清单

- [ ] 熔断器在 book-service 宕机时自动打开
- [ ] 熔断器在 book-service 恢复后自动关闭
- [ ] 重试在瞬时故障时自动恢复
- [ ] 重试尊重 context 取消
- [ ] 限流在高并发时生效
- [ ] Traefik 网关层限流正常工作
- [ ] Prometheus 指标正确记录
- [ ] Grafana Dashboard 正常显示
