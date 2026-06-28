# Day 5 - gRPC Resolver 机制

## gRPC 的服务发现架构

gRPC 通过 **Name Resolver** 和 **Balancer** 两个组件实现服务发现和负载均衡：

```
gRPC Client
    │
    ├─ Name Resolver ──── 解析服务名 → 地址列表
    │   (consul:///user-service)
    │
    └─ Balancer ──── 从地址列表中选择一个
        (round-robin / pick-first)
            │
            ▼
        book-service:9092
```

## Resolver 接口

```go
// resolver.Builder 负责创建 Resolver
type Builder interface {
    // Scheme 返回此 resolver 支持的 URL scheme
    // 例如: "consul", "dns", "etcd"
    Scheme() string

    // Build 根据 target 创建 Resolver
    // target 格式: scheme:///endpoint
    // 例如: consul:///book-service
    Build(target Target, cc ClientConn, opts BuildOptions) (Resolver, error)
}

// Resolver 负责解析服务名并更新地址列表
type Resolver interface {
    // ResolveNow 立即触发解析
    ResolveNow(ResolveNowOptions)

    // Close 关闭 resolver
    Close()
}
```

## 自定义 Resolver 实现

### 步骤 1: 实现 Builder

```go
package discovery

import (
    "google.golang.org/grpc/resolver"
)

const consulScheme = "consul"

// ConsulResolverBuilder 实现 resolver.Builder
type ConsulResolverBuilder struct {
    registry *Registry  // Consul 客户端
}

func NewConsulResolverBuilder(registry *Registry) *ConsulResolverBuilder {
    return &ConsulResolverBuilder{registry: registry}
}

// Scheme 返回支持的 URL scheme
func (b *ConsulResolverBuilder) Scheme() string {
    return consulScheme
}

// Build 创建 resolver
func (b *ConsulResolverBuilder) Build(
    target resolver.Target,
    cc resolver.ClientConn,
    opts resolver.BuildOptions,
) (resolver.Resolver, error) {
    // target.Endpoint() = "book-service" (从 consul:///book-service 解析)
    serviceName := target.Endpoint()

    r := &consulResolver{
        serviceName: serviceName,
        registry:    b.registry,
        cc:          cc,
    }

    // 立即解析一次
    r.resolve()

    // 启动定期刷新
    go r.watch()

    return r, nil
}
```

### 步骤 2: 实现 Resolver

```go
// consulResolver 实现 resolver.Resolver
type consulResolver struct {
    serviceName string
    registry    *Registry
    cc          resolver.ClientConn
}

// resolve 从 Consul 查询服务并更新 gRPC
func (r *consulResolver) resolve() {
    // 1. 查询 Consul 获取健康实例
    instances, err := r.registry.Discover(r.serviceName)
    if err != nil {
        return
    }

    // 2. 转换为 gRPC Address 格式
    addrs := make([]resolver.Address, len(instances))
    for i, inst := range instances {
        addrs[i] = resolver.Address{
            Addr: fmt.Sprintf("%s:%d", inst.Address, inst.Port),
        }
    }

    // 3. 更新 gRPC 连接
    r.cc.UpdateState(resolver.State{Addresses: addrs})
}

// watch 定期刷新服务列表
func (r *consulResolver) watch() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        r.resolve()
    }
}

// ResolveNow 立即触发解析
func (r *consulResolver) ResolveNow(opts resolver.ResolveNowOptions) {
    r.resolve()
}

// Close 关闭 resolver
func (r *consulResolver) Close() {
    // 停止 watch goroutine
}
```

### 步骤 3: 注册 Builder

```go
// 在 init 中注册，这样 gRPC 就能识别 consul scheme
func init() {
    resolver.Register(&ConsulResolverBuilder{})
}
```

### 步骤 4: 使用

