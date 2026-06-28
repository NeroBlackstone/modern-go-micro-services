package main

import (
	"fmt"
	"log"

	"modern-micro-services/internal/discovery"
	"modern-micro-services/internal/order/client"
	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/handler"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/repository"
	"modern-micro-services/internal/order/service"
	rabbitmqpkg "modern-micro-services/internal/order/rabbitmq"

	"github.com/gin-gonic/gin"
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

	// 初始化 gRPC clients，使用 consul:/// 服务名 进行服务发现
	// gRPC 会自动使用我们的 Consul resolver 解析地址
	bookConn, err := grpc.NewClient(
		"consul:///book-service",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(discovery.ServiceConfigJSON()),
	)
	if err != nil {
		logger.Fatal("failed to connect to book-service via consul", zap.Error(err))
	}
	bookClient := client.NewBookClientFromConn(bookConn, logger)

	userConn, err := grpc.NewClient(
		"consul:///user-service",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(discovery.ServiceConfigJSON()),
	)
	if err != nil {
		logger.Warn("failed to connect to user-service via consul (non-critical)", zap.Error(err))
	}
	if userConn != nil {
		defer userConn.Close()
	}

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

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "order-service"})
	})

	// API v1 路由组
	v1 := r.Group("/api/v1")
	{
		// 订单相关（需要认证）
		orders := v1.Group("/orders")
		orders.Use(orderHandler.JWTAuth())
		{
			orders.POST("", orderHandler.CreateOrder)
			orders.GET("", orderHandler.ListOrders)
			orders.GET("/:id", orderHandler.GetOrder)
			orders.PUT("/:id/status", orderHandler.UpdateOrderStatus)
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
