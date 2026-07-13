package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor 返回 gRPC 服务端一元拦截器，自动创建 Span
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		// 从 gRPC metadata 中提取 trace context
		md, _ := metadata.FromIncomingContext(ctx)
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, metadataCarrier(md))

		// 获取 tracer 并创建 server span
		tracer := otel.Tracer("grpc-server")
		spanName := info.FullMethod
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		// 调用实际 handler
		resp, err := handler(ctx, req)

		// 记录状态
		duration := time.Since(start)
		span.SetAttributes(attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())))

		if err != nil {
			st, _ := status.FromError(err)
			span.SetStatus(codes.Error, st.Message())
			span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(st.Code())))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return resp, err
	}
}

// UnaryClientInterceptor 返回 gRPC 客户端一元拦截器，自动注入 trace context
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()

		// 获取 tracer 并创建 client span
		tracer := otel.Tracer("grpc-client")
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.target", cc.Target()),
			),
		)
		defer span.End()

		// 将 trace context 注入到 gRPC metadata 中
		propagator := otel.GetTextMapPropagator()
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		propagator.Inject(ctx, metadataCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		// 调用实际 RPC
		err := invoker(ctx, method, req, reply, cc, opts...)

		// 记录状态
		duration := time.Since(start)
		span.SetAttributes(attribute.Float64("rpc.duration_ms", float64(duration.Milliseconds())))

		if err != nil {
			st, _ := status.FromError(err)
			span.SetStatus(codes.Error, st.Message())
			span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(st.Code())))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return err
	}
}

// metadataCarrier 适配 propagation.TextMapCarrier 接口，用于 gRPC metadata 传播
type metadataCarrier metadata.MD

// Get 从 metadata 中获取值
func (c metadataCarrier) Get(key string) string {
	md := metadata.MD(c)
	vals := md.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// Set 向 metadata 中设置值
func (c metadataCarrier) Set(key, value string) {
	md := metadata.MD(c)
	md.Set(key, value)
}

// Keys 返回 metadata 中所有的 key
func (c metadataCarrier) Keys() []string {
	md := metadata.MD(c)
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	return keys
}