```go
// 1. 创建 Consul 客户端
registry, _ := discovery.NewRegistry("consul:8500", logger)

// 2. 创建 resolver builder
builder := discovery.NewConsulResolverBuilder(registry)

// 3. 注册到 gRPC
resolver.Register(builder)

// 4. 使用 consul:/// 服务名 进行连接
conn, _ := grpc.NewClient(
    "consul:///book-service",  // gRPC 会用我们的 resolver 解析
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
)

// 5. 创建客户端
client := bookv1.NewBookServiceClient(conn)
```

## ClientConn 接口

Resolver 通过 `ClientConn` 通知 gRPC 更新地址列表：

```go
type ClientConn interface {
    // UpdateState 更新连接状态（包含新的地址列表）
    UpdateState(State) error

    // ReportError 报告错误
    ReportError(error)
}
```

**调用时机：**
- `resolve()` 查询到新实例后调用 `UpdateState`
- 查询失败时可以调用 `ReportError`

## 负载均衡策略

gRPC 内置两种负载均衡策略：

### pick_first（默认）

```go
grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"pick_first":{}}]}`)
```

- 只连接第一个可用的地址
- 不做负载均衡

### round_robin

```go
grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`)
```

- 轮询所有可用地址
- 均匀分配请求

## 完整流程图

```
┌─────────────────────────────────────────────────────────┐
│                    gRPC Client                          │
│                                                         │
│  1. grpc.Dial("consul:///book-service")                │
│     │                                                   │
│     ▼                                                   │
│  2. ConsulResolverBuilder.Build()                      │
│     │                                                   │
│     ▼                                                   │
│  3. consulResolver.resolve()                           │
│     │                                                   │
│     ├─→ registry.Discover("book-service")              │
│     │   │                                               │
│     │   ▼                                               │
│     │   Consul Health API                               │
│     │   │                                               │
│     │   ▼                                               │
│     │   返回: [172.18.0.3:9092, 172.18.0.4:9092]       │
│     │                                                   │
│     ▼                                                   │
│  4. cc.UpdateState([{172.18.0.3:9092}, ...])          │
│     │                                                   │
│     ▼                                                   │
│  5. Balancer (round_robin) 选择一个地址                 │
│     │                                                   │
│     ▼                                                   │
│  6. 建立 gRPC 连接                                     │
└─────────────────────────────────────────────────────────┘
```

## 调试技巧

### 1. 启用 gRPC 日志

```go
import "google.golang.org/grpc/grpclog"

// 在 main 中
grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stderr, os.Stderr, os.Stderr, 4))
```

### 2. 查看 resolver 状态

```go
// 连接建立后，查看当前连接的地址
state := conn.GetState()
fmt.Printf("Current state: %v\n", state)
```

### 3. 使用 gRPC 的 health check

```go
import "google.golang.org/grpc/health/grpc_health_v1"

healthClient := grpc_health_v1.NewHealthClient(conn)
resp, _ := healthClient.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{
    Service: "book-service",
})
fmt.Printf("Health: %v\n", resp.Status)
```

## 常见问题

### Q: 为什么 resolver 要在 init 中注册？

gRPC 使用全局的 resolver 注册表。在 init 中注册可以确保：
- 导入包时自动注册
- 避免重复注册
- 其他包可以直接使用

### Q: resolve 失败了怎么办？

```go
func (r *consulResolver) resolve() {
    instances, err := r.registry.Discover(r.serviceName)
    if err != nil {
        // 记录日志，但不中断
        log.Printf("resolve failed: %v", err)
        return
    }
    // 继续使用上次的地址列表
}
```

gRPC 会保持上一次成功的地址列表，直到下次成功解析。

### Q: 如何处理服务实例变化？

watch goroutine 每 10 秒查询一次 Consul，自动感知实例变化：
- 新实例启动 → 自动加入列表
- 实例故障 → 自动从列表移除
- 实例恢复 → 自动重新加入

## 扩展阅读

- [gRPC Load Balancing 文档](https://github.com/grpc/grpc/blob/master/doc/load-balancing.md)
- [gRPC Resolver 接口](https://pkg.go.dev/google.golang.org/grpc/resolver)
- [自定义 Resolver 示例](https://github.com/grpc/grpc-go/tree/master/examples/features/name_resolving)
