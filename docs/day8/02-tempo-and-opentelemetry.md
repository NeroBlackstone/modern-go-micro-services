# Tempo 与 OpenTelemetry

## OpenTelemetry 简介

OpenTelemetry（简称 OTel）是 CNCF 的可观测性标准项目，提供：

- **API**：定义了 Tracer、Meter、Logger 等接口
- **SDK**：接口的实现，负责数据采集、采样、导出
- **Collector**：可部署的代理服务，接收遥测数据并路由到后端
- **Propagators**：上下文传播的编解码器（W3C Trace Context、B3 等）

### 为什么选 OTel？

```
传统方式（每个工具一套 SDK）：
┌─────────┐  ┌─────────┐  ┌─────────┐
│  Jaeger  │  │  Zipkin │  │ Datadog │  ← 每个后端需要不同的埋点代码
│  SDK     │  │  SDK    │  │  SDK    │
└─────────┘  └─────────┘  └─────────┘

OTel 方式（标准 SDK + 可插拔 Exporter）：
┌──────────────────────┐
│   OTel SDK (标准)     │  ← 一次埋点，到处可用
│   Tracer / Meter     │
└──────────┬───────────┘
           │
    ┌──────┼──────┐
    │      │      │
┌───▼──┐ ┌▼───┐ ┌▼────┐
│Jaeger│ │Tempo│ │Prom │  ← 切换后端只需改 Exporter 配置
└──────┘ └────┘ └─────┘
```

## Grafana Tempo 架构

Tempo 是 Grafana Labs 推出的分布式链路追踪后端，负责存储和查询 OTel Trace。Tempo 3.0 引入了**流式架构**，通过 Kafka 实现写入与查询的解耦，支持近实时查询和弹性扩展。

### 整体数据流

```
服务
  ↓
OTel SDK
  ↓
OTel Collector
  ↓
┌─────────────────────────────────────────────────────────┐
│                    Tempo 3.0                            │
│                                                         │
│   Distributor ──→ Kafka ──┬──→ Live Store (实时查询)    │
│                           │                             │
│                           └──→ Block Builder ──→ Object Storage
│                                                         │
│   Querier ←── Live Store + Object Storage               │
│                                                         │
│   Metrics Generator ←── Kafka ──→ Prometheus/Mimir     │
│                                                         │
└─────────────────────────────────────────────────────────┘
  ↓
Grafana (TraceQL 查询 + 可视化)
```

### Tempo 核心组件

#### Distributor — 写入入口

Distributor 是 Tempo 写入链路的入口组件，负责接收和分发 Trace 数据。

- **接收 Trace**：支持 OTLP、Jaeger、Zipkin 等协议
- **校验请求**：检查租户、Span、Trace 是否符合限制
- **限流**：按租户执行 Rate Limit，超限请求直接拒绝
- **路由数据**：根据 `traceID` 哈希，将同一 Trace 路由到同一分区/节点，避免跨节点聚合
- **分发数据**：微服务模式写入 Kafka；单体模式直接发送到 Live Store

**为什么要按 TraceID 路由？** 保证同一 Trace 的所有 Span 进入同一处理节点，简化 Trace 聚合，提高写入和查询效率。

#### Kafka — 流式消息总线

Kafka 在 Tempo 3 中充当写入缓冲和数据总线：

- **解耦**：Distributor 与存储流程解耦
- **削峰**：缓冲突发写入流量
- **顺序保证**：按 TraceID 分区保证同一 Trace 的 Span 顺序
- **消息重放**：Block Builder 崩溃后可重新消费 Kafka 恢复数据

Kafka 不负责长期存储，仅作为写入缓冲层。

#### Live Store — 实时 Trace 缓存

Live Store 是 Tempo 3.0 流式架构中的实时 Trace 缓存层，负责维护内存中的 Live Trace：

- 消费 Kafka 中的 Trace 数据
- 根据 `traceID` 聚合属于同一 Trace 的 Span
- 在内存维护尚未持久化的 Live Trace
- 为 Querier 提供最新 Trace 的**近实时查询**能力
- 达到 Flush 条件后，将 Trace 交给 Block Builder 构建 Parquet Block

**为什么需要 Live Store？** 避免用户查询最新 Trace 时，因尚未写入对象存储而查不到数据。

#### Block Builder — 持久化写入

Block Builder 负责将 Kafka 中的 Trace 数据转换成对象存储中的 Parquet Block：

- 从 Kafka 消费 Span 数据
- 按 Trace 组织和聚合 Span
- 构建压缩、高效查询的 **Parquet Block**
- 将 Block 写入 S3/GCS/MinIO 等对象存储

**与 Live Store 的区别**：

| 组件 | 职责 | 存储位置 | 查询延迟 |
|------|------|---------|---------|
| Live Store | 保存近期 Trace，提供低延迟实时查询 | 内存 | 极低 |
| Block Builder | 生成历史 Block，写入对象存储，支持长期查询 | 对象存储 | 较低 |

