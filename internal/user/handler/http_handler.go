package handler

import (
	"net/http"
	"strings"

	"modern-micro-services/internal/user/config"
	"modern-micro-services/internal/user/model"
	"modern-micro-services/internal/user/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type HTTPHandler struct {
	userSvc service.UserService
	jwtCfg  *config.JWTConfig
}

func NewHTTPHandler(userSvc service.UserService, jwtCfg *config.JWTConfig) *HTTPHandler {
	return &HTTPHandler{userSvc: userSvc, jwtCfg: jwtCfg}
}

// Register 用户注册
func (h *HTTPHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	result, err := h.userSvc.Register(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}

// Login 用户登录
func (h *HTTPHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	result, err := h.userSvc.Login(&req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}

// GetProfile 获取用户信息
func (h *HTTPHandler) GetProfile(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	user, err := h.userSvc.GetProfile(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": user})
}

// UpdateProfile 更新用户信息
func (h *HTTPHandler) UpdateProfile(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	user, err := h.userSvc.UpdateProfile(userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": user})
}

// JWTAuth JWT 认证中间件
func (h *HTTPHandler) JWTAuth() gin.HandlerFunc {
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

		token, err := jwt.ParseWithClaims(parts[1], &service.Claims{}, func(token *jwt.Token) (any, error) {
			return []byte(h.jwtCfg.Secret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*service.Claims)
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
