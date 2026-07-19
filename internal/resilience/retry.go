package resilience

import (
	"context"
	"math"
	"math/rand"
	"slices"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"modern-micro-services/internal/metrics"
)

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxRetries 最大重试次数（不含首次调用）
	MaxRetries int
	// InitialBackoff 初始退避时间
	InitialBackoff time.Duration
	// MaxBackoff 最大退避时间上限
	MaxBackoff time.Duration
	// BackoffMultiplier 退避倍数
	BackoffMultiplier float64
	// RetryableStatuses 可重试的 gRPC 状态码
	RetryableStatuses []codes.Code
	// Logger 日志记录器
	Logger *zap.Logger
}

// DefaultRetryConfig 返回默认重试配置
func DefaultRetryConfig(logger *zap.Logger) RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        2 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableStatuses: []codes.Code{
			codes.Unavailable,
			codes.DeadlineExceeded,
			codes.ResourceExhausted,
			codes.Aborted,
		},
		Logger: logger,
	}
}

// isRetryable 检查错误是否可重试
func (cfg RetryConfig) isRetryable(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return slices.Contains(cfg.RetryableStatuses, st.Code())
}

// calculateBackoff 计算第 attempt 次重试的退避时间（含 jitter）
func (cfg RetryConfig) calculateBackoff(attempt int) time.Duration {
	// 指数退避：initialBackoff * multiplier^attempt
	backoff := float64(cfg.InitialBackoff) * math.Pow(cfg.BackoffMultiplier, float64(attempt))

	// 限制最大退避
	if backoff > float64(cfg.MaxBackoff) {
		backoff = float64(cfg.MaxBackoff)
	}

	// 添加 jitter：[0, backoff*0.1]
	jitter := rand.Float64() * backoff * 0.1
	return time.Duration(backoff + jitter)
}

// RetryInterceptor 返回重试 gRPC 拦截器
// 对可重试的错误进行指数退避重试
func RetryInterceptor(cfg RetryConfig) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var lastErr error

		for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
			// 如果不是第一次调用，等待退避时间
			if attempt > 0 {
				backoff := cfg.calculateBackoff(attempt - 1)
				if cfg.Logger != nil {
					cfg.Logger.Debug("retrying gRPC call",
						zap.String("method", method),
						zap.Int("attempt", attempt),
						zap.Duration("backoff", backoff),
						zap.Error(lastErr),
					)
				}

				// 记录重试指标
				metrics.RecordRetryAttempt(ctx, method, attempt)

				// 等待退避时间，但尊重 context 取消
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
			}

			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			if lastErr == nil {
				return nil
			}

			// 如果错误不可重试，直接返回
			if !cfg.isRetryable(lastErr) {
				return lastErr
			}
		}

		return lastErr
	}
}
