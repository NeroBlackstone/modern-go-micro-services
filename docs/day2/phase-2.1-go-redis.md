# Phase 2.1: Go-Redis 客户端使用

## 1. 安装

```bash
go get github.com/redis/go-redis/v9
```

## 2. 连接 Redis

```go
package main

import (
    "context"
    "fmt"
    "github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func main() {
    rdb := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "",  // 空表示无密码
        DB:       0,
    })

    // 测试连接
    if err := rdb.Ping(ctx).Err(); err != nil {
        panic(err)
    }
    fmt.Println("Connected to Redis!")
}
```

## 3. String 操作

最常用的数据类型，可存储字符串、数字、JSON 等。

```go
// 设置值（带过期时间）
rdb.Set(ctx, "name", "John", 10*time.Minute)
rdb.Set(ctx, "counter", 0, 0)  // 永不过期

// 获取值
val, err := rdb.Get(ctx, "name").Result()

// 设置/获取多个值
rdb.MSet(ctx, "key1", "value1", "key2", "value2")
vals, _ := rdb.MGet(ctx, "key1", "key2").Result()

// 递增/递减
rdb.Incr(ctx, "counter")       // +1
rdb.IncrBy(ctx, "counter", 10) // +10
rdb.Decr(ctx, "counter")       // -1

// 检查存在 / 删除
exists, _ := rdb.Exists(ctx, "name").Result()  // 1 或 0
rdb.Del(ctx, "name")

// 设置过期时间
rdb.Expire(ctx, "name", 30*time.Minute)
```

## 4. Hash 操作

适合存储对象，类似于 Go 的 map。

```go
// 设置字段
rdb.HSet(ctx, "user:1", "name", "John", "age", "30", "email", "john@example.com")

// 获取单个字段
name, _ := rdb.HGet(ctx, "user:1", "name").Result()

// 获取所有字段（返回 map[string]string）
user, _ := rdb.HGetAll(ctx, "user:1").Result()

// 获取多个字段
vals, _ := rdb.HMGet(ctx, "user:1", "name", "email").Result()

// 删除字段
rdb.HDel(ctx, "user:1", "email")

// 检查字段存在
exists, _ := rdb.HExists(ctx, "user:1", "name").Result()

// 字段递增
rdb.HIncrBy(ctx, "user:1", "age", 1)
```

## 5. List 操作

双向链表，支持从两端操作。

```go
// 插入
rdb.LPush(ctx, "queue", "task1", "task2")  // 左边
rdb.RPush(ctx, "queue", "task3")           // 右边

// 弹出
val, _ := rdb.LPop(ctx, "queue").Result()  // 左边
val, _ := rdb.RPop(ctx, "queue").Result()  // 右边

// 阻塞弹出（等待新元素，超时返回空）
val, _ := rdb.BLPop(ctx, 5*time.Second, "queue").Result()
val, _ := rdb.BRPop(ctx, 5*time.Second, "queue").Result()

// 获取长度
length, _ := rdb.LLen(ctx, "queue").Result()

// 获取范围（0 到 -1 表示所有）
values, _ := rdb.LRange(ctx, "queue", 0, -1).Result()

// 修剪列表（只保留前 10 个）
rdb.LTrim(ctx, "queue", 0, 9)
```

## 6. Set 操作

无序、不重复的元素集合。

```go
// 添加元素
rdb.SAdd(ctx, "tags", "go", "redis", "cache")

// 获取所有元素
members, _ := rdb.SMembers(ctx, "tags").Result()

// 检查元素存在
exists, _ := rdb.SIsMember(ctx, "tags", "go").Result()

// 获取数量
count, _ := rdb.SCard(ctx, "tags").Result()

// 删除元素
rdb.SRem(ctx, "tags", "cache")

// 随机获取/弹出
val, _ := rdb.SRandMember(ctx, "tags").Result()
val, _ := rdb.SPop(ctx, "tags").Result()

// 集合运算
rdb.SAdd(ctx, "set1", "a", "b", "c")
rdb.SAdd(ctx, "set2", "b", "c", "d")

intersection, _ := rdb.SInter(ctx, "set1", "set2").Result()  // 交集 ["b", "c"]
union, _ := rdb.SUnion(ctx, "set1", "set2").Result()          // 并集 ["a", "b", "c", "d"]
diff, _ := rdb.SDiff(ctx, "set1", "set2").Result()            // 差集 ["a"]
```

