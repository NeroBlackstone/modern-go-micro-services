package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// InitMeterProvider 初始化 OTel MeterProvider，使用 Prometheus exporter 导出指标
// 返回 shutdown 函数用于优雅关闭
func InitMeterProvider(serviceName, environment string) (func(context.Context) error, error) {
	// 创建 Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// 创建 resource
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		semconv.DeploymentEnvironmentNameKey.String(environment),
	)

	// 创建 MeterProvider
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}
