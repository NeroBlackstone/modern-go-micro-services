package discovery

import (
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
)

const RoundRobinBalancerName = "consul-round-robin"

// init 注册自定义的负载均衡器
func init() {
	balancer.Register(newBuilder())
}

func newBuilder() balancer.Builder {
	return base.NewBalancerBuilder(
		RoundRobinBalancerName,
		&pickFirstBalancerPickerBuilder{},
		base.Config{HealthCheck: true},
	)
}

// pickFirstBalancerPickerBuilder 使用 round-robin 策略选择后端
type pickFirstBalancerPickerBuilder struct{}

func (b *pickFirstBalancerPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	// 收集所有可用的连接
	scs := make([]balancer.SubConn, 0, len(info.ReadySCs))
	for sc := range info.ReadySCs {
		scs = append(scs, sc)
	}

	return &roundRobinPicker{
		scs: scs,
	}
}

type roundRobinPicker struct {
	scs    []balancer.SubConn
	offset uint64
}

func (p *roundRobinPicker) Pick(balancer.PickInfo) (balancer.PickResult, error) {
	if len(p.scs) == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	offset := atomic.AddUint64(&p.offset, 1)
	idx := offset % uint64(len(p.scs))

	return balancer.PickResult{
		SubConn: p.scs[idx],
	}, nil
}

// RoundRobinBalancerConfig 负载均衡配置
type RoundRobinBalancerConfig struct {
	Logger *zap.Logger
}

// ServiceConfigJSON 返回 gRPC 的服务配置 JSON，使用 round-robin 负载均衡
func ServiceConfigJSON() string {
	return fmt.Sprintf(`{"loadBalancingConfig":[{"%s":{}}]}`, RoundRobinBalancerName)
}
