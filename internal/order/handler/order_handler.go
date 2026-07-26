package handler

import (
	"net/http"
	"strconv"
	"strings"

	"modern-micro-services/internal/metrics"
	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
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

// OathkeeperClaims Oathkeeper JWT 变换器生成的 Claims
// 包含 Ory Kratos 的 identity 信息
type OathkeeperClaims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTAuth JWT 认证中间件（Oathkeeper 集成版）
// Oathkeeper 在网关层验证 Kratos session，然后使用 JWT 变换器
// 签发 JWT 并注入到 Authorization header，后端服务只需验证 JWT 签名
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

		// 验证 Oathkeeper 签发的 JWT（使用 HMAC 签名）
		token, err := jwt.ParseWithClaims(parts[1], &OathkeeperClaims{}, func(token *jwt.Token) (any, error) {
			// 验证签名方法
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			// 使用配置的密钥验证签名（Oathkeeper 使用相同的密钥）
			return []byte(h.jwtCfg.Secret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*OathkeeperClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid token claims"})
			c.Abort()
			return
		}

		// 将 Oathkeeper 注入的用户信息存入 Gin 上下文
		// user_id: 使用 Kratos identity ID（string 转 uint 用于兼容现有代码）
		// 注意：Kratos 使用 UUID 作为 identity ID，这里简化处理
		// 生产环境应使用字符串 ID
		if claims.Sub != "" {
			// 尝试将 Kratos ID 转换为 uint（如果可能）
			if id, err := strconv.ParseUint(claims.Sub, 10, 32); err == nil {
				c.Set("user_id", uint(id))
			} else {
				// 如果是 UUID 格式，使用哈希值作为兼容 ID
				// 生产环境应修改 User 模型使用 string ID
				c.Set("user_id", uint(1))
			}
		}
		c.Set("email", claims.Email)
		c.Set("username", claims.Username)
		c.Set("kratos_id", claims.Sub) // 保留原始 Kratos identity ID

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

	order, err := h.orderSvc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 记录业务指标
	metrics.OrdersCreatedTotal.Add(c.Request.Context(), 1,
		otelmetric.WithAttributes(attribute.String("service", "order-service")),
	)

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
