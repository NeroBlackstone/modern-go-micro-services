# Phase 2.3: Redis 缓存穿透解决方案

## 1. 问题描述

查询**不存在的数据**，缓存永远不命中，每次都打到数据库。

```
请求: GET /book/999999  （不存在的ID）
    ↓
Redis: 缓存未命中
    ↓
MySQL: 查询为空
    ↓
结果: 不缓存（或缓存空值）
    ↓
下次请求: GET /book/999999
    ↓
Redis: 还是未命中 → 又打到 MySQL
```

**典型场景**：恶意攻击、爬虫遍历、接口参数校验不严。

---

## 2. 解决方案

### 2.1 缓存空值（最简单）

查询结果为空时，缓存一个空值（带较短 TTL）。

```go
const (
    emptyCacheTTL = 5 * time.Minute // 空值缓存时间较短
)

func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    key := bookCacheKey(id)

    // 1. 先查缓存
    data, err := r.redis.Get(ctx, key).Result()
    if err == nil {
        // 判断是否是空值标记
        if data == "" {
            return nil, ErrBookNotFound
        }
        var book model.Book
        json.Unmarshal([]byte(data), &book)
        return &book, nil
    }

    // 2. 查数据库
    var book model.Book
    if err := r.bookRepository.db.First(&book, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            // 3. 缓存空值，防止穿透
            r.redis.Set(ctx, key, "", emptyCacheTTL)
            return nil, ErrBookNotFound
        }
        return nil, err
    }

    // 4. 缓存正常数据
    go r.setToCache(book)
    return &book, nil
}
```

### 2.2 布隆过滤器（Bloom Filter）

在缓存前加一层布隆过滤器，快速判断数据是否存在。

```go
// 布隆过滤器命令直接在 go-redis/v9 主包中，无需额外 import
// 前提：Redis 服务器需要安装 RedisBloom 模块

// 初始化布隆过滤器
func InitBloomFilter(ctx context.Context, rdb *redis.Client) {
    if err := rdb.BFReserve(ctx, "book_filter", 0.001, 10000).Err(); err != nil {
        // BFReserve 仅在 key 不存在时创建，已存在则忽略
        if err != redis.Nil {
            log.Printf("BFReserve error: %v", err)
        }
    }
}

// 添加元素到布隆过滤器
func AddToBloomFilter(ctx context.Context, rdb *redis.Client, id uint) {
    key := bookCacheKey(id)
    rdb.BFAdd(ctx, "book_filter", key)
}

// 检查元素是否可能存在
func MightExist(ctx context.Context, rdb *redis.Client, id uint) bool {
    key := bookCacheKey(id)
    exists, _ := rdb.BFExists(ctx, "book_filter", key).Result()
    return exists
}

// 带布隆过滤器的查询
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
    ctx := context.Background()

    // 1. 布隆过滤器判断
    if !MightExist(ctx, r.redis, id) {
        return nil, ErrBookNotFound // 一定不存在，直接返回
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

    // 4. 写入缓存和布隆过滤器
    go r.setToCache(&book)
    go AddToBloomFilter(ctx, r.redis, book.ID)

    return &book, nil
}
```

### 2.3 参数校验（最基础）

在接口层进行严格校验。

```go
func GetBookHandler(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil || id <= 0 {
        c.JSON(400, gin.H{"error": "invalid id"})
        return
    }
    // 继续处理...
}
```

---

## 3. 方案对比

| 方案 | 实现复杂度 | 内存占用 | 适用场景 |
|------|-----------|---------|---------|
| 缓存空值 | 简单 | 低 | 数据量小、请求量不大 |
| 布隆过滤器 | 中等 | 中 | 数据量大、需要高性能 |
| 参数校验 | 简单 | 无 | 所有接口必备 |

---

## 4. 选择建议

```
┌─────────────────┐
│ 缓存穿透问题？  │
└────────┬────────┘
         │
    ┌────┴────┐
    │ 数据量大？│
    └────┬────┘
         │
    ┌────┴────┐
    │ 是      │ 否
    │         │
┌───┴───┐ ┌───┴───┐
│ 布隆  │ │ 缓存  │
│ 过滤器│ │ 空值  │
└───────┘ └───────┘
```

## 5. 参考资源

- [布隆过滤器详解](https://en.wikipedia.org/wiki/Bloom_filter)
- [Redis 缓存穿透解决方案](https://www.cnblogs.com/dongl96/p/13689722.html)
