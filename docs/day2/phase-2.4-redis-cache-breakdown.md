# Phase 2.4: Redis 缓存击穿解决方案

## 1. 问题描述

某个**热点 key** 突然失效，大量并发请求同时打到数据库。

```
时间线:
    t1: 热点key "book:1" 过期
    t2: 请求1 → 缓存未命中 → 查DB → 需要100ms
    t2: 请求2 → 缓存未命中 → 查DB → 需要100ms
    t2: 请求3 → 缓存未命中 → 查DB → 需要100ms
    ...
    t2: 请求N → 缓存未命中 → 查DB → 需要100ms
    t3: 请求1 返回 → 写入缓存
    t4: 请求2~N 返回 → 又写入缓存（重复写入）
```

**典型场景**：明星微博、热点商品、秒杀商品。

---

## 2. 解决方案

### 2.1 互斥锁（Mutex Lock）

只允许一个线程去查数据库，其他线程等待。

```go
import "sync"

func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    key := bookCacheKey(id)

    // 1. 先查缓存
    book, err := r.getFromCache(id)
    if err == nil {
        return book, nil
    }

    // 2. 缓存未命中，加锁查数据库
    lockKey := fmt.Sprintf("lock:%s", key)
    lock := r.acquireLock(lockKey, 5*time.Second)
    if !lock {
        // 获取锁失败，等待后重试缓存
        time.Sleep(100 * time.Millisecond)
        return r.getFromCache(id)
    }
    defer r.releaseLock(lockKey)

    // 3. 双重检查（可能其他线程已经写入缓存）
    book, err = r.getFromCache(id)
    if err == nil {
        return book, nil
    }

    // 4. 查数据库
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        return nil, err
    }

    // 5. 写入缓存
    r.setToCache(&book)
    return &book, nil
}

// acquireLock 获取分布式锁
func (r *CachedBookRepository) acquireLock(key string, ttl time.Duration) bool {
    result, err := r.redis.SetNX(ctx, key, "1", ttl).Result()
    return err == nil && result
}

// releaseLock 释放分布式锁
func (r *CachedBookRepository) releaseLock(key string) {
    r.redis.Del(ctx, key)
}
```

### 2.2 逻辑过期（Logical Expiration）

缓存数据时设置一个逻辑过期时间，不真正删除 key。

```go
type CacheItem struct {
    Data       any `json:"data"`
    ExpireTime time.Time   `json:"expire_time"` // 逻辑过期时间
}

// 带逻辑过期的缓存设置
func (r *CachedBookRepository) setToCacheWithLogicalExpire(book *model.Book) error {
    key := bookCacheKey(book.ID)
    item := CacheItem{
        Data:       book,
        ExpireTime: time.Now().Add(bookCacheTTL),
    }
    data, _ := json.Marshal(item)
    // 设置永不过期（或超长过期）
    return r.redis.Set(ctx, key, data, 0).Err()
}

// 带逻辑过期的查询
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    key := bookCacheKey(id)

    // 1. 查询缓存
    data, err := r.redis.Get(ctx, key).Result()
    if err != nil {
        return r.loadFromDB(id) // 缓存不存在，从DB加载
    }

    // 2. 解析缓存
    var item CacheItem
    json.Unmarshal([]byte(data), &item)

    // 3. 判断逻辑过期
    if time.Now().Before(item.ExpireTime) {
        // 未过期，直接返回
        return item.Data.(*model.Book), nil
    }

    // 4. 已过期，异步刷新缓存
    go func() {
        r.loadFromDB(id) // 后台线程刷新
    }()

    // 5. 返回旧数据（保证可用性）
    return item.Data.(*model.Book), nil
}

// loadFromDB 从数据库加载并缓存
func (r *CachedBookRepository) loadFromDB(id uint) (*model.Book, error) {
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        return nil, err
    }
    r.setToCacheWithLogicalExpire(&book)
    return &book, nil
}
```

### 2.3 热点 key 永不过期 + 异步刷新

```go
// 热点key使用单独的缓存策略
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    key := bookCacheKey(id)
    isHotKey := r.isHotKey(id)

    // 1. 查缓存
    book, err := r.getFromCache(id)
    if err == nil {
        if isHotKey {
            r.refreshIfStale(id, book) // 异步检查是否需要刷新
        }
        return book, nil
    }

    // 2. 查数据库
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        return nil, err
    }

    // 3. 热点key设置永不过期
    if isHotKey {
        r.setToCacheNoExpire(&book)
    } else {
        go r.setToCache(&book)
    }

    return &book, nil
}

// isHotKey 判断是否是热点key
func (r *CachedBookRepository) isHotKey(id uint) bool {
    // 实际项目中可以从访问日志、计数器等判断
    // 这里简化为：ID < 100 的是热点
    return id < 100
}

// refreshIfStale 异步刷新过期数据
func (r *CachedBookRepository) refreshIfStale(id uint, book *model.Book) {
    // 检查是否需要刷新（比如缓存时间超过阈值）
    // 这里简化处理
}
```

---

## 3. 方案对比

| 方案 | 一致性 | 可用性 | 实现复杂度 | 适用场景 |
|------|--------|--------|-----------|---------|
| 互斥锁 | 强 | 低（等待） | 中等 | 数据一致性要求高 |
| 逻辑过期 | 最终一致 | 高（返回旧数据） | 中等 | 可接受短暂不一致 |
| 热点key永不过期 | 最终一致 | 高 | 简单 | 热点数据明确 |

---

## 4. 选择建议

```
┌─────────────────┐
│ 缓存击穿问题？  │
└────────┬────────┘
         │
    ┌────┴────┐
    │需要强一致？│
    └────┬────┘
         │
    ┌────┴────┐
    │ 是      │ 否
    │         │
┌───┴───┐ ┌───┴───┐
│ 互斥  │ │ 逻辑  │
│ 锁    │ │ 过期  │
└───────┘ └───────┘
```

## 5. 参考资源

- [分布式锁实现方案](https://redis.io/docs/manual/patterns/distributed-locks/)
- [缓存击穿解决方案](https://www.cnblogs.com/dongl96/p/13689722.html)
