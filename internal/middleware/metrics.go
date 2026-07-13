package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"modern-micro-services/internal/metrics"
)

// MetricsMiddleware 创建一个 OTel 指标收集中间件
func MetricsMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 处理请求
		c.Next()

		// 记录指标
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())

		metrics.RecordHTTPRequest(
			c.Request.Context(),
			serviceName,
			c.Request.Method,
			c.FullPath(),
			status,
			duration,
		)
	}
}
