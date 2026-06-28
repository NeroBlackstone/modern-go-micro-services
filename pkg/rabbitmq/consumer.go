package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	// OrderNotificationQueue 订单通知队列
	OrderNotificationQueue = "order_notifications"
)

// MessageHandler 消息处理函数签名
type MessageHandler func(body []byte) error

// Consumer 消息消费者
type Consumer struct {
	ch     *amqp.Channel
	logger *zap.Logger
}

// NewConsumer 创建消息消费者
func NewConsumer(conn *amqp.Connection, logger *zap.Logger) (*Consumer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}

	// 声明队列（如果不存在则创建）
	_, err = ch.QueueDeclare(
		OrderNotificationQueue, // name
		true,                   // durable — 持久化队列，重启后不丢失
		false,                  // autoDelete
		false,                  // exclusive
		false,                  // noWait
		nil,                    // args
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("declare queue: %w", err)
	}

	// 将队列绑定到 exchange，接收所有 order.* 事件
	err = ch.QueueBind(
		OrderNotificationQueue, // queue
		"order.#",              // routing key pattern — 匹配所有 order 开头的事件
		OrderExchange,          // exchange
		false,                  // noWait
		nil,                    // args
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("bind queue: %w", err)
	}

	logger.Info("rabbitmq consumer initialized",
		zap.String("queue", OrderNotificationQueue),
		zap.String("binding", "order.#"),
	)

	return &Consumer{ch: ch, logger: logger}, nil
}

// StartListening 开始监听消息（阻塞）
func (c *Consumer) StartListening(handler MessageHandler) error {
	// 每次只预取 1 条消息，处理完再拿下一条（公平分发）
	err := c.ch.Qos(1, 0, false)
	if err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	msgs, err := c.ch.Consume(
		OrderNotificationQueue, // queue
		"",                    // consumer — 空字符串让 RabbitMQ 自动生成
		false,                 // autoAck — 关闭自动确认，手动 ACK
		false,                 // exclusive
		false,                 // noLocal
		false,                 // noWait
		nil,                   // args
	)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	// 在当前 goroutine 中处理消息
	for msg := range msgs {
		if err := handler(msg.Body); err != nil {
			c.logger.Error("failed to handle message",
				zap.Error(err),
				zap.ByteString("body", msg.Body),
			)
			// 处理失败，拒绝消息并重新入队（稍后重试）
			msg.Nack(false, true)
		} else {
			// 处理成功，确认消息
			msg.Ack(false)
		}
	}

	return nil
}

// Close 关闭消费者
func (c *Consumer) Close() {
	if c.ch != nil {
		c.ch.Close()
	}
}
