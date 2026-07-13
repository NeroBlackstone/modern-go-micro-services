# 分布式链路追踪概念

## 为什么需要分布式链路追踪？

在微服务架构中，一个用户请求往往需要经过多个服务的协作处理。例如在我们的在线书店系统中，一次"创建订单"请求的完整链路：

```
用户请求
  → Traefik (API 网关)
    → order-service (HTTP)
      → book-service (gRPC) — 获取图书信息
      → book-service (gRPC) — 扣减库存
      → PostgreSQL (order_db) — 创建订单
      → RabbitMQ — 发布订单创建事件
        → consumer — 消费通知消息
```

当这个链路出现延迟或错误时，如何定位问题？

- **Logs（日志）**：每个服务有自己的日志，但无法自动关联跨服务调用
- **Metrics（指标）**：能看到各服务的 QPS 和延迟，但不知道一个请求具体经过了哪些服务
- **Traces（链路）**：**唯一能展示完整请求链路的方式**——从入口到出口，每一跳的耗时、状态、依赖关系

## 核心概念

### Trace（链路）

一个完整的请求链路。每个 Trace 有一个全局唯一的 **Trace ID**，贯穿所有服务。

```
Trace ID: abc-123-def-456
├── Span A: order-service.CreateOrder (200ms)
│   ├── Span B: book-service.GetBooks (50ms)
│   └── Span C: book-service.DeductStock (30ms)
│       └── Span D: PostgreSQL INSERT (5ms)
└── Span E: RabbitMQ.Publish (10ms)
```

### Span（跨度）

Trace 中的一个工作单元。每个 Span 包含：

| 字段 | 说明 |
|------|------|
| Trace ID | 所属链路的全局唯一标识 |
| Span ID | 自身的唯一标识 |
| Parent Span ID | 父 Span 的 ID（形成调用树） |
| Operation Name | 操作名称，如 `HTTP POST /api/v1/orders` |
| Start Time | 开始时间 |
| Duration | 持续时间 |
| Status | 状态（OK / Error） |
| Tags | 键值对元数据（如 `http.status_code=200`） |
| Logs | 时间戳事件（如错误堆栈） |

### Context Propagation（上下文传播）

跨服务传递 Trace ID 和 Span ID 的机制。这是分布式追踪的核心——没有传播，链路就断了。

**传播方式：**

```
HTTP 请求：通过 Header 传播
┌─────────────────────────────────────┐
│ traceparent: 00-abc123-span456-01  │  ← W3C Trace Context 标准
└─────────────────────────────────────┘

gRPC 请求：通过 Metadata 传播
┌─────────────────────────────────────┐
│ grpc-trace-bin: <binary encoded>   │
└─────────────────────────────────────┘

消息队列：通过 Message Header 传播
┌─────────────────────────────────────┐
│ traceparent: 00-abc123-span456-01  │
└─────────────────────────────────────┘
```

## 分布式追踪 vs 单机追踪

```
单机追踪（如 pprof）：
┌──────────────────┐
│  一个进程内的调用  │
│  函数 A → B → C   │
└──────────────────┘

分布式追踪：
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ order-service │───→│ book-service │───→│  PostgreSQL  │
│  Span A       │    │  Span B      │    │  Span C      │
└──────────────┘    └──────────────┘    └──────────────┘
     同一个 Trace ID 贯穿所有服务
```

## 可观测性三支柱的关系

```
                    ┌─────────────┐
                    │  Traces     │ ← 请求级全景（哪个服务慢？哪里出错？）
                    │  (链路追踪)  │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
       ┌──────▼──────┐  ┌──▼───┐  ┌────▼────┐
       │   Metrics    │  │      │  │  Logs   │
       │  (指标监控)   │  │      │  │ (日志)  │
       └─────────────┘  └──────┘  └─────────┘
       聚合趋势分析      ↑        详细事件记录
                    关联入口
```

三者互补：
- **Metrics** 告诉你"系统整体怎么样"（QPS、延迟 p99、错误率）
- **Logs** 告诉你"具体发生了什么"（某次请求的详细错误信息）
- **Traces** 告诉你"请求经过了哪里，每步耗时多少"（端到端链路可视化）

## 常用术语

| 术语 | 说明 |
|------|------|
| Trace | 一个完整请求链路 |
| Span | 链路中的一个工作单元 |
| SpanContext | 跨服务传播的上下文（Trace ID + Span ID + 采样标志） |
| Propagation | 将 SpanContext 注入/提取到传输载体（HTTP Header / gRPC Metadata） |
| Collector | 接收、处理、存储 Trace 数据的后端服务 |
| Sampling | 采样策略——生产环境不一定 100% 采集 |
| Baggage | 跨服务传递的键值对（非追踪数据，是业务元数据） |

## 主流技术选型

| 方案 | 特点 |
|------|------|
| Grafana Tempo | Grafana Labs 出品，轻量无索引，原生 OTLP，配合 Grafana TraceQL 查询 |
| Zipkin | Twitter 开源，社区萎缩，私有格式 |
| SkyWalking | Apache 顶级项目，国内生态强，但架构重 |
| Grafana Tempo | 轻量后端，无索引，依赖 Grafana TraceQL |
| OpenTelemetry | **标准本身**（SDK + API + Collector），后端可插拔 |

**推荐组合：OpenTelemetry SDK + Grafana Tempo 后端**（本项目采用）
