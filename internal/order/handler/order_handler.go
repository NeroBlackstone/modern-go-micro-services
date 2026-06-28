package handler

import (
	"net/http"
	"strconv"
	"strings"

	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type OrderHandler struct {
	orderSvc  service.OrderService
	reviewSvc service.ReviewService
	jwtCfg    *config.JWTConfig
}

func NewOrderHandler(orderSvc service.OrderService, reviewSvc service.ReviewService, jwtCfg *config.JWTConfig) *OrderHandler {
	return &OrderHandler{
		orderSvc:  orderSvc,
		reviewSvc: reviewSvc,
		jwtCfg:    jwtCfg,
	}
}

// JWTAuth JWT 认证中间件
func (h *OrderHandler) JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "missing authorization header"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid authorization format"})
			c.Abort()
			return
		}

		type Claims struct {
			UserID uint   `json:"user_id"`
			Email  string `json:"email"`
			jwt.RegisteredClaims
		}

		token, err := jwt.ParseWithClaims(parts[1], &Claims{}, func(token *jwt.Token) (any, error) {
			return []byte(h.jwtCfg.Secret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid token claims"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}

// CreateOrder 创建订单
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req model.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	order, err := h.orderSvc.Create(userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": order})
}

// GetOrder 获取订单详情
func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid order id"})
		return
	}

	order, err := h.orderSvc.GetByID(uint(id), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "order not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": order})
}

// ListOrders 订单列表
func (h *OrderHandler) ListOrders(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var query model.OrderQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	orders, total, err := h.orderSvc.ListByUserID(userID, &query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"list":      orders,
			"total":     total,
			"page":      query.GetPage(),
			"page_size": query.GetPageSize(),
		},
	})
}

// UpdateOrderStatus 更新订单状态
func (h *OrderHandler) UpdateOrderStatus(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid order id"})
		return
	}

	var req struct {
		Status model.OrderStatus `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	if err := h.orderSvc.UpdateStatus(uint(id), userID, req.Status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "order status updated"})
}

// ========== Review Handlers ==========

func (h *OrderHandler) CreateReview(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req model.CreateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	review, err := h.reviewSvc.Create(userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": review})
}

func (h *OrderHandler) ListReviews(c *gin.Context) {
	bookID, err := strconv.ParseUint(c.Param("book_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid book id"})
		return
	}

	var query model.ReviewQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	reviews, total, err := h.reviewSvc.ListByBookID(uint(bookID), &query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"list":      reviews,
			"total":     total,
			"page":      query.GetPage(),
			"page_size": query.GetPageSize(),
		},
	})
}

func (h *OrderHandler) GetReviewStats(c *gin.Context) {
	bookID, err := strconv.ParseUint(c.Param("book_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid book id"})
		return
	}

	stats, err := h.reviewSvc.GetStats(uint(bookID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": stats})
}
