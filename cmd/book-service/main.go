package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"modern-micro-services/internal/book/config"
	"modern-micro-services/internal/book/handler"
	"modern-micro-services/internal/book/model"
	"modern-micro-services/internal/book/repository"
	"modern-micro-services/internal/book/server"
	"modern-micro-services/internal/book/service"
	redispkg "modern-micro-services/internal/book/redis"

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down book-service...")
	grpcServer.Stop()
}
