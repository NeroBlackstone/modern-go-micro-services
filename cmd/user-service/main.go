package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"modern-micro-services/internal/discovery"
	"modern-micro-services/internal/user/config"
	"modern-micro-services/internal/user/handler"
	"modern-micro-services/internal/user/model"
	"modern-micro-services/internal/user/repository"
	"modern-micro-services/internal/user/server"
	"modern-micro-services/internal/user/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// 加载配置
	cfg, err := config.Load("configs/user-service.yaml")
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

	// 连接数据库
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}

	// 自动迁移
	if err := db.AutoMigrate(&model.User{}); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}
	logger.Info("database migration completed")

	// 初始化各层
	userRepo := repository.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo, &cfg.JWT)

	// HTTP handlers (register, login, profile)
	httpHandler := handler.NewHTTPHandler(userSvc, &cfg.JWT)

	// gRPC handler (GetUser for internal service calls)
	grpcHandler := handler.NewGRPCHandler(userSvc)

	// 启动 gRPC server
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
		ServiceName: "user-service",
		Address:     hostname,
		Port:        cfg.Server.GRPCPort,
		Tags:        []string{"grpc", "user"},
		Meta: map[string]string{
			"gRPC_port": fmt.Sprintf("%d", cfg.Server.GRPCPort),
		},
	}, 10*time.Second)
	if err != nil {
		logger.Fatal("failed to register to consul", zap.Error(err))
	}

	// 启动 HTTP server (register, login, profile)
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 公开接口
	r.POST("/api/v1/auth/register", httpHandler.Register)
	r.POST("/api/v1/auth/login", httpHandler.Login)

	// 需要认证的接口
	authorized := r.Group("/api/v1/user")
	authorized.Use(httpHandler.JWTAuth())
	{
		authorized.GET("/profile", httpHandler.GetProfile)
		authorized.PUT("/profile", httpHandler.UpdateProfile)
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "user-service"})
	})

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
		logger.Info("user-service HTTP server starting", zap.String("addr", addr))
		if err := r.Run(addr); err != nil {
			logger.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	<-quit
	logger.Info("shutting down user-service...")

	// 注销服务
	registry.Deregister(fmt.Sprintf("user-service-%s-%d", hostname, cfg.Server.GRPCPort))

	grpcServer.Stop()
}
