# Go 服务集成链路追踪

## 实现概览

本项目为三个微服务（user-service、book-service、order-service）接入了 OpenTelemetry + Grafana Tempo 链路追踪，实现：

1. **服务入口追踪**：HTTP Gin 中间件自动创建 Span
2. **gRPC 服务端追踪**：拦截器自动提取 trace context 并创建 Span
3. **gRPC 客户端追踪**：拦截器自动注入 trace context 并创建 Span
4. **跨服务链路传播**：通过 HTTP Header 和 gRPC Metadata 自动传播

## 代码实现

### 1. Tracing 初始化模块

**`internal/tracing/tracing.go`** — 所有服务共享的初始化代码

核心流程（详见[概念篇 02](./02-tempo-and-opentelemetry.md)）：

1. 通过 OTLP HTTP 连接 Tempo（`otlptracehttp`）
2. 定义服务资源（`resource.Merge`），用于在 Grafana 中区分服务
3. 创建 `TracerProvider`，配置批量导出 + AlwaysSample
4. 设置全局 Provider 和 Propagator

**关键设计决策：**
- **降级处理**：如果追踪后端不可用，自动降级为 noop tracer，不影响业务
- **批量导出**：使用 `WithBatcher` 而非 `Sync` exporter，避免阻塞业务请求
- **全局 Propagator**：设置一次，所有 OTel SDK 调用自动使用

### 2. gRPC 拦截器

**`internal/tracing/grpc_interceptor.go`** — gRPC 拦截器实现

#### 服务端拦截器

```go
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        // 1. 从 gRPC metadata 提取 trace context
        md, _ := metadata.FromIncomingContext(ctx)
        ctx = otel.GetTextMapPropagator().Extract(ctx, metadataCarrier(md))

        // 2. 创建 server span
        ctx, span := tracer.Start(ctx, info.FullMethod,
            trace.WithSpanKind(trace.SpanKindServer),
        )
        defer span.End()

        // 3. 调用实际 handler
        resp, err := handler(ctx, req)

        // 4. 记录状态和耗时
        span.SetAttributes(attribute.Float64("rpc.duration_ms", ...))
        if err != nil {
            span.SetStatus(codes.Error, err.Error())
        }
        return resp, err
    }
}
```

#### 客户端拦截器

```go
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
    return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        // 1. 创建 client span
        ctx, span := tracer.Start(ctx, method,
            trace.WithSpanKind(trace.SpanKindClient),
        )
        defer span.End()

        // 2. 将 trace context 注入到 gRPC metadata
        md, _ := metadata.FromOutgoingContext(ctx)
        otel.GetTextMapPropagator().Inject(ctx, metadataCarrier(md))
        ctx = metadata.NewOutgoingContext(ctx, md)

        // 3. 调用实际 RPC
        err := invoker(ctx, method, req, reply, cc, opts...)

        // 4. 记录状态
        if err != nil {
            span.SetStatus(codes.Error, err.Error())
        }
        return err
    }
}
```

#### MetadataCarrier 适配器

gRPC 使用 `metadata.MD` 传递 header，需要适配为 OTel 的 `TextMapCarrier` 接口：

```go
type metadataCarrier metadata.MD

func (c metadataCarrier) Get(key string) string {
    vals := metadata.MD(c).Get(key)
    if len(vals) > 0 { return vals[0] }
    return ""
}

func (c metadataCarrier) Set(key, value string) {
    metadata.MD(c).Set(key, value)
}
```

### 3. Gin HTTP 中间件

**`internal/middleware/tracing.go`**

```go
func Tracing() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 从 HTTP Header 提取 trace context
        ctx := propagator.Extract(c.Request.Context(), HeaderCarrier(c.Request.Header))

        // 2. 创建 HTTP server span
        ctx, span := tracer.Start(ctx, c.Request.Method + " " + c.FullPath(),
            trace.WithSpanKind(trace.SpanKindServer),
            trace.WithAttributes(
                attribute.String("http.method", c.Request.Method),
                attribute.String("http.url", c.Request.URL.String()),
            ),
        )
        defer span.End()

        // 3. 注入到请求 context
        c.Request = c.Request.WithContext(ctx)

        // 4. 处理请求
        c.Next()

        // 5. 记录响应状态
        span.SetAttributes(attribute.Int("http.status_code", c.Writer.Status()))
    }
}
```

### 4. 服务入口集成

在每个服务的 `main.go` 中初始化 tracing：

```go
// book-service / user-service / order-service
_, tracingShutdown := tracing.InitTracing("service-name", cfg.Tracing.Endpoint)
defer tracingShutdown(context.Background())
```

gRPC server 注册拦截器：

```go
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(tracing.UnaryServerInterceptor()),
)
```

gRPC client 注册拦截器：

```go
bookConn, err := grpc.NewClient(
    "consul:///book-service",
    grpc.WithUnaryInterceptor(tracing.UnaryClientInterceptor()),
)
```

HTTP server 注册中间件：

```go
r.Use(middleware.Tracing())
```

## 链路传播流程示例

以"创建订单"请求为例，完整的 trace 传播流程：

```
1. 用户发送 HTTP POST /api/v1/orders
   Header: (无 traceparent — 第一个请求)

2. order-service Gin 中间件
   → 提取 Header（无）→ 创建 root Span → Trace ID: abc-123
   → Span ID: span-1, Operation: POST /api/v1/orders

3. order-service 调用 book-service.GetBooks (gRPC)
   → Client 拦截器注入 traceparent 到 gRPC Metadata
   → Metadata: traceparent: 00-abc-123-span-2-01

4. book-service gRPC 拦截器
   → 从 Metadata 提取 traceparent → 创建 child Span
   → Span ID: span-2, Parent: span-1, Operation: /bookstore.book.v1.BookService/GetBooks

5. book-service 返回响应
   → gRPC Metadata 携带 traceparent 返回

6. order-service 继续处理
   → Span span-1 结束
```

在 Grafana Tempo 中看到的链路图：

```
POST /api/v1/orders (200ms)          ← order-service
├── /bookstore.book.v1.BookService/GetBooks (50ms)   ← book-service
├── /bookstore.book.v1.BookService/DeductStock (30ms) ← book-service
└── INSERT INTO orders (5ms)          ← PostgreSQL (如果 instrumented)
```

## 配置

每个服务的 YAML 配置中添加追踪端点（OTLP HTTP 格式）：

```yaml
tracing:
  endpoint: "tempo:4318"
```

Docker Compose 中添加 Tempo 容器：

```yaml
tempo:
  image: grafana/tempo:latest
  ports:
    - "3200:3200"   # HTTP API
    - "4318:4318"   # OTLP HTTP（接收 traces）
  volumes:
    - ./configs/tempo.yml:/etc/tempo/tempo.yml
  command: -config.file=/etc/tempo/tempo.yml
```

## 验证

1. 启动服务：`docker compose up --build`
2. 发送请求：`curl -X POST http://localhost/api/v1/orders ...`
3. 打开 Grafana：`http://localhost:3000` → Explore → 选择 Tempo 数据源
4. 搜索 `order-service` → 查看 trace → 确认能看到跨服务调用链
