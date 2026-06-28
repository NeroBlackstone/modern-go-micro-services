# Day 5 - Consul 基础

## Consul 是什么？

HashiCorp Consul 是一个服务发现和配置工具，提供：
- **服务发现**：服务注册与查询
- **健康检查**：自动摘除故障实例
- **KV 存储**：分布式配置中心
- **Service Mesh**：服务网格（Consul Connect）

## 安装与启动

### Docker 方式（推荐开发环境）

```bash
# 单节点开发模式
docker run -d --name consul \
  -p 8500:8500 \
  -p 8600:8600/udp \
  hashicorp/consul:2.0.1 agent -dev -client=0.0.0.0
```

### 访问 Web UI

打开浏览器：http://localhost:8500

你会看到 Consul 的管理界面，可以查看：
- 已注册的服务
- 健康检查状态
- KV 存储
- 节点信息

## 核心 API

### 1. 服务注册

```bash
# 通过 HTTP API 注册服务
curl -X PUT http://localhost:8500/v1/agent/service/register -d '{
  "ID": "user-service-1",
  "Name": "user-service",
  "Address": "172.18.0.2",
  "Port": 9091,
  "Tags": ["grpc", "user"],
  "Check": {
    "TCP": "172.18.0.2:9091",
    "Interval": "10s",
    "Timeout": "5s",
    "DeregisterCriticalServiceAfter": "30s"
  }
}'
```

**参数说明：**
- `ID`：服务唯一标识
- `Name`：服务名称（客户端用这个查询）
- `Address`：服务地址
- `Port`：服务端口
- `Tags`：标签（可用于过滤）
- `Check`：健康检查配置

### 2. 服务注销

```bash
# 注销指定服务
curl -X PUT http://localhost:8500/v1/agent/service/deregister/user-service-1
```

### 3. 查询服务

```bash
# 查询所有健康的 user-service 实例
curl http://localhost:8500/v1/health/service/user-service?passing=true

# 返回示例：
[
  {
    "Service": {
      "ID": "user-service-1",
      "Service": "user-service",
      "Address": "172.18.0.2",
      "Port": 9091,
      "Tags": ["grpc", "user"]
    },
    "Checks": [
      {
        "Status": "passing"
      }
    ]
  }
]
```

### 4. 健康检查类型

| 类型 | 配置 | 说明 |
|------|------|------|
| HTTP | `"HTTP": "http://172.18.0.2:8080/health"` | 定期 HTTP 请求 |
| TCP | `"TCP": "172.18.0.2:9091"` | TCP 连接探测 |
| gRPC | `"GRPC": "172.18.0.2:9091"` | gRPC 健康检查协议 |
| Script | `"Script": "/bin/check.sh"` | 执行脚本 |
| TTL | 需要服务主动上报 | 服务心跳 |

## Go SDK 使用

### 安装

```bash
go get github.com/hashicorp/consul/api
```

### 基本用法

```go
package main

import (
    "fmt"
    "github.com/hashicorp/consul/api"
)

func main() {
    // 1. 创建 Consul 客户端
    config := api.DefaultConfig()
    config.Address = "localhost:8500"
    client, _ := api.NewClient(config)

    // 2. 注册服务
    registration := &api.AgentServiceRegistration{
        ID:      "user-service-1",
        Name:    "user-service",
        Address: "172.18.0.2",
        Port:    9091,
        Tags:    []string{"grpc", "user"},
        Check: &api.AgentServiceCheck{
            TCP:                           "172.18.0.2:9091",
            Interval:                      "10s",
            Timeout:                       "5s",
            DeregisterCriticalServiceAfter: "30s",
        },
    }
    client.Agent().ServiceRegister(registration)

    // 3. 查询服务
    services, _, _ := client.Health().Service("user-service", "", true, nil)
    for _, svc := range services {
        fmt.Printf("Found: %s:%d\n", svc.Service.Address, svc.Service.Port)
    }

    // 4. 注销服务
    client.Agent().ServiceDeregister("user-service-1")
}
```

## 服务 ID 的设计

### 为什么需要唯一 ID？

当同一服务有多个实例时，需要用 ID 区分：

```
user-service-1 (172.18.0.2:9091)
user-service-2 (172.18.0.3:9091)
user-service-3 (172.18.0.4:9091)
```

### 推荐的 ID 格式

```go
// 格式: 服务名-主机名-端口
serviceID := fmt.Sprintf("%s-%s-%d", serviceName, hostname, port)
// 示例: user-service-hostname-9091
```

**为什么包含 hostname？**
- 保证同一主机上的实例 ID 唯一
- 容器重启后 hostname 可能变化，自动注册新 ID

## 健康检查的工作流程

```
1. 服务注册时配置健康检查
   ↓
2. Consul 定期（每10s）探测服务端口
   ↓
3. 探测成功 → 状态 = passing
   探测失败 → 状态 = critical
   ↓
4. critical 持续超过 DeregisterCriticalServiceAfter（30s）
   ↓
5. 自动从服务列表移除
```

## KV 存储（配置中心）

Consul 的 KV 存储可以用作分布式配置中心：

```bash
# 写入配置
curl -X PUT http://localhost:8500/v1/kv/config/database/host -d 'postgres-host'
curl -X PUT http://localhost:8500/v1/kv/config/database/port -d '5432'

# 读取配置
curl http://localhost:8500/v1/kv/config/database/host?raw
# 返回: postgres-host
```

**Go SDK 读取：**

```go
kv := client.KV()
pair, _, _ := kv.Get("config/database/host", nil)
fmt.Println(string(pair.Value))  // postgres-host
```

## Consul 集群（生产环境）

### 单节点（开发）

```bash
consul agent -dev
```

### 3 节点集群（生产最小规模）

```bash
# Server 1
consul agent -server -bootstrap-expect=3 -datacenter=dc1 \
  -node=node1 -bind=10.0.0.1

# Server 2
consul agent -server -bootstrap-expect=3 -datacenter=dc1 \
  -node=node2 -bind=10.0.0.2 -join=10.0.0.1

# Server 3
consul agent -server -bootstrap-expect=3 -datacenter=dc1 \
  -node=node3 -bind=10.0.0.3 -join=10.0.0.1
```

**为什么至少 3 个 Server？**
- 使用 Raft 共识算法
- 需要多数派（2/3）才能达成共识
- 容忍 1 个节点故障

## 常见问题

### Q: Consul 和 etcd 怎么选？

| 场景 | 推荐 |
|------|------|
| K8s 环境 | etcd（K8s 内置） |
| 非 K8s 环境 | Consul（功能更全） |
| 需要 Service Mesh | Consul（Consul Connect） |
| 轻量级需求 | etcd（更简单） |

### Q: 健康检查间隔怎么配？

- 开发环境：10s（快速反馈）
- 生产环境：15-30s（平衡准确性和负载）

### Q: DeregisterCriticalServiceAfter 什么意思？

服务被标记为 critical 后，多久自动注销。设置为 30s 意味着：
- 服务故障后，30s 内如果恢复，自动重新注册
- 服务故障超过 30s，自动从列表移除

## 扩展阅读

- [Consul 官方文档](https://www.consul.io/docs)
- [Consul API 参考](https://www.consul.io/api)
- [Consul vs etcd vs zookeeper](https://www.consul.io/docs/internals/consul.html)
