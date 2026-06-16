package main

import (
	"fmt"
	"log"

	"modern-micro-services/internal/config"
	"modern-micro-services/internal/handler"
	"modern-micro-services/internal/model"
	"modern-micro-services/internal/repository"
	"modern-micro-services/internal/service"

	_ "modern-micro-services/docs" // Swagger docs

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// @title           在线书店 API
// @version         1.0
// @description     在线书店微服务单体应用 API 文档
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.email  support@bookstore.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
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

	// 连接数据库
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
	if err != nil {
		logger.Fatal("failed to connect database", zap.Error(err))
	}

	// 自动迁移
	if err := db.AutoMigrate(
		&model.User{},
		&model.Book{},
		&model.Order{},
		&model.OrderItem{},
		&model.Review{},
	); err != nil {
		logger.Fatal("failed to migrate database", zap.Error(err))
	}
	logger.Info("database migration completed")

	// 初始化各层
	userRepo := repository.NewUserRepository(db)
	bookRepo := repository.NewBookRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	reviewRepo := repository.NewReviewRepository(db)

	userSvc := service.NewUserService(userRepo, &cfg.JWT)
	bookSvc := service.NewBookService(bookRepo)
	orderSvc := service.NewOrderService(orderRepo, bookRepo, db)
	reviewSvc := service.NewReviewService(reviewRepo)

	userHandler := handler.NewUserHandler(userSvc)
	bookHandler := handler.NewBookHandler(bookSvc)
	orderHandler := handler.NewOrderHandler(orderSvc)
	reviewHandler := handler.NewReviewHandler(reviewSvc)

	// 设置路由
	router := handler.NewRouter(cfg, logger, userHandler, bookHandler, orderHandler, reviewHandler)
	engine := router.Setup()

	// 启动服务器
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("server starting", zap.String("addr", addr))
	logger.Info("swagger docs available at", zap.String("url", fmt.Sprintf("http://localhost:%d/swagger/index.html", cfg.Server.Port)))

	if err := engine.Run(addr); err != nil {
		logger.Fatal("failed to start server", zap.Error(err))
	}
}
