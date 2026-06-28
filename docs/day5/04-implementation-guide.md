# Day 5 - 实现指南：Consul 服务发现

## 项目结构

```
internal/discovery/
├── registry.go      # 服务注册/注销/发现
├── resolver.go      # gRPC 自定义 resolver
└── balancer.go      # 负载均衡器
```

## 实现步骤

### Step 1: Docker Compose 添加 Consul

```yaml
# docker-compose.yml
consul:
  image: hashicorp/consul:2.0.1
  container_name: bookstore-consul
  ports:
    - "8500:8500"   # Web UI + HTTP API
    - "8600:8600/udp" # DNS
  command: agent -dev -client=0.0.0.0
  healthcheck:
    test: ["CMD", "consul", "members"]
    interval: 10s
    timeout: 5s
    retries: 5
```

### Step 2: 创建 Registry

```go
// internal/discovery/registry.go

type Registry struct {
    client *api.Client
    logger *zap.Logger
}

func NewRegistry(consulAddr string, logger *zap.Logger) (*Registry, error) {
    config := api.DefaultConfig()
    config.Address = consulAddr
    client, err := api.NewClient(config)
    return &Registry{client: client, logger: logger}, nil
}

func (r *Registry) Register(reg *ServiceRegistration, checkInterval time.Duration) error {
    hostname, _ := os.Hostname()
    serviceID := fmt.Sprintf("%s-%s-%d", reg.ServiceName, hostname, reg.Port)

    registration := &api.AgentServiceRegistration{
        ID:      serviceID,
        Name:    reg.ServiceName,
        Address: reg.Address,
        Port:    reg.Port,
        Tags:    reg.Tags,
        Check: &api.AgentServiceCheck{
            TCP:                           fmt.Sprintf("%s:%d", reg.Address, reg.Port),
            Interval:                      checkInterval.String(),
            DeregisterCriticalServiceAfter: "30s",
            Timeout:                       "5s",
        },
    }

    return r.client.Agent().ServiceRegister(registration)
}

func (r *Registry) Deregister(serviceID string) error {
    return r.client.Agent().ServiceDeregister(serviceID)
}

func (r *Registry) Discover(serviceName string) ([]*ServiceInstance, error) {
    entries, _, err := r.client.Health().Service(serviceName, "", true, nil)
    // 转换为 ServiceInstance 列表
    return instances, nil
}
```

### Step 3: 创建 gRPC Resolver

```go
// internal/discovery/resolver.go

const consulScheme = "consul"

type ConsulResolverBuilder struct {
    registry *Registry
    logger   *zap.Logger
}

func (b *ConsulResolverBuilder) Scheme() string {
    return consulScheme
}

func (b *ConsulResolverBuilder) Build(
    target resolver.Target,
    cc resolver.ClientConn,
    opts resolver.BuildOptions,
) (resolver.Resolver, error) {
    serviceName := target.Endpoint()
    r := &consulResolver{
        serviceName: serviceName,
        registry:    b.registry,
        cc:          cc,
        logger:      b.logger,
        stopCh:      make(chan struct{}),
    }
    r.resolve()
    go r.watch()
    return r, nil
}

type consulResolver struct {
    serviceName string
    registry    *Registry
    cc          resolver.ClientConn
    logger      *zap.Logger
    stopCh      chan struct{}
}

func (r *consulResolver) resolve() {
    instances, err := r.registry.Discover(r.serviceName)
    if err != nil {
        r.logger.Warn("resolve failed", zap.Error(err))
        return
    }

    addrs := make([]resolver.Address, len(instances))
    for i, inst := range instances {
        addrs[i] = resolver.Address{
            Addr: fmt.Sprintf("%s:%d", inst.Address, inst.Port),
        }
    }
    r.cc.UpdateState(resolver.State{Addresses: addrs})
}

func (r *consulResolver) watch() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            r.resolve()
        case <-r.stopCh:
            return
        }
    }
}

func (r *consulResolver) ResolveNow(opts resolver.ResolveNowOptions) {
    r.resolve()
}

func (r *consulResolver) Close() {
    close(r.stopCh)
}
```

