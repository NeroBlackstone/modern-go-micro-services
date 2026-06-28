package discovery

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"
)

const consulScheme = "consul"

// ConsulResolverBuilder 实现 resolver.Builder 接口
// 用于解析 consul:///service-name 格式的 gRPC 地址
type ConsulResolverBuilder struct {
	registry *Registry
	logger   *zap.Logger
}

// NewConsulResolverBuilder 创建 Consul resolver builder
func NewConsulResolverBuilder(registry *Registry, logger *zap.Logger) *ConsulResolverBuilder {
	return &ConsulResolverBuilder{
		registry: registry,
		logger:   logger,
	}
}

// Scheme 返回此 resolver 支持的 URL scheme
func (b *ConsulResolverBuilder) Scheme() string {
	return consulScheme
}

// Build 根据 target URI 创建 resolver
// target 格式: consul:///service-name
func (b *ConsulResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := target.Endpoint()
	if serviceName == "" {
		return nil, fmt.Errorf("consul resolver: service name is required in target (e.g. consul:///book-service)")
	}

	r := &consulResolver{
		serviceName: serviceName,
		registry:    b.registry,
		cc:          cc,
		logger:      b.logger,
		stopCh:      make(chan struct{}),
	}

	// 立即解析一次
	r.resolve()

	// 启动定期刷新 goroutine（每 10 秒刷新一次服务列表）
	go r.watch()

	return r, nil
}

// consulResolver 实现 resolver.Resolver 接口
type consulResolver struct {
	serviceName string
	registry    *Registry
	cc          resolver.ClientConn
	logger      *zap.Logger
	stopCh      chan struct{}
	mu          sync.Mutex
}

// resolve 从 Consul 查询服务实例并更新 gRPC 连接
func (r *consulResolver) resolve() {
	instances, err := r.registry.Discover(r.serviceName)
	if err != nil {
		r.logger.Warn("consul resolve failed",
			zap.String("service", r.serviceName),
			zap.Error(err),
		)
		return
	}

	// 转换为 gRPC resolver 的 Address 格式
	addrs := make([]resolver.Address, len(instances))
	for i, inst := range instances {
		addrs[i] = resolver.Address{
			Addr: fmt.Sprintf("%s:%d", inst.Address, inst.Port),
		}
	}

	// 更新 gRPC 的连接地址列表
	r.cc.UpdateState(resolver.State{Addresses: addrs})

	r.logger.Debug("consul resolver updated",
		zap.String("service", r.serviceName),
		zap.Int("instances", len(instances)),
	)
}

// watch 定期从 Consul 刷新服务实例列表
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

// Stop 停止 resolver
func (r *consulResolver) Stop() {
	close(r.stopCh)
}

// Close 关闭 resolver（实现 resolver.Resolver 接口）
func (r *consulResolver) Close() {
	r.Stop()
}

// ResolveNow 立即触发解析（实现 resolver.Resolver 接口）
func (r *consulResolver) ResolveNow(resolver.ResolveNowOptions) {
	r.resolve()
}
