package rabbitmq

import (
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	// OrderExchange 订单事件 exchange
	OrderExchange = "order.events"
	// OrderCreatedKey 订单创建事件 routing key
	OrderCreatedKey = "order.created"
)

// Producer 消息生产者
type Producer struct {
	ch     *amqp.Channel
	logger *zap.Logger
}

// NewProducer 创建消息生产者
func NewProducer(conn *amqp.Connection, logger *zap.Logger) (*Producer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}

	// 声明 topic exchange，用于订单事件分发
	err = ch.ExchangeDeclare(
		OrderExchange, // name
		"topic",       // type
		true,          // durable
		false,         // autoDelete
		false,         // internal
		false,         // noWait
		nil,           // args
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("declare exchange: %w", err)
	}

	logger.Info("rabbitmq producer initialized", zap.String("exchange", OrderExchange))

	return &Producer{ch: ch, logger: logger}, nil
}

// PublishOrderCreated 发布订单创建事件
func (p *Producer) PublishOrderCreated(event any) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	err = p.ch.Publish(
		OrderExchange,   // exchange
		OrderCreatedKey, // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // 消息持久化，重启后不丢失
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	p.logger.Info("order event published",
		zap.String("routing_key", OrderCreatedKey),
		zap.Int("body_size", len(body)),
	)

	return nil
}

// Close 关闭生产者
func (p *Producer) Close() {
	if p.ch != nil {
		p.ch.Close()
	}
}
