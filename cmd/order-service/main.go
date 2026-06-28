package main

import (
	"fmt"
	"log"

	"modern-micro-services/internal/order/client"
	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/handler"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/repository"
	"modern-micro-services/internal/order/service"
	rabbitmqpkg "modern-micro-services/internal/order/rabbitmq"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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

	// 初始化 gRPC clients
	bookClient, err := client.NewBookClient(cfg.GRPC.BookServiceAddr, logger)
	if err != nil {
		logger.Fatal("failed to connect to book-service", zap.Error(err))
	}
	defer bookClient.Close()

	userClient, err := client.NewUserClient(cfg.GRPC.UserServiceAddr, logger)
	if err != nil {
		logger.Warn("failed to connect to user-service (non-critical)", zap.Error(err))
		// user-service 不影响核心下单流程，降级处理
	}
	if userClient != nil {
		defer userClient.Close()
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