**Parquet 是什么？** 一种列式存储文件格式，同一字段连续存储，查询时只读取需要的列，压缩效率高，适合 Trace 这类需要按字段检索的数据。

#### Object Storage — 长期存储

长期存储 Trace Block，支持 S3、MinIO、GCS、Azure Blob 等。基于对象存储，低成本扩展。

#### Querier — 查询执行

Querier 负责执行 Trace 查询，同时查询 Live Store 和对象存储，合并返回完整 Trace：

```
Query Frontend → Querier → Live Store (实时数据)
                          → Object Storage (Parquet Block)
```

#### Query Frontend — 查询调度

Query Frontend 是 Tempo 的查询调度组件：

- **查询拆分**：将大查询拆分为多个小查询并行执行
- **查询缓存**：缓存热点查询结果
- **查询排队/限流**：控制并发查询数量
- **负载均衡**：将查询分发到多个 Querier

**与 Querier 的区别**：Query Frontend 管理查询任务，Querier 执行查询。

#### Metrics Generator — Trace → Metrics

Metrics Generator 消费 Trace，自动生成 Metrics，减少应用手动埋点：

- **Span Metrics**：从 Trace 生成 RED 指标（请求量、错误率、延迟）
- **Service Graph**：根据 Trace 的调用关系生成服务依赖拓扑
- 将生成的 Metrics 写入 Prometheus/Mimir

**限制**：不能完全替代 Metrics。系统指标（CPU、内存、GC）和业务指标（订单量、交易额）仍需独立采集。

### 为什么选 Tempo？

| 特性 | Tempo | Jaeger |
|------|-------|--------|
| 存储模型 | 列式存储（Parquet），无索引，基于对象存储 | 需要索引（内存/Cassandra/ES） |
| 查询语言 | TraceQL（类 LogQL/PromQL 语法） | 标签搜索 |
| 资源占用 | 极低（无索引开销，对象存储低成本） | 较高（索引 + 存储） |
| Grafana 集成 | 原生（同一生态） | 需要额外配置数据源 |
| Trace-to-Metrics | 内置 metrics_generator | 需要额外组件 |
| 依赖 | 对象存储 + Kafka（可选） | Cassandra / Elasticsearch |
| 架构 | 流式架构（Kafka 解耦），支持弹性扩展 | 传统架构 |

### 单机模式 vs 微服务模式

Tempo 支持两种部署模式，通过启动参数 `target` 指定：

#### 单机模式（Monolithic Mode）

所有组件运行在同一个进程中，通过 `target: all` 启动：

- **优点**：部署简单，适合开发测试和小规模环境
- **缺点**：无法针对单个组件独立扩缩容

#### 微服务模式（Microservices Mode）

各组件拆分为独立服务运行，每个组件通过独立 `target` 启动：

- **优点**：高可用、弹性扩展，适合生产环境和大规模 Trace 场景
- **缺点**：部署和运维复杂度更高

```yaml
# 微服务模式：每个组件独立部署
# distributor
command: -config.file=/etc/tempo/tempo.yml -target=distributor
# querier
command: -config.file=/etc/tempo/tempo.yml -target=querier
# block-builder
command: -config.file=/etc/tempo/tempo.yml -target=block-builder
```

### Tempo 配置文件

Tempo 核心配置文件为 `tempo.yaml`，定义各组件运行配置：

```yaml
server:
  http_listen_port: 3200

distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: "0.0.0.0:4318"

ingest:
  kafka:
    address: kafka:9092
    topic: tempo-traces
    consumer_group: tempo

live_store:
  max_live_traces: 10000

block_builder:
  block:
    max_block_duration: 5m

storage:
  trace:
    backend: local
    local:
      path: /var/tempo/traces

metrics_generator:
  registry:
    external_labels:
      source: tempo
      cluster: docker-compose
  storage:
    path: /var/tempo/generator/wal
    remote_write:
      - url: http://prometheus:9090/api/v1/write
  traces_storage:
    path: /var/tempo/generator/traces
  processor:
    service_graphs:
      dimensions:
        - http.method
        - http.target
    span_metrics:
      dimensions:
        - http.method
        - http.target
```

**核心配置模块**：

| 模块 | 作用 |
|------|------|
| `server` | HTTP/gRPC 服务端口 |
| `distributor` | 接收 OTLP Trace 数据并分发 |
| `ingest` | Kafka 接入配置 |
| `live_store` | 实时 Trace 缓存 |
| `block_builder` | 构建 Parquet Block |
| `storage` | 后端对象存储（S3、GCS、本地磁盘） |
| `querier` | Trace 查询 |
| `query_frontend` | 查询拆分、缓存和优化 |
| `metrics_generator` | 从 Trace 生成 Metrics |
| `overrides` | 租户级限流和参数覆盖 |

## OTel Go SDK 核心 API

### 初始化 TracerProvider