### Step 4: 配置文件

```yaml
# configs/order-service.yaml
consul:
  addr: consul:8500

# 删除原来的硬编码地址
# grpc:
#   user_service_addr: user-service:9091
#   book_service_addr: book-service:9092
```

### Step 5: 修改各服务入口

**user-service (服务注册):**

```go
// cmd/user-service/main.go

// 启动 gRPC server 后注册到 Consul
registry, _ := discovery.NewRegistry(cfg.Consul.Addr, logger)

hostname, _ := os.Hostname()
registry.Register(&discovery.ServiceRegistration{
    ServiceName: "user-service",
    Address:     hostname,
    Port:        cfg.Server.GRPCPort,
    Tags:        []string{"grpc", "user"},
}, 10*time.Second)

// 优雅退出时注销
<-quit
registry.Deregister(fmt.Sprintf("user-service-%s-%d", hostname, cfg.Server.GRPCPort))
grpcServer.Stop()
```

**order-service (服务发现):**

```go
// cmd/order-service/main.go

// 创建 Consul resolver
registry, _ := discovery.NewRegistry(cfg.Consul.Addr, logger)
consulBuilder := discovery.NewConsulResolverBuilder(registry, logger)
resolver.Register(consulBuilder)

// 使用 consul:/// 服务名 连接
bookConn, _ := grpc.NewClient(
    "consul:///book-service",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithDefaultServiceConfig(discovery.ServiceConfigJSON()),
)
bookClient := client.NewBookClientFromConn(bookConn, logger)
```

## 验证步骤

### 1. 启动服务

```bash
docker-compose up -d
```

### 2. 查看 Consul UI

访问 http://localhost:8500

你应该看到：
- user-service (passing)
- book-service (passing)

### 3. 测试 API

```bash
# 健康检查
curl http://localhost:8080/health

# 查看 Consul 中的服务
curl http://localhost:8500/v1/catalog/services
```

### 4. 测试故障摘除

```bash
# 停止 book-service
docker-compose stop book-service

# 等待 30 秒后，查看 Consul UI
# book-service 应该变为 critical
```

## 关键学习点

1. **服务注册**：服务启动时向 Consul 注册自己的地址、端口、健康检查
2. **服务发现**：客户端通过服务名查询可用实例列表
3. **健康检查**：Consul 定期探测服务状态，自动剔除不健康实例
4. **gRPC Resolver**：gRPC 的可插拔服务发现机制，自定义 resolver 对接 Consul
5. **优雅退出**：进程退出时先注销服务，避免流量打到已下线的实例
6. **负载均衡**：基于服务发现结果的客户端负载均衡（round-robin）

## 与 K8s 的对比

| 操作 | Consul | K8s |
|------|--------|-----|
| 服务注册 | 调用 API 手动注册 | 创建 Service 资源自动注册 |
| 服务发现 | 查询 Consul API | DNS 查询（user-service:9091） |
| 健康检查 | Consul 配置 TCP/HTTP 检查 | Pod 配置 liveness/readiness probe |
| 负载均衡 | gRPC resolver 实现 | kube-proxy / IPVS |

**本质相同**：都是维护一个「服务名 → 地址列表」的映射，只是 K8s 把这个过程自动化了。

## 扩展思考

1. 如何实现跨数据中心的服务发现？
2. 如何用 Consul 的 KV 存储做配置中心？
3. 如何实现灰度发布（基于 tags 路由）？
4. Consul Connect 如何实现 Service Mesh？

## 参考代码

完整实现见项目代码：
- `internal/discovery/registry.go`
- `internal/discovery/resolver.go`
- `internal/discovery/balancer.go`
- `cmd/*/main.go`
