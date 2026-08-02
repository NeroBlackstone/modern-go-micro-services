package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"modern-micro-services/internal/admin/handler"
	"modern-micro-services/internal/discovery"
	"modern-micro-services/internal/metrics"
	"modern-micro-services/internal/middleware"
	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/repository"
	"modern-micro-services/internal/tracing"
	"modern-micro-services/internal/vaultclient"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load("configs/admin-service.yaml")
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

	// ========== Vault 密钥管理 ==========
	vaultClient := vaultclient.NewClientFromEnv()
	if vaultClient != nil {
		logger.Info("vault client connected, overlaying secrets")
		kvPath := os.Getenv("VAULT_KV_PATH")
		// 从 Vault 覆盖数据库配置
		vaultclient.OverlayDatabaseConfig(
			vaultClient,
			kvPath,
			&cfg.Database.Host,
			&cfg.Database.Port,
			&cfg.Database.User,
			&cfg.Database.Password,
			&cfg.Database.DBName,
			logger,
		)
		// 从 Vault 覆盖 JWT 密钥
		if secrets, err := vaultclient.GetKVSecret(vaultClient, "secret/data/jwt"); err == nil {
			vaultclient.OverlayString(secrets, "secret", &cfg.JWT.Secret)
		}
	} else {
		logger.Info("vault not available, using local config")
	}

	// 初始化链路追踪
	_, tracingShutdown := tracing.InitTracing("admin-service", cfg.Tracing.Endpoint)
	defer tracingShutdown(context.Background())

	// 初始化 OTel Metrics
	metricsShutdown, err := tracing.InitMeterProvider("admin-service", cfg.Server.Mode)
	if err != nil {
		logger.Fatal("failed to init meter provider", zap.Error(err))
	}
	defer metricsShutdown(context.Background())

	if err := metrics.InitMetrics(); err != nil {
		logger.Fatal("failed to init metrics", zap.Error(err))
	}

	// 连接数据库（与 order-service 共享 order_db）
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}

	if err := db.AutoMigrate(
		&model.Order{},
		&model.OrderItem{},
		&model.Review{},
	); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}
	logger.Info("database migration completed")

	// 初始化 Consul 服务发现
	registry, err := discovery.NewRegistry(cfg.Consul.Addr, logger)
	if err != nil {
		logger.Fatal("failed to create consul registry", zap.Error(err))
	}

	// 注册服务到 Consul
	reg := &discovery.ServiceRegistration{
		ServiceName: "admin-service",
		Address:     "admin-service",
		Port:        cfg.Server.Port,
		Tags:        []string{"admin"},
	}
	if err := registry.Register(reg, 30*time.Second); err != nil {
		logger.Fatal("failed to register admin-service", zap.Error(err))
	}
	defer registry.Deregister("admin-service")

	// 初始化各层
	orderRepo := repository.NewOrderRepository(db)
	adminHandler := handler.NewAdminHandler(orderRepo, &cfg.JWT, logger)

	// 设置路由
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Tracing())
	r.Use(middleware.MetricsMiddleware("admin-service"))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "admin-service"})
	})

	// Prometheus 指标端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 管理 API
	v1 := r.Group("/api/v1/admin")
	v1.Use(adminHandler.JWTAuth())
	{
		orders := v1.Group("/orders")
		{
			orders.PUT("/:id/status", adminHandler.UpdateOrderStatus)
		}
	}

	// 启动 HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("admin-service HTTP server starting", zap.String("addr", addr))

	if err := r.Run(addr); err != nil {
		logger.Fatal("failed to start server", zap.Error(err))
	}
}
