# Phase 2.2: 缓存模式

## 1. 为什么需要缓存模式？

数据库和缓存之间需要一套约定的读写策略，确保数据一致性。

```
应用 → 缓存 → 数据库
```

## 2. 三种核心模式

### 2.1 Cache-Aside（旁路缓存）

**最常用**，应用自己管理缓存和数据库的读写。

```
读流程：
应用 → 查缓存 → 命中？ → 返回
                未命中 → 查数据库 → 写入缓存 → 返回

写流程：
应用 → 更新/删除数据库 → 删除缓存
```

#### 代码示例

```go
// 读取
func GetUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, id uint) (*User, error) {
    key := fmt.Sprintf("user:%d", id)

    // 1. 先查缓存
    cached, err := rdb.Get(ctx, key).Result()
    if err == nil {
        var user User
        json.Unmarshal([]byte(cached), &user)
        return &user, nil
    }

    // 2. 缓存未命中，查数据库
    var user User
    if err := db.First(&user, id).Error; err != nil {
        return nil, err
    }

    // 3. 写入缓存
    data, _ := json.Marshal(user)
    rdb.Set(ctx, key, data, 30*time.Minute)

    return &user, nil
}

// 更新
func UpdateUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, user *User) error {
    // 1. 更新数据库
    if err := db.Save(user).Error; err != nil {
        return err
    }

    // 2. 删除缓存（不是更新！）
    key := fmt.Sprintf("user:%d", user.ID)
    rdb.Del(ctx, key)

    return nil
}
```

#### 特点

| 优点 | 缺点 |
|------|------|
| 实现简单 | 首次请求会 miss 缓存 |
| 缓存只存热点数据 | 写操作后缓存可能短暂不一致 |
| 高性能 | 需要处理缓存失效逻辑 |

#### 适用场景

- 读多写少的场景
- 大部分 Web 应用

---

### 2.2 Write-Through（写穿透）

写操作时，**同步**更新缓存和数据库，缓存是主要数据源。

```
读流程：
应用 → 查缓存 → 命中？ → 返回
                未命中 → 查数据库 → 写入缓存 → 返回

写流程：
应用 → 写入缓存 → 缓存同步写入数据库 → 返回
```

#### 代码示例

```go
// 读取（同 Cache-Aside）
func GetUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, id uint) (*User, error) {
    key := fmt.Sprintf("user:%d", id)

    cached, err := rdb.Get(ctx, key).Result()
    if err == nil {
        var user User
        json.Unmarshal([]byte(cached), &user)
        return &user, nil
    }

    var user User
    if err := db.First(&user, id).Error; err != nil {
        return nil, err
    }

    data, _ := json.Marshal(user)
    rdb.Set(ctx, key, data, 30*time.Minute)

    return &user, nil
}

// 更新
func UpdateUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, user *User) error {
    // 1. 先写缓存
    key := fmt.Sprintf("user:%d", user.ID)
    data, _ := json.Marshal(user)
    if err := rdb.Set(ctx, key, data, 30*time.Minute).Err(); err != nil {
        return err
    }

    // 2. 同步写数据库
    if err := db.Save(user).Error; err != nil {
        // 写数据库失败，回滚缓存
        rdb.Del(ctx, key)
        return err
    }

    return nil
}
```

#### 特点

| 优点 | 缺点 |
|------|------|
| 数据一致性好 | 写操作延迟高（要等数据库） |
| 缓存始终有最新数据 | 数据库写失败时需要回滚缓存 |
| 读性能高 | 写入热点数据时缓存命中率高 |

#### 适用场景

- 写入后立即需要读取
- 对数据一致性要求高

---

### 2.3 Write-Behind（写回）

写操作时，**异步**更新数据库，缓存是主要数据源。

```
读流程：
应用 → 查缓存 → 命中？ → 返回
                未命中 → 查数据库 → 写入缓存 → 返回

写流程：
应用 → 写入缓存 → 返回
              ↓（异步）
        后台任务写入数据库
```

#### 代码示例

