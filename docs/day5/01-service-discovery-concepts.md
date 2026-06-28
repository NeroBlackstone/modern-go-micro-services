# Day 5 - 服务发现基本概念

## 为什么需要服务发现？

### 传统单体架构

```
客户端 → 单体应用（一个进程，所有功能）
```

- 地址固定：`localhost:8080`
- 无需发现：只有一个实例

### 微服务架构

```
客户端 → user-service (172.18.0.2:9091)
       → book-service (172.18.0.3:9092)
       → order-service (172.18.0.4:8080)
```

**问题：**
1. 服务地址是动态的（容器 IP 会变）
2. 服务会扩缩容（实例数量不固定）
3. 服务会故障（需要自动摘除）

## 核心概念

### 服务注册（Service Registration）

服务启动时，将自己的地址信息注册到注册中心。

```
user-service 启动
    ↓
调用注册中心 API：{"name": "user-service", "address": "172.18.0.2", "port": 9091}
    ↓
注册中心存储服务信息
```

### 服务发现（Service Discovery）

客户端需要调用其他服务时，从注册中心查询可用实例。

```
order-service 需要调用 book-service
    ↓
查询注册中心：book-service 有哪些健康实例？
    ↓
返回：[172.18.0.3:9092, 172.18.0.4:9092]
    ↓
order-service 选择一个实例调用
```

### 健康检查（Health Check）

注册中心定期探测服务实例是否健康，不健康的实例会被摘除。

```
注册中心 ──(每10s)──→ user-service:9091
                      ↓
               响应正常 → 标记为 healthy
               响应超时 → 标记为 unhealthy → 从服务列表移除
```

## 两种服务发现模式

### 客户端发现（Client-Side Discovery）

```
┌─────────────┐
│ order-service│
└──────┬──────┘
       │ 1. 查询注册中心
       ▼
┌─────────────┐
│   Consul    │
└──────┬──────┘
       │ 2. 返回实例列表
       ▼
┌─────────────┐
│ order-service│──→ 3. 负载均衡选择一个 ──→ book-service
└─────────────┘
```

**优点：** 客户端控制负载均衡策略，灵活
**缺点：** 客户端逻辑复杂，需要实现负载均衡

### 服务端发现（Server-Side Discovery）

```
┌─────────────┐
│   Client    │──→ Load Balancer ──→ book-service
└─────────────┘         │
                        │ 查询注册中心
                        ▼
                   ┌─────────────┐
                   │   Consul    │
                   └─────────────┘
```

**优点：** 客户端简单，只需调用 Load Balancer
**缺点：** 需要维护额外的 Load Balancer

**本项目使用客户端发现模式**，因为 gRPC 原生支持自定义 resolver。

## 主流注册中心对比

| 注册中心 | 语言 | 一致性 | 健康检查 | KV 存储 | 适用场景 |
|----------|------|--------|----------|---------|----------|
| Consul | Go | CP (Raft) | HTTP/TCP/gRPC | ✅ | 通用，功能全面 |
| etcd | Go | CP (Raft) | HTTP | ✅ | K8s 生态，轻量 |
| Nacos | Java | AP/CP 可切换 | HTTP/TCP | ✅ | Java 生态，阿里开源 |
| Eureka | Java | AP | HTTP | ❌ | Spring Cloud，已停止维护 |

## 服务发现 vs K8s DNS

### 传统方案（Consul）

```
需要：
- 部署并维护 Consul 集群
- 服务启动时调用 API 注册
- 客户端集成 Consul SDK 查询
```

### 现代方案（K8s DNS）

```
K8s 自动提供：
- Service 资源 → 自动创建 DNS 记录
- Endpoints → 自动管理实例列表
- kube-proxy → 自动负载均衡

直接用：user-service:9091
```

### 为什么还要学 Consul？

1. **理解原理**：Consul 展示了服务发现的完整流程，K8s 只是自动化了
2. **非 K8s 环境**：VM 部署、混合云、边缘计算
3. **面试常问**：很多公司还在用，面试会考
4. **更强功能**：跨数据中心、Service Mesh（Consul Connect）

## 本项目的学习目标

1. 使用 Consul 实现服务注册与发现
2. 理解 gRPC 的 Resolver 机制
3. 实现客户端负载均衡
4. 为后续学习 K8s 部署打下基础

## 扩展阅读

- [Microservices Patterns - Service Discovery](https://microservices.io/patterns/service-discovery.html)
- [Consul vs etcd vs Nacos](https://www.consul.io/docs/architecture)
- [gRPC Load Balancing](https://github.com/grpc/grpc/blob/master/doc/load-balancing.md)
