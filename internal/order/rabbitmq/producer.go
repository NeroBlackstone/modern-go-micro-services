package rabbitmq

import (
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	OrderExchange  = "order.events"
	OrderCreatedKey = "order.created"
)

type Producer struct {
	ch     *amqp.Channel
	logger *zap.Logger
}

func NewProducer(conn *amqp.Connection, logger *zap.Logger) (*Producer, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}

	err = ch.ExchangeDeclare(
		OrderExchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("declare exchange: %w", err)
	}

	logger.Info("rabbitmq producer initialized", zap.String("exchange", OrderExchange))

	return &Producer{ch: ch, logger: logger}, nil
}

func (p *Producer) PublishOrderCreated(event any) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	err = p.ch.Publish(
		OrderExchange,
		OrderCreatedKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
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

func (p *Producer) Close() {
	if p.ch != nil {
		p.ch.Close()
	}
}
