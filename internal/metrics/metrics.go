package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// 全局 metric instruments
var (
	HTTPRequestsTotal    otelmetric.Int64Counter
	HTTPRequestDuration  otelmetric.Float64Histogram
	GRPCRequestsTotal    otelmetric.Int64Counter
	UsersCreatedTotal    otelmetric.Int64Counter
	OrdersCreatedTotal   otelmetric.Int64Counter
	BooksViewedTotal     otelmetric.Int64Counter
	DBQueryDuration      otelmetric.Float64Histogram
	RedisOperationsTotal otelmetric.Int64Counter
	RabbitMQMessagesTotal otelmetric.Int64Counter

	// 容错指标
	CircuitBreakerStateChanges otelmetric.Int64Counter
	RetryAttempts              otelmetric.Int64Counter
	RateLimitRejections        otelmetric.Int64Counter
)

// InitMetrics 初始化所有 metric instruments
// 需要在 MeterProvider 初始化之后调用
func InitMetrics() error {
	meter := otel.Meter("modern-micro-services")
	var err error

	// HTTP 指标
	HTTPRequestsTotal, err = meter.Int64Counter(
		"http_requests_total",
		otelmetric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return err
	}

	HTTPRequestDuration, err = meter.Float64Histogram(
		"http_request_duration_seconds",
		otelmetric.WithDescription("Duration of HTTP requests in seconds"),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	// gRPC 指标
	GRPCRequestsTotal, err = meter.Int64Counter(
		"grpc_requests_total",
		otelmetric.WithDescription("Total number of gRPC requests"),
	)
	if err != nil {
		return err
	}

	// 业务指标
	UsersCreatedTotal, err = meter.Int64Counter(
		"users_created_total",
		otelmetric.WithDescription("Total number of users created"),
	)
	if err != nil {
		return err
	}

	OrdersCreatedTotal, err = meter.Int64Counter(
		"orders_created_total",
		otelmetric.WithDescription("Total number of orders created"),
	)
	if err != nil {
		return err
	}

	BooksViewedTotal, err = meter.Int64Counter(
		"books_viewed_total",
		otelmetric.WithDescription("Total number of books viewed"),
	)
	if err != nil {
		return err
	}

	// 数据库指标
	DBQueryDuration, err = meter.Float64Histogram(
		"db_query_duration_seconds",
		otelmetric.WithDescription("Duration of database queries in seconds"),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	// Redis 指标
	RedisOperationsTotal, err = meter.Int64Counter(
		"redis_operations_total",
		otelmetric.WithDescription("Total number of Redis operations"),
	)
	if err != nil {
		return err
	}

	// RabbitMQ 指标
	RabbitMQMessagesTotal, err = meter.Int64Counter(
		"rabbitmq_messages_total",
		otelmetric.WithDescription("Total number of RabbitMQ messages"),
	)
	if err != nil {
		return err
	}

	// 容错指标
	CircuitBreakerStateChanges, err = meter.Int64Counter(
		"circuit_breaker_state_changes_total",
		otelmetric.WithDescription("Total number of circuit breaker state changes"),
	)
	if err != nil {
		return err
	}

	RetryAttempts, err = meter.Int64Counter(
		"retry_attempts_total",
		otelmetric.WithDescription("Total number of retry attempts"),
	)
	if err != nil {
		return err
	}

	RateLimitRejections, err = meter.Int64Counter(
		"rate_limit_rejected_total",
		otelmetric.WithDescription("Total number of rate limit rejections"),
	)
	if err != nil {
		return err
	}

	return nil
}

// HTTPAttributes 返回 HTTP 请求常用的 attribute 组合
func HTTPAttributes(service, method, endpoint, status string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("service", service),
		attribute.String("method", method),
		attribute.String("endpoint", endpoint),
		attribute.String("status", status),
	}
}

// GRPCAttributes 返回 gRPC 请求常用的 attribute 组合
func GRPCAttributes(service, method, code string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("service", service),
		attribute.String("method", method),
		attribute.String("code", code),
	}
}

// RecordHTTPRequest 记录 HTTP 请求指标
func RecordHTTPRequest(ctx context.Context, service, method, endpoint, status string, duration float64) {
	attrs := otelmetric.WithAttributes(HTTPAttributes(service, method, endpoint, status)...)
	HTTPRequestsTotal.Add(ctx, 1, attrs)
	HTTPRequestDuration.Record(ctx, duration, attrs)
}

// RecordGRPCRequest 记录 gRPC 请求指标
func RecordGRPCRequest(ctx context.Context, service, method, code string) {
	GRPCRequestsTotal.Add(ctx, 1, otelmetric.WithAttributes(GRPCAttributes(service, method, code)...))
}

// RecordCircuitBreakerStateChange 记录熔断器状态变化
func RecordCircuitBreakerStateChange(ctx context.Context, name, from, to string) {
	CircuitBreakerStateChanges.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("breaker", name),
		attribute.String("from", from),
		attribute.String("to", to),
	))
}

// RecordRetryAttempt 记录重试尝试
func RecordRetryAttempt(ctx context.Context, method string, attempt int) {
	RetryAttempts.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("method", method),
		attribute.Int("attempt", attempt),
	))
}

// RecordRateLimitRejection 记录限流拒绝
func RecordRateLimitRejection(ctx context.Context, layer, method string) {
	RateLimitRejections.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("layer", layer),
		attribute.String("method", method),
	))
}
