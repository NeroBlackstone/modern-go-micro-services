# Day 4 - Saga 分布式事务模式

## 什么是分布式事务？

在微服务架构中，一个业务操作可能涉及多个服务的数据变更。例如"创建订单"需要：
1. 在 order-service 创建订单记录
2. 在 book-service 扣减库存

这两个操作需要要么都成功，要么都失败。但由于它们在不同的数据库中，无法使用传统的数据库事务。

## Saga 模式

Saga 是一种管理分布式事务的模式，将一个长事务拆分为一系列本地事务，每个本地事务都有对应的**补偿操作**。

### 核心思想

```
正向流程：T1 → T2 → T3 → ... → Tn
补偿流程：C1 ← C2 ← C3 ← ... ← Cn

如果 T3 失败：执行 C2 → C1（逆序补偿）
```

### 两种实现方式

1. **编排式（Choreography）**：每个服务监听事件并决定下一步
2. **协调式（Orchestration）**：一个协调器负责指挥整个流程

本项目使用**协调式 Saga**，由 order-service 作为协调者。

## 项目中的 Saga 实现

### 创建订单的 Saga 流程

```
order-service (协调者)
│
├─ Step 1: 本地创建订单 (status=pending)
│   └─ 成功 → 继续
│   └─ 失败 → 终止，返回错误
│
├─ Step 2: gRPC 调用 book-service.DeductStock()
│   └─ 成功 → 继续
│   └─ 失败 → 执行补偿 ↓
│
├─ Step 3 (成功): 更新订单 status=paid
│   └─ 发布 OrderCreatedEvent 到 RabbitMQ
│
└─ Step 3 (失败补偿):
    ├─ 调用 book-service.RestoreStock() 恢复库存
    └─ 更新订单 status=cancelled
```

### 代码实现

```go
func (s *orderService) Create(userID uint, req *model.CreateOrderRequest) (*model.Order, error) {
    // Step 1: 本地创建订单 (pending)
    order := &model.Order{...}
    s.orderRepo.Create(order)

    // Step 2: gRPC 扣减库存
    for _, item := range req.Items {
        if err := s.bookClient.DeductStock(item.BookID, item.Quantity); err != nil {
            // 失败 → 补偿
            s.compensate(order, req.Items)
            return nil, err
        }
    }

    // Step 3: 更新为 paid
    s.orderRepo.UpdateStatus(order.ID, model.OrderStatusPaid)
    s.producer.PublishOrderCreated(event)

    return order, nil
}

// compensate 补偿逻辑
func (s *orderService) compensate(order *model.Order, items []model.OrderItemRequest) {
    // 恢复库存
    for _, item := range items {
        s.bookClient.RestoreStock(item.BookID, item.Quantity)
    }
    // 取消订单
    s.orderRepo.UpdateStatus(order.ID, model.OrderStatusCancelled)
}
```

## 补偿操作设计

### 补偿操作的要求

1. **幂等性**：补偿操作执行多次，结果相同
2. **可交换性**：补偿操作的顺序不影响最终结果
3. **可完成性**：补偿操作必须最终能成功

### 本项目的补偿操作

| 操作 | 补偿操作 | 幂等性 |
|------|----------|--------|
| 创建订单 | 取消订单 | ✅ 已取消的订单再次取消无影响 |
| 扣减库存 | 恢复库存 | ⚠️ 需要确保不会多次恢复 |

## Saga 的挑战

### 1. 补偿失败

如果补偿操作也失败了（如 RestoreStock 失败），需要：
- 记录失败日志
- 人工介入处理
- 或者使用重试队列

### 2. 并发问题

多个订单同时扣减同一个图书的库存，需要：
- 数据库行锁（`WHERE stock >= quantity`）
- 或者使用 Redis 分布式锁

### 3. 数据不一致窗口

在 Saga 执行过程中，数据可能处于中间状态：
- 订单已创建但库存未扣减
- 库存已扣减但订单未更新状态

这被称为**最终一致性**，是分布式系统的常见权衡。

## 替代方案对比

| 模式 | 一致性 | 性能 | 复杂度 | 适用场景 |
|------|--------|------|--------|----------|
| 2PC（两阶段提交） | 强一致 | 低 | 高 | 金融、支付 |
| TCC（Try-Confirm-Cancel） | 强一致 | 中 | 高 | 金融、支付 |
| Saga | 最终一致 | 高 | 中 | 电商、订单 |
| 本地消息表 | 最终一致 | 高 | 中 | 异步处理 |

## 扩展阅读

- [Microservices Patterns - Saga](https://microservices.io/patterns/data/saga.html)
- [Chris Richardson - Saga Pattern](https://chrisrichardson.me/post/using-sagas-to-maintain-data-consistency.html)
- [Event Sourcing vs Saga](https://martinfowler.com/eaaDev/EventSourcing.html)
