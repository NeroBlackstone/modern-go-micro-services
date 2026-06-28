package discovery

import (
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// Registry 封装 Consul 服务注册/注销功能
type Registry struct {
	client    *api.Client
	consulAddr string
	logger    *zap.Logger
}

// NewRegistry 创建 Consul 客户端连接
func NewRegistry(consulAddr string, logger *zap.Logger) (*Registry, error) {
	config := api.DefaultConfig()
	config.Address = consulAddr

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	return &Registry{
		client:     client,
		consulAddr: consulAddr,
		logger:     logger,
	}, nil
}

// ServiceRegistration 服务注册信息
type ServiceRegistration struct {
	ServiceID   string
	ServiceName string
	Address     string
	Port        int
	Tags        []string
	Meta        map[string]string
}

// Register 将服务注册到 Consul
func (r *Registry) Register(reg *ServiceRegistration, checkInterval time.Duration) error {
	// 生成唯一的 service ID
	hostname, _ := os.Hostname()
	serviceID := fmt.Sprintf("%s-%s-%d", reg.ServiceName, hostname, reg.Port)

	registration := &api.AgentServiceRegistration{
		ID:      serviceID,
		Name:    reg.ServiceName,
		Address: reg.Address,
		Port:    reg.Port,
		Tags:    reg.Tags,
		Meta:    reg.Meta,
		Check: &api.AgentServiceCheck{
			// TCP 健康检查：定期探测服务端口
			TCP:                           fmt.Sprintf("%s:%d", reg.Address, reg.Port),
			Interval:                      checkInterval.String(),
			DeregisterCriticalServiceAfter: "30s", // 服务不健康超过 30s 自动注销
			Timeout:                       "5s",
		},
	}

	if err := r.client.Agent().ServiceRegister(registration); err != nil {
		return fmt.Errorf("failed to register service %s: %w", serviceID, err)
	}

	r.logger.Info("service registered to consul",
		zap.String("service_id", serviceID),
		zap.String("service_name", reg.ServiceName),
		zap.String("address", reg.Address),
		zap.Int("port", reg.Port),
	)

	return nil
}

// Deregister 从 Consul 注销服务
func (r *Registry) Deregister(serviceID string) error {
	if err := r.client.Agent().ServiceDeregister(serviceID); err != nil {
		return fmt.Errorf("failed to deregister service %s: %w", serviceID, err)
	}

	r.logger.Info("service deregistered from consul", zap.String("service_id", serviceID))
	return nil
}

// ServiceInstance 表示一个健康的服务实例
type ServiceInstance struct {
	Address string
	Port    int
}

// Discover 查询指定服务的健康实例列表
func (r *Registry) Discover(serviceName string) ([]*ServiceInstance, error) {
	entries, _, err := r.client.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to discover service %s: %w", serviceName, err)
	}

	var instances []*ServiceInstance
	for _, entry := range entries {
		instances = append(instances, &ServiceInstance{
			Address: entry.Service.Address,
			Port:    entry.Service.Port,
		})
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no healthy instances found for service %s", serviceName)
	}

	return instances, nil
}