```go
import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// 1. 创建 OTLP HTTP exporter（Tempo 原生支持 OTLP 协议）
exporter, _ := otlptracehttp.New(
    context.Background(),
    otlptracehttp.WithEndpoint("tempo:4318"),
    otlptracehttp.WithInsecure(),
)

// 2. 定义服务资源
res, _ := resource.Merge(
    resource.Default,
    resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceNameKey.String("my-service"),
    ),
)

// 3. 创建 TracerProvider
tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),  // 批量导出
    sdktrace.WithResource(res),      // 服务元数据
    sdktrace.WithSampler(sdktrace.AlwaysSample()), // 采样策略
)

// 4. 设置为全局 Provider
otel.SetTracerProvider(tp)

// 5. 设置 Context Propagator
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
    propagation.TraceContext{},  // W3C 标准
    propagation.Baggage{},       // W3C Baggage
))
```

### 创建和使用 Span

```go
// 获取 Tracer
tracer := otel.Tracer("my-service")

// 创建 Span
ctx, span := tracer.Start(ctx, "operation-name",
    trace.WithSpanKind(trace.SpanKindClient),  // 客户端 Span
    trace.WithAttributes(
        attribute.String("http.method", "POST"),
        attribute.Int("http.status_code", 200),
    ),
)
defer span.End()  // Span 结束时自动上报

// 业务逻辑...
doSomething(ctx)

// 记录错误
if err != nil {
    span.SetStatus(codes.Error, err.Error())
    span.RecordError(err)
}
```

### Context Propagation

```go
// 注入（发送端）：将 trace context 写入 carrier
propagator.Inject(ctx, carrier)

// 提取（接收端）：从 carrier 中读取 trace context
ctx = propagator.Extract(ctx, carrier)
```

Carrier 是一个 key-value 接口，不同传输协议有不同实现：
- HTTP Header → `propagation.HeaderCarrier`
- gRPC Metadata → 自定义 `metadataCarrier`

## 采样策略

| 策略 | 说明 | 适用场景 |
|------|------|---------|
| AlwaysSample | 100% 采样 | 开发/测试环境 |
| NeverSample | 0% 采样 | 关闭追踪 |
| TraceIDRatioBased | 按比例采样 | 生产环境（如 10%） |
| ParentBased | 跟随父 Span 的采样决定 | 生产环境（推荐） |

生产环境推荐配置：
```go
sdktrace.WithSampler(
    sdktrace.ParentBased(
        sdktrace.TraceIDRatioBased(0.1),  // 10% 采样率
    ),
)
```

## Grafana 中查看 Traces

启动后打开 Grafana (`http://localhost:3000`) → **Explore** → 选择 **Tempo** 数据源：

1. **搜索 Trace**：按 Service 名称、操作名称、时间范围搜索
2. **TraceQL 查询**：使用 TraceQL 语法进行高级查询
   - `{resource.service.name = "order-service"}` — 按服务名搜索
   - `{http.method = "POST"} |= "error"` — 搜索包含错误的 POST 请求
   - `{duration > 100ms}` — 搜索耗时超过 100ms 的 trace
3. **查看链路**：点击 Trace 查看完整调用树
4. **Span 详情**：查看每个 Span 的 Tags、Duration
5. **Service Graph**：Tempo metrics_generator 自动生成的服务依赖图
6. **Trace to Metrics**：从 trace 直接跳转到关联的 Prometheus 指标

## Docker Compose 完整示例

```yaml
# Kafka（Tempo 3 写入缓冲）
kafka:
  image: bitnami/kafka:latest
  ports:
    - "9092:9092"
  environment:
    - KAFKA_CFG_NODE_ID=1
    - KAFKA_CFG_PROCESS_ROLES=controller,broker
    - KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=1@kafka:9093
    - KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093
    - KAFKA_CFG_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092
    - KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
    - KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER
  volumes:
    - kafka_data:/bitnami/kafka

# Tempo 3（单机模式，target=all）
tempo:
  image: grafana/tempo:latest
  container_name: bookstore-tempo
  ports:
    - "3200:3200"
    - "4318:4318"
  volumes:
    - ./configs/tempo.yml:/etc/tempo/tempo.yml
    - tempo_data:/var/tempo
  command: -config.file=/etc/tempo/tempo.yml
  depends_on:
    - kafka

# Grafana（已配置 Tempo 数据源）
grafana:
  image: grafana/grafana:latest
  ports:
    - "3000:3000"
  volumes:
    - grafana_data:/var/lib/grafana
    - ./configs/grafana/provisioning:/etc/grafana/provisioning
  depends_on:
    - prometheus
    - tempo
```

Grafana 自动 provisioning Tempo 数据源 (`configs/grafana/provisioning/datasources/datasources.yml`)：

```yaml
apiVersion: 1
datasources:
  - name: Tempo
    type: tempo
    uid: tempo
    access: proxy
    url: http://tempo:3200
    jsonData:
      tracesToLogsV2:
        datasourceUid: prometheus
      nodeGraph:
        enabled: true
```
