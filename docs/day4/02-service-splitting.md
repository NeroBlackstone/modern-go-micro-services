# Day 4 - 服务拆分

## 为什么要拆分单体？

单体架构（Monolith）在项目初期有优势（开发快、部署简单、事务一致性），但随着业务增长会出现：

1. **代码耦合**：修改一个模块可能影响其他模块
2. **部署瓶颈**：任何小改动都需要重新部署整个应用
3. **扩展困难**：无法针对热点模块单独扩容
4. **技术债务**：模块边界不清晰，容易产生"意大利面条式"代码

## 拆分策略

### 按业务领域拆分（DDD Bounded Context）

本项目按业务领域拆分为 3 个服务：

```
┌─────────────────────────────────────────────────────────────┐
│                      Online Bookstore                       │
├─────────────────┬─────────────────┬─────────────────────────┤
│  user-service   │  book-service   │    order-service        │
│                 │                 │                         │
│  用户注册/登录   │  图书 CRUD      │  订单管理               │
│  用户信息管理    │  库存管理       │  评价管理               │
│                 │  Redis 缓存     │  RabbitMQ 事件发布      │
│                 │                 │                         │
│  PostgreSQL     │  PostgreSQL     │  PostgreSQL             │
│  (user_db)      │  (book_db)      │  (order_db)             │
│                 │  Redis          │  RabbitMQ               │
├─────────────────┼─────────────────┼─────────────────────────┤
│  gRPC :9091     │  gRPC :9092     │  HTTP :8080             │
│  HTTP :8081     │                 │  (对外 REST API)        │
│  (register/     │                 │                         │
│   login/profile)│                 │                         │
└─────────────────┴─────────────────┴─────────────────────────┘
```

### 拆分原则

1. **数据库独立**：每个服务拥有自己的数据库（Database per Service）
   - 避免跨服务直接查询数据库
   - 通过 API 或事件同步数据

2. **接口通信**：
   - **同步**：gRPC（服务间内部调用）
   - **异步**：RabbitMQ（事件驱动、解耦）

3. **数据一致性**：
   - 同一服务内：数据库事务保证强一致性
   - 跨服务：Saga 模式保证最终一致性

## 服务间通信

### 同步通信（gRPC）

```
用户下单流程：
order-service ──gRPC──> book-service.GetBooks()
                     <── 返回图书信息
order-service ──gRPC──> book-service.DeductStock()
                     <── 扣库存结果
```

### 异步通信（RabbitMQ）

```
订单创建后：
order-service ──发布事件──> RabbitMQ ──> consumer（通知服务）
```

## 数据库拆分

### 原单体数据库

```
bookstore_db
├── users
├── books
├── orders
├── order_items
└── reviews
```

### 拆分后

```
user_db:    users
book_db:    books
order_db:   orders, order_items, reviews
```

**注意：** review 保留在 order_db 中，因为 HasPurchased 需要查 order_items。

## 服务发现

### 硬编码地址（当前方案）

```yaml
grpc:
  user_service_addr: user-service:9091
  book_service_addr: book-service:9092
```

**优点：** 简单，适合学习项目和小规模部署
**缺点：** 服务地址变更需要重新配置

### 服务注册与发现（生产方案）

使用 Consul、etcd 或 Nacos 等工具：

```
服务启动 → 注册到 Consul → 从 Consul 查询地址 → 发起调用
```

后续学习阶段会介绍 Consul 的使用。
