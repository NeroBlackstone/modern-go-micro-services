package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"modern-micro-services/internal/config"
	rabbitmqpkg "modern-micro-services/pkg/rabbitmq"

	"go.uber.org/zap"
)

func main() {
	// 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 初始化日志
	var logger *zap.Logger
	if cfg.Server.Mode == "debug" {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	// 连接 RabbitMQ
	conn, err := rabbitmqpkg.NewConnection(&cfg.RabbitMQ, logger)
	if err != nil {
		logger.Fatal("failed to connect to rabbitmq", zap.Error(err))
	}
	defer conn.Close()

	// 创建消费者
	consumer, err := rabbitmqpkg.NewConsumer(conn, logger)
	if err != nil {
		logger.Fatal("failed to create consumer", zap.Error(err))
	}
	defer consumer.Close()

	// 注册消息处理器
	handler := func(body []byte) error {
		// 简单打印消息内容，模拟通知处理
		// 后续可扩展为：发送邮件、短信、推送等
		logger.Info("received order notification",
			zap.String("message", string(body)),
		)
		return nil
	}

	// 监听系统信号，支持优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 在 goroutine 中启动消费者
	errCh := make(chan error, 1)
	go func() {
		logger.Info("notification consumer started, waiting for messages...")
		errCh <- consumer.StartListening(handler)
	}()

	// 等待退出信号或错误
	select {
	case <-quit:
		logger.Info("shutting down consumer...")
	case err := <-errCh:
		if err != nil {
			logger.Fatal("consumer error", zap.Error(err))
		}
	}

	fmt.Println("consumer stopped")
}
