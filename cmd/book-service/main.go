package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"modern-micro-services/internal/book/config"
	"modern-micro-services/internal/book/handler"
	"modern-micro-services/internal/book/model"
	"modern-micro-services/internal/book/repository"
	"modern-micro-services/internal/book/server"
	"modern-micro-services/internal/book/service"
	redispkg "modern-micro-services/internal/book/redis"
	"modern-micro-services/internal/discovery"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load("configs/book-service.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	var logger *zap.Logger
	if cfg.Server.Mode == "debug" {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}

	if err := db.AutoMigrate(&model.Book{}); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}
	logger.Info("database migration completed")

	redisClient, err := redispkg.NewClient(&cfg.Redis, logger)
	if err != nil {
		logger.Fatal("failed to connect to redis", zap.Error(err))
	}
	defer redisClient.Close()

	bookRepo := repository.NewCachedBookRepository(db, redisClient, logger)
	bookSvc := service.NewBookService(bookRepo)
	grpcHandler := handler.NewGRPCHandler(bookSvc)

	grpcServer, err := server.NewGRPCServer(grpcHandler, cfg.Server.GRPCPort, logger)
	if err != nil {
		logger.Fatal("failed to create gRPC server", zap.Error(err))
	}

	go func() {
		if err := grpcServer.Start(); err != nil {
			logger.Fatal("gRPC server error", zap.Error(err))
		}
	}()

	// 注册到 Consul
	registry, err := discovery.NewRegistry(cfg.Consul.Addr, logger)
	if err != nil {
		logger.Fatal("failed to create consul registry", zap.Error(err))
	}

	// 获取本机 IP（在 Docker 中使用容器名）
	hostname, _ := os.Hostname()
	err = registry.Register(&discovery.ServiceRegistration{
		ServiceName: "book-service",
		Address:     hostname,
		Port:        cfg.Server.GRPCPort,
		Tags:        []string{"grpc", "book"},
		Meta: map[string]string{
			"gRPC_port": fmt.Sprintf("%d", cfg.Server.GRPCPort),
		},
	}, 10*time.Second)
	if err != nil {
		logger.Fatal("failed to register to consul", zap.Error(err))
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down book-service...")

	// 注销服务
	registry.Deregister(fmt.Sprintf("book-service-%s-%d", hostname, cfg.Server.GRPCPort))

	grpcServer.Stop()
}