## 7. Sorted Set 操作

有序元素集合，每个元素有一个分数（score）。

```go
// 添加元素（带分数）
rdb.ZAdd(ctx, "leaderboard",
    redis.Z{Score: 100, Member: "player1"},
    redis.Z{Score: 200, Member: "player2"},
    redis.Z{Score: 150, Member: "player3"},
)

// 获取排名（分数从低到高）
rank, _ := rdb.ZRank(ctx, "leaderboard", "player1").Result()

// 获取排名（分数从高到低）
revRank, _ := rdb.ZRevRank(ctx, "leaderboard", "player1").Result()

// 获取分数
score, _ := rdb.ZScore(ctx, "leaderboard", "player1").Result()

// 获取所有元素（按分数从高到低）
values, _ := rdb.ZRevRangeWithScores(ctx, "leaderboard", 0, -1).Result()

// 获取前 3 名
top3, _ := rdb.ZRevRangeWithScores(ctx, "leaderboard", 0, 2).Result()

// 递增分数
rdb.ZIncrBy(ctx, "leaderboard", 10, "player1")

// 获取数量
count, _ := rdb.ZCard(ctx, "leaderboard").Result()

// 获取分数范围内的元素
values, _ = rdb.ZRangeByScore(ctx, "leaderboard", &redis.ZRangeBy{
    Min: "100",
    Max: "200",
}).Result()

// 删除元素
rdb.ZRem(ctx, "leaderboard", "player1")
```

## 8. 管道（Pipeline）

批量执行多个命令，减少网络往返次数。

```go
// 使用管道
cmds, _ := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
    pipe.Set(ctx, "key1", "value1", 0)
    pipe.Set(ctx, "key2", "value2", 0)
    pipe.Set(ctx, "key3", "value3", 0)
    return nil
})

// 批量查询
pipe := rdb.Pipeline()
cmds := make(map[uint]*redis.StringCmd)
for _, id := range ids {
    cmds[id] = pipe.Get(ctx, fmt.Sprintf("book:%d", id))
}
pipe.Exec(ctx)

// 处理结果
for id, cmd := range cmds {
    val, err := cmd.Result()
    if err == nil {
        // 处理 val
    }
}
```

## 9. 事务

保证多个命令原子执行。

```go
// 使用 TxPipeline
txPipe := rdb.TxPipeline()
txPipe.Set(ctx, "key1", "value1", 0)
txPipe.Set(ctx, "key2", "value2", 0)
cmds, err := txPipe.Exec(ctx)

// 使用 WATCH 实现乐观锁
err := rdb.Watch(ctx, func(tx *redis.Tx) error {
    val, _ := tx.Get(ctx, "counter").Result()
    _, err := tx.Pipelined(ctx, func(pipe redis.Pipeliner) error {
        pipe.Set(ctx, "counter", val, 0)
        return nil
    })
    return err
}, "counter")
```

## 10. 发布订阅

```go
// 订阅频道
sub := rdb.Subscribe(ctx, "channel1")
ch := sub.Channel()

// 接收消息
go func() {
    for msg := range ch {
        fmt.Println("Received:", msg.Payload)
    }
}()

// 发布消息
rdb.Publish(ctx, "channel1", "hello")

// 关闭订阅
sub.Close()
```

## 11. 错误处理

```go
val, err := rdb.Get(ctx, "key").Result()
if err != nil {
    switch {
    case errors.Is(err, redis.Nil):
        // key 不存在
    case errors.Is(err, context.DeadlineExceeded):
        // 超时
    case errors.Is(err, context.Canceled):
        // 被取消
    default:
        // 其他错误
    }
}
```

## 12. 参考资源

- [Go-Redis 官方文档](https://redis.uptrace.dev/guide/)
- [Go-Redis GitHub](https://github.com/redis/go-redis)
- [Redis 命令参考](https://redis.io/commands/)
