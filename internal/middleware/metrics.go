package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"modern-micro-services/internal/metrics"
)

// PrometheusMiddleware 创建一个 Prometheus 指标收集中间件
func PrometheusMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 处理请求
		c.Next()

		// 记录指标
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())

		metrics.HTTPRequestsTotal.WithLabelValues(
			serviceName,
			c.Request.Method,
			c.FullPath(),
			status,
		).Inc()

		metrics.HTTPRequestDuration.WithLabelValues(
			serviceName,
			c.Request.Method,
			c.FullPath(),
		).Observe(duration)
	}
}
