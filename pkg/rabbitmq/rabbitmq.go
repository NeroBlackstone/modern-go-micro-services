package rabbitmq

import (
	"fmt"

	"modern-micro-services/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// NewConnection 创建 RabbitMQ 连接
func NewConnection(cfg *config.RabbitMQConfig, logger *zap.Logger) (*amqp.Connection, error) {
	conn, err := amqp.Dial(cfg.AmqpURL())
	if err != nil {
		logger.Error("failed to connect to rabbitmq", zap.Error(err))
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}

	logger.Info("rabbitmq connected successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
	)

	return conn, nil
}
