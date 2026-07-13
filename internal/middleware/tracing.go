package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Tracing Gin 链路追踪中间件
// 从 HTTP 请求头中提取 trace context，创建当前请求的 span
func Tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头提取 trace context（支持 W3C Traceparent）
		propagator := otel.GetTextMapPropagator()
		ctx := propagator.Extract(c.Request.Context(), HeaderCarrier(c.Request.Header))

		// 创建 HTTP server span
		tracer := otel.Tracer("http-server")
		spanName := c.Request.Method + " " + c.FullPath()
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", c.Request.Method),
				attribute.String("http.url", c.Request.URL.String()),
				attribute.String("http.target", c.Request.URL.Path),
				attribute.String("http.host", c.Request.Host),
				attribute.String("http.scheme", func() string {
					if c.Request.TLS != nil {
						return "https"
					}
					return "http"
				}()),
				attribute.String("http.user_agent", c.Request.UserAgent()),
				attribute.String("http.remote_addr", c.ClientIP()),
			),
		)
		defer span.End()

		// 将带 trace 的 context 注入到请求中
		c.Request = c.Request.WithContext(ctx)

		// 处理请求
		c.Next()

		// 记录响应状态
		span.SetAttributes(
			attribute.Int("http.status_code", c.Writer.Status()),
		)
		if c.Writer.Status() >= 400 {
			span.SetAttributes(attribute.String("http.error", c.Errors.String()))
		}
	}
}

// HeaderCarrier 适配 propagation.TextMapCarrier 接口，用于 HTTP header 传播
type HeaderCarrier map[string][]string

func (h HeaderCarrier) Get(key string) string {
	vals := h[key]
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (h HeaderCarrier) Set(key, value string) {
	h[key] = []string{value}
}

func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}
