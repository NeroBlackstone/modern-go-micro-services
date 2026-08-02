package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	"modern-micro-services/internal/metrics"
	"modern-micro-services/internal/serviceauth"
	"modern-micro-services/internal/tracing"
	"modern-micro-services/internal/vaultclient"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
		// 从 Vault 覆盖服务间认证密钥
		if secrets, err := vaultclient.GetKVSecret(vaultClient, "secret/data/service-auth"); err == nil {
			vaultclient.OverlayString(secrets, "shared_secret", &cfg.ServiceAuth.SharedSecret)
		}
	} else {
		logger.Info("vault not available, using local config")
	}

	// 初始化链路追踪（Tempo + OpenTelemetry）
	_, tracingShutdown := tracing.InitTracing("book-service", cfg.Tracing.Endpoint)
	defer tracingShutdown(context.Background())

	// 初始化 OTel Metrics（Prometheus exporter）
	metricsShutdown, err := tracing.InitMeterProvider("book-service", cfg.Server.Mode)
	if err != nil {
		logger.Fatal("failed to init meter provider", zap.Error(err))
	}
	defer metricsShutdown(context.Background())

	// 初始化 metric instruments
	if err := metrics.InitMetrics(); err != nil {
		logger.Fatal("failed to init metrics", zap.Error(err))
	}

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

	// 初始化服务间认证配置
	serviceAuthCfg := &serviceauth.Config{
		SharedSecret: cfg.ServiceAuth.SharedSecret,
		ServiceName:  cfg.ServiceAuth.ServiceName,
	}

	grpcServer, err := server.NewGRPCServer(grpcHandler, cfg.Server.GRPCPort, logger, serviceAuthCfg)
	if err != nil {
		logger.Fatal("failed to create gRPC server", zap.Error(err))
	}

	go func() {
		if err := grpcServer.Start(); err != nil {
			logger.Fatal("gRPC server error", zap.Error(err))
		}
	}()

	// 启动 HTTP 服务器用于 metrics 和健康检查
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
		metricsMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "book-service"})
		})

		metricsAddr := fmt.Sprintf(":%d", cfg.Server.MetricsPort)
		logger.Info("book-service metrics server starting", zap.String("addr", metricsAddr))
		metricsServer := &http.Server{
			Addr:    metricsAddr,
			Handler: metricsMux,
		}

		// 优雅关闭
		go func() {
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			<-quit
			logger.Info("shutting down metrics server...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			metricsServer.Shutdown(ctx)
		}()

		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("metrics server error", zap.Error(err))
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
