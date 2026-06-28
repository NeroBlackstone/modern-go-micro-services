# Phase 3.2: 异步通信模式

## 1. 同步 vs 异步

### 同步调用（改造前）

```
用户下单 → [扣库存] → [创建订单] → [发通知] → 返回响应
                         ↑ 全部在同一事务中
```

- 优点：简单，强一致性
- 缺点：通知失败会影响下单；响应时间 = 所有步骤之和

### 异步解耦（改造后）

```
用户下单 → [扣库存 + 创建订单] → 返回响应
                    ↓ (MQ)
              [发通知] (异步)
```

- 优点：通知失败不影响下单；响应时间 = 扣库存 + 创建订单
- 缺点：最终一致性，需要额外处理失败场景

## 2. 事件驱动架构（EDA）

Event-Driven Architecture 是微服务间通信的核心模式。

### 核心概念

- **Event（事件）**：系统中发生的事实，如 `order.created`、`order.paid`
- **Producer**：产生事件的组件
- **Consumer**：消费事件并做出响应的组件
- **Event Channel**：传递事件的中间件（RabbitMQ、Kafka 等）

### 事件命名规范

```
<实体>.<动作>     →  order.created, order.paid, user.registered
<实体>.<动作>.<状态> →  order.status.changed
```

### 事件结构

```json
{
    "order_id": 1,
    "user_id": 100,
    "total_amount": 299.00,
    "status": "pending",
    "items": [
        {"book_id": 1, "book_title": "Go 语言圣经", "quantity": 2, "price": 149.50}
    ],
    "created_at": "2025-01-15T10:30:00Z"
}
```

## 3. 消息队列的三种模式

### 点对点（Point-to-Point）

一个消息只被一个消费者处理。

```
Producer → Queue → Consumer
```

适用场景：任务分发、工作队列

### 发布/订阅（Publish/Subscribe）

一个消息被多个消费者处理。

```
Producer → Exchange → Queue1 → Consumer1
                    → Queue2 → Consumer2
```

适用场景：广播通知、多系统同步

### 事件路由（Event Routing）

根据 routing key 选择性地路由消息。

```
Producer → Exchange (topic)
    routing_key="order.created" → Queue1 (订单服务)
    routing_key="order.paid"    → Queue2 (支付服务)
    routing_key="order.#"       → Queue3 (审计服务)
```

适用场景：复杂的事件流转

## 4. 最终一致性

分布式系统中，不追求强一致性，而是保证最终所有系统达到一致状态。

```
时间线：
T0: 订单服务：订单已创建 ✓
T1: MQ 传递事件
T2: 通知服务：收到事件 ✓
T3: 通知服务：发送邮件 ✓

→ 最终一致：订单和通知都完成了
```

### 失败处理策略

| 策略 | 说明 | 适用场景 |
|------|------|---------|
| **重试** | 失败后自动重试 N 次 | 瞬时故障（网络抖动） |
| **死信队列（DLQ）** | 重试 N 次后转入死信队列 | 永久性故障 |
| **幂等消费** | 同一消息处理多次结果相同 | 保证重试安全 |
| **补偿事务** | 执行反向操作回滚 | 关键业务流程 |

## 5. 本项目的实现

### 事件流

```
order_svc.CreateOrder()
    ↓ 事务提交成功
producer.PublishOrderCreated()
    ↓ AMQP
Exchange: order.events (topic, routing_key="order.created")
    ↓ binding: order.#
Queue: order_notifications
    ↓
consumer: handleOrderCreated()
    → 打印日志（模拟发送通知）
```

### 关键设计决策

1. **Fire-and-forget**：消息发布失败只记日志，不阻塞主流程
2. **持久化**：exchange、queue、message 都设置 durable/persistent
3. **手动 ACK**：处理成功才确认，失败重新入队
4. **公平分发**：prefetch=1，避免消费者过载
5. **独立进程**：consumer 是独立二进制，可以独立部署和扩展

## 6. 后续扩展方向

- [ ] 通知服务：发送真实邮件（SMTP）
- [ ] 死信队列：处理多次失败的消息
- [ ] 消息幂等性：基于 order_id 去重
- [ ] 事件溯源：记录所有事件用于审计
- [ ] 换用 Kafka：当需要事件回放或高吞吐时
