package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"modern-micro-services/internal/discovery"
	"modern-micro-services/internal/metrics"
	"modern-micro-services/internal/middleware"
	"modern-micro-services/internal/order/client"
	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/handler"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/repository"
	"modern-micro-services/internal/order/service"
	rabbitmqpkg "modern-micro-services/internal/order/rabbitmq"
	"modern-micro-services/internal/resilience"
	"modern-micro-services/internal/serviceauth"
	"modern-micro-services/internal/tracing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load("configs/order-service.yaml")
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

	// 初始化链路追踪（Tempo + OpenTelemetry）
	_, tracingShutdown := tracing.InitTracing("order-service", cfg.Tracing.Endpoint)
	defer tracingShutdown(context.Background())

	// 初始化 OTel Metrics（Prometheus exporter）
	metricsShutdown, err := tracing.InitMeterProvider("order-service", cfg.Server.Mode)
	if err != nil {
		logger.Fatal("failed to init meter provider", zap.Error(err))
	}
	defer metricsShutdown(context.Background())

	// 初始化 metric instruments
	if err := metrics.InitMetrics(); err != nil {
		logger.Fatal("failed to init metrics", zap.Error(err))
	}

	// 连接数据库
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}

	// 自动迁移
	if err := db.AutoMigrate(
		&model.Order{},
		&model.OrderItem{},
		&model.Review{},
	); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}
	logger.Info("database migration completed")

	// 初始化 RabbitMQ
	rabbitConn, err := rabbitmqpkg.NewConnection(&cfg.RabbitMQ, logger)
	if err != nil {
		logger.Fatal("failed to connect to rabbitmq", zap.Error(err))
	}
	defer rabbitConn.Close()

	producer, err := rabbitmqpkg.NewProducer(rabbitConn, logger)
	if err != nil {
		logger.Fatal("failed to create rabbitmq producer", zap.Error(err))
	}
	defer producer.Close()

	// 初始化 Consul 服务发现
	registry, err := discovery.NewRegistry(cfg.Consul.Addr, logger)
	if err != nil {
		logger.Fatal("failed to create consul registry", zap.Error(err))
	}

	// 创建 Consul resolver builder
	consulBuilder := discovery.NewConsulResolverBuilder(registry, logger)

	// 注册自定义 resolver 到 gRPC
	resolver.Register(consulBuilder)

	// ========== 初始化容错组件 ==========

	// 为 book-service 创建熔断器
	bookCB := resilience.NewCircuitBreaker(
		resilience.CircuitBreakerConfig{
			Name:        "book-service",
			MaxRequests: 3,
			Interval:    30 * time.Second,
			Timeout:     10 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures > 5
			},
		},
		logger,
	)

	// 重试配置
	retryCfg := resilience.DefaultRetryConfig(logger)

	// 为 book-service 创建限流器（每秒 100 请求，突发 50）
	bookLimiter := resilience.NewGRPCRateLimiter(resilience.RateLimiterConfig{Rate: 100, Burst: 50})

	// ========== 初始化服务间认证 ==========
	serviceAuthTTL, err := time.ParseDuration(cfg.ServiceAuth.TTL)
	if err != nil {
		serviceAuthTTL = 5 * time.Minute
	}
	serviceAuthCfg := &serviceauth.Config{
		SharedSecret:  cfg.ServiceAuth.SharedSecret,
		ServiceName:   cfg.ServiceAuth.ServiceName,
		TargetService: cfg.ServiceAuth.TargetService,
		TTL:           serviceAuthTTL,
	}

	// ========== 初始化 gRPC clients，使用 consul:/// 服务名 进行服务发现 ==========
	// 拦截器链：Tracing → ServiceAuth → CircuitBreaker → Retry → RateLimiter
	bookConn, err := grpc.NewClient(
		"consul:///book-service",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
		grpc.WithChainUnaryInterceptor(
			tracing.UnaryClientInterceptor(),
			serviceauth.UnaryClientInterceptor(serviceAuthCfg, logger),
			resilience.CircuitBreakerInterceptor(bookCB),
			resilience.RetryInterceptor(retryCfg),
			resilience.RateLimiterInterceptor(bookLimiter),
		),
	)
	if err != nil {
		logger.Fatal("failed to connect to book-service via consul", zap.Error(err))
	}
	bookClient := client.NewBookClientFromConn(bookConn, logger)

	// 初始化各层
	orderRepo := repository.NewOrderRepository(db)
	reviewRepo := repository.NewReviewRepository(db)

	orderSvc := service.NewOrderService(orderRepo, bookClient, producer, logger, db)
	reviewSvc := service.NewReviewService(reviewRepo)

	orderHandler := handler.NewOrderHandler(orderSvc, reviewSvc, &cfg.JWT)

	// 设置路由
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Tracing())
	r.Use(middleware.MetricsMiddleware("order-service"))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "order-service"})
	})

	// Prometheus 指标端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 路由组
	v1 := r.Group("/api/v1")
	{
		// 订单相关（需要认证）
		// 注意：认证现在由 Traefik + Oathkeeper 在网关层处理
		// 后端服务只需读取 Oathkeeper 注入的 Authorization header
		orders := v1.Group("/orders")
		orders.Use(orderHandler.JWTAuth()) // 使用 Oathkeeper JWT 验证
		{
			orders.POST("", orderHandler.CreateOrder)
			orders.GET("", orderHandler.ListOrders)
			orders.GET("/:id", orderHandler.GetOrder)
		}

		// 评价相关
		reviews := v1.Group("/reviews")
		{
			reviews.GET("/book/:book_id", orderHandler.ListReviews)
			reviews.GET("/book/:book_id/stats", orderHandler.GetReviewStats)
			reviews.POST("", orderHandler.JWTAuth(), orderHandler.CreateReview)
		}
	}

	// 启动 HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("order-service HTTP server starting", zap.String("addr", addr))

	if err := r.Run(addr); err != nil {
		logger.Fatal("failed to start server", zap.Error(err))
	}
}
