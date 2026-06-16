package handler

import (
	"modern-micro-services/internal/config"
	"modern-micro-services/internal/middleware"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

type Router struct {
	engine        *gin.Engine
	cfg           *config.Config
	logger        *zap.Logger
	userHandler   *UserHandler
	bookHandler   *BookHandler
	orderHandler  *OrderHandler
	reviewHandler *ReviewHandler
}

func NewRouter(
	cfg *config.Config,
	logger *zap.Logger,
	userHandler *UserHandler,
	bookHandler *BookHandler,
	orderHandler *OrderHandler,
	reviewHandler *ReviewHandler,
) *Router {
	return &Router{
		engine:        gin.New(),
		cfg:           cfg,
		logger:        logger,
		userHandler:   userHandler,
		bookHandler:   bookHandler,
		orderHandler:  orderHandler,
		reviewHandler: reviewHandler,
	}
}

func (r *Router) Setup() *gin.Engine {
	gin.SetMode(r.cfg.Server.Mode)

	// 全局中间件
	r.engine.Use(middleware.Recovery(r.logger))
	r.engine.Use(middleware.Logger(r.logger))
	r.engine.Use(middleware.CORS())

	// Swagger 文档路由
	r.engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 健康检查
	r.engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API v1 路由组
	v1 := r.engine.Group("/api/v1")
	{
		// 认证相关（公开接口）
		auth := v1.Group("/auth")
		{
			auth.POST("/register", r.userHandler.Register)
			auth.POST("/login", r.userHandler.Login)
		}

		// 图书相关
		books := v1.Group("/books")
		{
			books.GET("", r.bookHandler.ListBooks)
			books.GET("/:id", r.bookHandler.GetBook)

			// 需要认证的操作
			books.POST("", middleware.JWTAuth(&r.cfg.JWT), r.bookHandler.CreateBook)
			books.PUT("/:id", middleware.JWTAuth(&r.cfg.JWT), r.bookHandler.UpdateBook)
			books.DELETE("/:id", middleware.JWTAuth(&r.cfg.JWT), r.bookHandler.DeleteBook)
		}

		// 用户相关（需要认证）
		user := v1.Group("/user")
		user.Use(middleware.JWTAuth(&r.cfg.JWT))
		{
			user.GET("/profile", r.userHandler.GetProfile)
			user.PUT("/profile", r.userHandler.UpdateProfile)
		}

		// 订单相关（需要认证）
		orders := v1.Group("/orders")
		orders.Use(middleware.JWTAuth(&r.cfg.JWT))
		{
			orders.POST("", r.orderHandler.CreateOrder)
			orders.GET("", r.orderHandler.ListOrders)
			orders.GET("/:id", r.orderHandler.GetOrder)
			orders.PUT("/:id/status", r.orderHandler.UpdateOrderStatus)
		}

		// 评价相关
		reviews := v1.Group("/reviews")
		{
			reviews.GET("/book/:book_id", r.reviewHandler.ListReviews)
			reviews.GET("/book/:book_id/stats", r.reviewHandler.GetReviewStats)

			// 发表评价需要认证
			reviews.POST("", middleware.JWTAuth(&r.cfg.JWT), r.reviewHandler.CreateReview)
		}
	}

	return r.engine
}
