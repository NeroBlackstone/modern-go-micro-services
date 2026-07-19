package resilience

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sony/gobreaker/v2"

	"modern-micro-services/internal/metrics"
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	// Name 熔断器名称，用于日志和指标标识
	Name string
	// MaxRequests Half-Open 状态下允许通过的最大请求数
	MaxRequests uint32
	// Interval Closed 状态下清零计数器的间隔
	Interval time.Duration
	// Timeout Open 状态持续时间，超时后进入 Half-Open
	Timeout time.Duration
	// ReadyToTrip 判断是否触发熔断的函数
	ReadyToTrip func(counts gobreaker.Counts) bool
	// OnStateChange 状态变化回调
	OnStateChange func(name string, from, to gobreaker.State)
}

// DefaultCircuitBreakerConfig 返回默认的熔断器配置
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:        name,
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 连续失败 5 次触发熔断
			return counts.ConsecutiveFailures > 5
		},
	}
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(cfg CircuitBreakerConfig, logger *zap.Logger) *gobreaker.CircuitBreaker[any] {
	onStateChange := cfg.OnStateChange
	if onStateChange == nil {
		onStateChange = func(name string, from, to gobreaker.State) {
			logger.Info("circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		}
	}

	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: cfg.ReadyToTrip,
		OnStateChange: func(name string, from, to gobreaker.State) {
			// 记录 Prometheus 指标
			metrics.RecordCircuitBreakerStateChange(context.Background(), cfg.Name, from.String(), to.String())
			// 用户回调
			onStateChange(name, from, to)
		},
	})
}

// CircuitBreakerInterceptor 返回熔断器 gRPC 拦截器
// 当熔断器处于 Open 状态时，请求直接返回 ErrCircuitOpen，不发起 RPC 调用
func CircuitBreakerInterceptor(cb *gobreaker.CircuitBreaker[any]) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 通过熔断器执行调用
		_, err := cb.Execute(func() (any, error) {
			// 如果熔断器 Open，这里不会被执行
			innerErr := invoker(ctx, method, req, reply, cc, opts...)
			return nil, innerErr
		})

		if err != nil {
			// 如果是熔断器打开错误，包装为 gRPC Unavailable
			if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
				return status.Error(codes.Unavailable, "circuit breaker is open: "+err.Error())
			}
			return err
		}
		return nil
	}
}
