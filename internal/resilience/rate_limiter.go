package resilience

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"modern-micro-services/internal/metrics"
)

// RateLimiterConfig 限流器配置
type RateLimiterConfig struct {
	// Rate 每秒产生的令牌数（QPS 上限）
	Rate float64
	// Burst 桶的最大容量（允许的突发量）
	Burst int
}

// GRPCRateLimiter gRPC 限流器，基于令牌桶算法
type GRPCRateLimiter struct {
	limiter *rate.Limiter
	config  RateLimiterConfig
}

// NewGRPCRateLimiter 创建 gRPC 限流器
func NewGRPCRateLimiter(cfg RateLimiterConfig) *GRPCRateLimiter {
	return &GRPCRateLimiter{
		limiter: rate.NewLimiter(rate.Limit(cfg.Rate), cfg.Burst),
		config:  cfg,
	}
}

// Allow 检查是否允许通过
func (rl *GRPCRateLimiter) Allow() bool {
	return rl.limiter.Allow()
}

// RateLimiterInterceptor 返回限流 gRPC 拦截器
// 当请求超过令牌桶容量时，返回 ResourceExhausted 错误
func RateLimiterInterceptor(rl *GRPCRateLimiter) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if !rl.Allow() {
			// 记录限流指标
			metrics.RecordRateLimitRejection(ctx, "grpc", method)
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ==================== per-IP HTTP 限流 ====================

// IPRateLimiter 基于客户端 IP 的 HTTP 限流器
type IPRateLimiter struct {
	limiters sync.Map // map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

// NewIPRateLimiter 创建 per-IP 限流器
func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		rate:  rate.Limit(rps),
		burst: burst,
	}
}

// GetLimiter 获取指定 IP 的限流器（不存在则创建）
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	if limiter, ok := i.limiters.Load(ip); ok {
		return limiter.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(i.rate, i.burst)
	i.limiters.Store(ip, limiter)

	// 定期清理不活跃的限流器（防止内存泄漏）
	go func() {
		time.Sleep(5 * time.Minute)
		i.limiters.Delete(ip)
	}()

	return limiter
}
