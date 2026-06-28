# Phase 3.1: RabbitMQ 核心概念

## 1. 安装

```bash
go get github.com/rabbitmq/amqp091-go
```

## 2. RabbitMQ 是什么？

RabbitMQ 是一个开源的消息代理（Message Broker），实现了 AMQP（Advanced Message Queuing Protocol）协议。它充当生产者和消费者之间的中间人，实现**异步通信**和**解耦**。

```
Producer → Exchange → Queue → Consumer
  (发布者)   (交换机)   (队列)   (消费者)
```

## 3. 核心组件

### Connection（连接）

基于 TCP 的长连接，客户端与 RabbitMQ 之间的网络连接。

```go
conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
defer conn.Close()
```

### Channel（通道）

建立在 Connection 之上的虚拟连接。所有 AMQP 操作都在 Channel 上进行，避免频繁创建/销毁 TCP 连接。

```go
ch, err := conn.Channel()
defer ch.Close()
```

### Exchange（交换机）

接收生产者的消息，根据规则路由到一个或多个队列。**消息不会直接发送到队列，必须经过 Exchange。**

交换机类型：

| 类型 | 路由规则 | 用途 |
|------|---------|------|
| **direct** | routing key 精确匹配 | 点对点通信 |
| **topic** | routing key 模式匹配（`*` 匹配一个词，`#` 匹配多个词） | 灵活的事件路由 |
| **fanout** | 广播到所有绑定的队列 | 通知所有消费者 |
| **headers** | 基于消息头匹配 | 复杂路由场景 |

本项目使用 **topic** 类型，routing key 为 `order.created`，队列绑定 `order.#` 匹配所有订单事件。

### Queue（队列）

存储消息的缓冲区，直到消费者取走。队列属性：

- **durable**：true 表示队列持久化，RabbitMQ 重启后队列不丢失
- **autoDelete**：true 表示最后一个消费者断开后自动删除
- **exclusive**：true 表示只允许一个连接使用，连接断开后删除

### Binding（绑定）

Exchange 与 Queue 之间的关联关系，指定 routing key 模式。

```
Exchange: order.events (topic)
    ↓ binding: order.#
Queue: order_notifications
```

### Routing Key（路由键）

生产者发布消息时指定的键，Exchange 根据它决定路由到哪个队列。

## 4. 消息投递流程

```
1. Producer → Exchange（带 routing key）
2. Exchange → 根据类型和 binding 规则路由
3. Queue 缓存消息
4. Consumer 从 Queue 拉取消息
5. Consumer 处理完毕后 ACK（确认）
```

## 5. 消息确认机制（ACK）

| 确认方式 | 说明 |
|---------|------|
| **Ack** | 消费者确认消息已正确处理，RabbitMQ 从队列移除 |
| **Nack** | 消费者拒绝消息，可选择重新入队或丢弃 |
| **Reject** | 类似 Nack，但不支持批量 |

本项目使用**手动确认**（autoAck=false），处理成功后 Ack，失败后 Nack + 重新入队。

```go
// 手动确认模式
msgs, _ := ch.Consume(queue, "", false, false, false, false, nil)

for msg := range msgs {
    // 处理消息
    if err := handle(msg.Body); err != nil {
        msg.Nack(false, true)  // 失败，重新入队
    } else {
        msg.Ack(false)         // 成功，确认
    }
}
```

## 6. 持久化

RabbitMQ 通过三重持久化保证消息不丢失：

| 层级 | 配置 | 说明 |
|------|------|------|
| Exchange | `durable: true` | 交换机持久化 |
| Queue | `durable: true` | 队列持久化 |
| Message | `DeliveryMode: amqp.Persistent` | 消息持久化 |

## 7. 公平分发（QoS）

默认 RabbitMQ 以轮询方式分发消息给多个消费者。设置 `Qos(PrefetchCount=1)` 可实现公平分发：只给空闲的消费者分配消息。

```go
ch.Qos(1, 0, false)  // prefetchCount=1
```

## 8. 管理界面

RabbitMQ Management UI 提供 Web 界面管理：

- `http://localhost:15672`（默认 guest/guest）
- 可查看 Exchanges、Queues、Connections、Channels
- 可手动发布/消费消息进行调试
- 可查看消息速率、队列深度等监控指标