```go
// 写入缓存并异步更新数据库
func UpdateUser(ctx context.Context, rdb *redis.Client, db *gorm.DB, user *User) error {
    // 1. 写入缓存
    key := fmt.Sprintf("user:%d", user.ID)
    data, _ := json.Marshal(user)
    if err := rdb.Set(ctx, key, data, 30*time.Minute).Err(); err != nil {
        return err
    }

    // 2. 异步写入数据库（使用 goroutine 或消息队列）
    go func() {
        time.Sleep(100 * time.Millisecond) // 延迟写入
        if err := db.Save(user).Error; err != nil {
            // 记录日志，后续重试
            log.Printf("failed to sync user %d to database: %v", user.ID, err)
        }
    }()

    return nil
}
```

#### 使用消息队列（更可靠）

```go
func UpdateUser(ctx context.Context, rdb *redis.Client, mq *MessageQueue, user *User) error {
    // 1. 写入缓存
    key := fmt.Sprintf("user:%d", user.ID)
    data, _ := json.Marshal(user)
    rdb.Set(ctx, key, data, 30*time.Minute)

    // 2. 发送到消息队列（由消费者异步写入数据库）
    msg, _ := json.Marshal(user)
    return mq.Publish("user:update", msg)
}

// 消费者
func consumeUserUpdates(mq *MessageQueue, db *gorm.DB) {
    for msg := range mq.Subscribe("user:update") {
        var user User
        json.Unmarshal(msg.Body, &user)
        db.Save(&user)
    }
}
```

#### 特点

| 优点 | 缺点 |
|------|------|
| 写性能极高（只写缓存） | 数据可能丢失（缓存故障时） |
| 可合并多次写操作 | 数据库最终一致，有延迟 |
| 适合高写入场景 | 实现复杂，需要处理失败重试 |

#### 适用场景

- 写操作非常频繁
- 可接受短暂的数据不一致
- 日志采集、计数器等

---

## 3. 对比总结

| 模式 | 写流程 | 数据一致性 | 写性能 | 实现复杂度 | 适用场景 |
|------|--------|-----------|--------|-----------|---------|
| **Cache-Aside** | 更新DB → 删缓存 | 最终一致 | 中等 | 简单 | 大部分场景 |
| **Write-Through** | 写缓存 → 同步写DB | 强一致 | 较低 | 中等 | 一致性要求高 |
| **Write-Behind** | 写缓存 → 异步写DB | 最终一致 | 极高 | 复杂 | 高写入场景 |

## 4. 选择建议

```
                ┌─────────────────┐
                │  读多写少？      │
                └────────┬────────┘
                         │
           ┌─────────────┴─────────────┐
           │                           │
         是                          否
           │                           │
    ┌──────┴──────┐            ┌───────┴───────┐
    │ Cache-Aside │            │ 写入量大？    │
    └─────────────┘            └───────┬───────┘
                                       │
                         ┌─────────────┴─────────────┐
                         │                           │
                       是                          否
                         │                           │
              ┌──────────┴──────────┐     ┌──────────┴──────────┐
              │  Write-Behind       │     │  Write-Through      │
              │  (可接受短暂不一致)  │     │  (需要强一致性)      │
              └─────────────────────┘     └─────────────────────┘
```

## 5. 实际应用

本项目使用的是 **Cache-Aside** 模式：

```go
// 读取：先查缓存，未命中查数据库，然后写入缓存
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    book, err := r.getFromCache(id)
    if err == nil {
        return book, nil
    }

    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        return nil, err
    }

    go r.setToCache(book)  // 异步写入缓存
    return book, nil
}

// 更新：更新数据库后删除缓存
func (r *CachedBookRepository) Update(book *model.Book) error {
    if err := r.bookRepository.Update(book); err != nil {
        return err
    }
    r.deleteFromCache(book.ID)  // 删除缓存
    return nil
}
```

## 6. 参考资源

- [Redis 缓存模式详解](https://redis.io/docs/manual/patterns/)
- [Cache-Aside Pattern](https://docs.microsoft.com/en-us/azure/architecture/patterns/cache-aside)
- [Martin Kleppmann - Designing Data-Intensive Applications](https://dataintensive.net/)
