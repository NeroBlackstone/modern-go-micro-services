package redis

import (
	"context"
	"fmt"

	"modern-micro-services/internal/config"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var ctx = context.Background()

// NewClient 创建新的Redis客户端
func NewClient(cfg *config.RedisConfig, logger *zap.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to redis", zap.Error(err))
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	logger.Info("redis connected successfully",
		zap.String("addr", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)),
		zap.Int("db", cfg.DB),
	)

	return client, nil
}
