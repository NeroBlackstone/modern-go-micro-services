package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// InitTracing 初始化 OpenTelemetry 并连接追踪后端（通过 OTLP HTTP）
// 支持 Tempo、Jaeger 等兼容 OTLP 协议的后端，endpoint 格式如 "tempo:4318"
// 返回 Tracer 实例和 shutdown 函数（服务退出时调用）
func InitTracing(serviceName, endpoint string) (trace.Tracer, func(context.Context) error) {
	// 创建 OTLP HTTP exporter
	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		// 追踪后端不可用时降级为 noop tracer，不影响业务
		otel.SetTracerProvider(noop.NewTracerProvider())
		return otel.Tracer(serviceName), func(ctx context.Context) error { return nil }
	}

	// 定义服务资源信息
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		attribute.String("environment", "development"),
	)

	// 创建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局 Context Propagator（用于 HTTP header 和 gRPC metadata 传播）
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context
		propagation.Baggage{},     // W3C Baggage
	))

	tracer := tp.Tracer(serviceName)

	// 返回 shutdown 函数，服务退出时刷新并关闭 exporter
	shutdown := func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}

	return tracer, shutdown
}
