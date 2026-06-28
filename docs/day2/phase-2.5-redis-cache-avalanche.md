# Phase 1.10: Redis 缓存雪崩解决方案

## 1. 问题描述

大量 key **同时过期**，或 Redis 服务宕机，导致请求全部打到数据库。

```
场景一：大量key同时过期
    t0: 1000个key 同时设置TTL=30分钟
    t30: 1000个key 同时过期
    t30: 1000个请求同时打到MySQL → 数据库压力骤增

场景二：Redis宕机
    t0: Redis服务不可用
    t0: 所有请求直接打到MySQL → 数据库崩溃
```

**典型场景**：定时任务批量写入缓存、Redis 重启。

---

## 2. 解决方案

### 2.1 随机过期时间（最简单）

在基础 TTL 上加一个随机值，避免同时过期。

```go
// 生成随机过期时间
func randomTTL(base time.Duration, maxJitter time.Duration) time.Duration {
    jitter := time.Duration(rand.Int63n(int64(maxJitter)))
    return base + jitter
}

// 使用示例
func (r *CachedBookRepository) setToCache(book *model.Book) error {
    key := bookCacheKey(book.ID)
    data, _ := json.Marshal(book)

    // 30分钟 ± 5分钟的随机过期时间
    ttl := randomTTL(30*time.Minute, 5*time.Minute)
    return r.redis.Set(ctx, key, data, ttl).Err()
}
```

### 2.2 多级缓存

本地缓存 + Redis 缓存 + 数据库。

```go
import (
    "sync"
    "time"
)

// LocalCache 本地缓存
type LocalCache struct {
    mu    sync.RWMutex
    items map[string]cacheItem
}

type cacheItem struct {
    value     any
    expireAt time.Time
}

func NewLocalCache() *LocalCache {
    c := &LocalCache{
        items: make(map[string]cacheItem),
    }
    // 启动清理协程
    go c.cleanup()
    return c
}

func (c *LocalCache) Get(key string) (any, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    item, ok := c.items[key]
    if !ok || time.Now().After(item.expireAt) {
        return nil, false
    }
    return item.value, true
}

func (c *LocalCache) Set(key string, value any, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.items[key] = cacheItem{
        value:     value,
        expireAt: time.Now().Add(ttl),
    }
}

func (c *LocalCache) cleanup() {
    ticker := time.NewTicker(time.Minute)
    for range ticker.C {
        c.mu.Lock()
        for key, item := range c.items {
            if time.Now().After(item.expireAt) {
                delete(c.items, key)
            }
        }
        c.mu.Unlock()
    }
}

// 多级缓存查询
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    key := bookCacheKey(id)

    // 1. 先查本地缓存
    if data, ok := r.localCache.Get(key); ok {
        return data.(*model.Book), nil
    }

    // 2. 查Redis缓存
    book, err := r.getFromCache(id)
    if err == nil {
        // 写入本地缓存（本地缓存TTL更短）
        r.localCache.Set(key, book, 5*time.Minute)
        return book, nil
    }

    // 3. 查数据库
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        return nil, err
    }

    // 4. 写入两级缓存
    go r.setToCache(&book)
    r.localCache.Set(key, &book, 5*time.Minute)

    return &book, nil
}
```

### 2.3 熔断降级

```go
import "time"

// CircuitBreaker 熔断器
type CircuitBreaker struct {
    failureCount int
    successCount int
    state        string // "closed", "open", "half-open"
    lastFailure  time.Time
    threshold    int
    timeout      time.Duration
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        state:     "closed",
        threshold: threshold,
        timeout:   timeout,
    }
}

func (cb *CircuitBreaker) Allow() bool {
    switch cb.state {
    case "closed":
        return true
    case "open":
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.state = "half-open"
            return true
        }
        return false
    case "half-open":
        return true
    }
    return false
}

func (cb *CircuitBreaker) Success() {
    cb.successCount++
    if cb.state == "half-open" {
        cb.state = "closed"
        cb.failureCount = 0
    }
}

func (cb *CircuitBreaker) Failure() {
    cb.failureCount++
    cb.lastFailure = time.Now()
    if cb.failureCount >= cb.threshold {
        cb.state = "open"
    }
}

// 带熔断的缓存查询
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    // 1. 检查熔断器
    if !r.breaker.Allow() {
        // 熔断打开，直接查数据库（降级）
        return r.bookRepository.FindByID(id)
    }

    // 2. 尝试查缓存
    book, err := r.getFromCache(id)
    if err == nil {
        r.breaker.Success()
        return book, nil
    }

    // 3. 缓存未命中，查数据库
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        r.breaker.Failure()
        return nil, err
    }

    r.breaker.Success()
    go r.setToCache(&book)
    return &book, nil
}
```

### 2.4 限流降级

```go
import "golang.org/x/time/rate"

// RateLimiter 限流器
type RateLimiter struct {
    limiter *rate.Limiter
}

func NewRateLimiter(rps int) *RateLimiter {
    return &RateLimiter{
        limiter: rate.NewLimiter(rate.Limit(rps), rps),
    }
}

func (rl *RateLimiter) Allow() bool {
    return rl.limiter.Allow()
}

// 带限流的查询
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    // 1. 限流检查
    if !r.limiter.Allow() {
        return nil, ErrRateLimitExceeded
    }

    // 2. 查缓存
    book, err := r.getFromCache(id)
    if err == nil {
        return book, nil
    }

    // 3. 查数据库
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        return nil, err
    }

    go r.setToCache(&book)
    return &book, nil
}
```

---

## 3. 方案对比

| 方案 | 实现复杂度 | 效果 | 适用场景 |
|------|-----------|------|---------|
| 随机过期时间 | 简单 | 防止同时过期 | 所有场景 |
| 多级缓存 | 中等 | 减少Redis压力 | 高并发场景 |
| 熔断降级 | 中等 | 保护数据库 | 分布式系统 |
| 限流降级 | 简单 | 控制请求量 | 所有场景 |

---

## 4. 选择建议

```
┌─────────────────┐
│ 缓存雪崩问题？  │
└────────┬────────┘
         │
    ┌────┴────┐
    │高并发？  │
    └────┬────┘
         │
    ┌────┴────┐
    │ 是      │ 否
    │         │
┌───┴───┐ ┌───┴───┐
│ 多级  │ │ 随机  │
│ 缓存  │ │ TTL   │
└───────┘ └───────┘
```

## 5. 监控与告警

```go
// 监控指标
type CacheMetrics struct {
    HitCount   int64 // 缓存命中次数
    MissCount  int64 // 缓存未命中次数
}

// 计算命中率
func (m *CacheMetrics) HitRate() float64 {
    total := m.HitCount + m.MissCount
    if total == 0 {
        return 0
    }
    return float64(m.HitCount) / float64(total)
}

// 监控建议
// 1. 监控缓存命中率（低于80%告警）
// 2. 监控数据库QPS（异常升高告警）
// 3. 监控Redis内存使用（防止内存溢出）
// 4. 监控慢查询日志
```

## 6. 参考资源

- [Redis 高可用方案](https://redis.io/docs/manual/replication/)
- [熔断器模式](https://learn.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker)
- [限流算法详解](https://www.infoq.cn/article/distributed-cache-design)
