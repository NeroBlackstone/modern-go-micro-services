package handler

import (
	"net/http"
	"strconv"
	"strings"

	"modern-micro-services/internal/order/config"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/repository"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type AdminHandler struct {
	orderRepo repository.OrderRepository
	jwtCfg    *config.JWTConfig
	logger    *zap.Logger
}

func NewAdminHandler(orderRepo repository.OrderRepository, jwtCfg *config.JWTConfig, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		orderRepo: orderRepo,
		jwtCfg:    jwtCfg,
		logger:    logger,
	}
}

// OathkeeperClaims Oathkeeper JWT 变换器生成的 Claims
type OathkeeperClaims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTAuth JWT 认证中间件（Oathkeeper 集成版）
func (h *AdminHandler) JWTAuth() gin.HandlerFunc {
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

		token, err := jwt.ParseWithClaims(parts[1], &OathkeeperClaims{}, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
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

		if claims.Sub != "" {
			if id, err := strconv.ParseUint(claims.Sub, 10, 32); err == nil {
				c.Set("user_id", uint(id))
			} else {
				c.Set("user_id", uint(1))
			}
		}
		c.Set("email", claims.Email)
		c.Set("username", claims.Username)
		c.Set("kratos_id", claims.Sub)

		c.Next()
	}
}

// UpdateOrderStatus 更新订单状态（管理员操作）
func (h *AdminHandler) UpdateOrderStatus(c *gin.Context) {
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

	// 查询订单是否存在
	order, err := h.orderRepo.FindByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "order not found"})
		return
	}

	// 验证状态转换合法性
	if !isValidStatusTransition(order.Status, req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid status transition from " + string(order.Status) + " to " + string(req.Status),
		})
		return
	}

	if err := h.orderRepo.UpdateStatus(uint(id), req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	h.logger.Info("order status updated by admin",
		zap.Uint("order_id", uint(id)),
		zap.String("from", string(order.Status)),
		zap.String("to", string(req.Status)),
		zap.String("admin", c.GetString("kratos_id")),
	)

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "order status updated"})
}

func isValidStatusTransition(from, to model.OrderStatus) bool {
	transitions := map[model.OrderStatus][]model.OrderStatus{
		model.OrderStatusPending:   {model.OrderStatusPaid, model.OrderStatusCancelled},
		model.OrderStatusPaid:      {model.OrderStatusShipped, model.OrderStatusCancelled},
		model.OrderStatusShipped:   {model.OrderStatusCompleted},
		model.OrderStatusCompleted: {},
		model.OrderStatusCancelled: {},
	}
	allowed, exists := transitions[from]
	if !exists {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
