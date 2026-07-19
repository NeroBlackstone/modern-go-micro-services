package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"modern-micro-services/internal/resilience"
)

// RateLimit 创建基于客户端 IP 的限流 Gin 中间件
// rps: 每秒允许的请求数
// burst: 允许的突发请求数
func RateLimit(rps float64, burst int) gin.HandlerFunc {
	limiter := resilience.NewIPRateLimiter(rps, burst)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}

		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			// 设置 Retry-After header，告诉客户端多久后重试
			retryAfter := time.Second / time.Duration(rps)
			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "rate limit exceeded, please retry later",
			})
			return
		}

		c.Next()
	}
}
