# 可观测性核心概念

## 什么是可观测性

可观测性（Observability）是指通过系统的外部输出来推断其内部状态的能力。在微服务架构中，由于服务数量多、调用链路复杂，可观测性变得至关重要。

## 三大支柱

### 1. 日志（Logs）

日志是离散事件的记录，描述了系统中发生的具体事情。

```
特点：
- 离散事件：每条日志是一个独立的事件
- 详细信息：包含时间戳、级别、消息、上下文
- 文本格式：易于阅读和搜索
- 低频写入：相比指标，写入频率较低
```

**日志级别：**
- DEBUG：调试信息，开发环境使用
- INFO：一般信息，记录正常操作
- WARN：警告信息，潜在问题
- ERROR：错误信息，操作失败
- FATAL：致命错误，服务无法继续

**日志示例：**
```json
{
  "level": "info",
  "ts": "2026-07-06T10:00:00Z",
  "caller": "user/handler.go:25",
  "msg": "user login successful",
  "user_id": 123,
  "email": "user@example.com",
  "ip": "192.168.1.100"
}
```

### 2. 指标（Metrics）

指标是数值型的时间序列数据，用于衡量系统的状态和性能。

```
特点：
- 数值型：每个指标是一个数值
- 时间序列：按时间顺序记录
- 聚合性：可以进行统计计算（求和、平均、百分位）
- 高频采集：通常每秒或每15秒采集一次
```

**指标类型：**
- **Counter（计数器）**：只增不减的累计值
  - 示例：`http_requests_total`（总请求数）
  - 用途：统计总量，如请求总数、错误总数

- **Gauge（仪表盘）**：可增可减的瞬时值
  - 示例：`cpu_usage_percent`（CPU使用率）
  - 用途：表示当前状态，如温度、内存使用量

- **Histogram（直方图）**：统计值的分布
  - 示例：`http_request_duration_seconds`（请求延迟）
  - 用途：计算百分位数，如 P95 延迟

- **Summary（摘要）**：类似 Histogram，但在客户端计算
  - 示例：`go_gc_duration_seconds`（GC 暂停时间）
  - 用途：精确的百分位计算

**指标示例：**
```
# 帮助信息
# HELP http_requests_total Total number of HTTP requests
# 类型
# TYPE http_requests_total counter
# 指标值（带标签）
http_requests_total{method="GET", endpoint="/api/v1/users", status="200"} 1234
http_requests_total{method="POST", endpoint="/api/v1/users", status="201"} 56
http_requests_total{method="GET", endpoint="/api/v1/users", status="404"} 12
```

### 3. 追踪（Traces）

追踪记录请求在分布式系统中的完整路径，包括跨服务调用。

```
特点：
- 链路跟踪：记录请求从入口到出口的完整路径
- 分布式：跨越多个服务
- 上下文传播：通过 Trace ID 关联不同服务的调用
- 延迟分析：识别性能瓶颈
```

**追踪示例：**
```
Trace ID: abc123def456

┌─────────────────────────────────────────────────────────────┐
│  user-service (100ms)                                       │
│  ├─ HTTP GET /api/v1/users (10ms)                          │
│  ├─ gRPC GetUser (50ms)                                    │
│  │  └─ DB Query SELECT * FROM users (30ms)                 │
│  └─ Redis GET user:123 (10ms)                              │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│  order-service (200ms)                                      │
│  ├─ HTTP POST /api/v1/orders (20ms)                        │
│  ├─ gRPC CreateOrder (150ms)                               │
│  │  ├─ DB INSERT INTO orders (80ms)                        │
│  │  └─ RabbitMQ Publish (20ms)                             │
│  └─ gRPC GetBook (30ms)                                    │
└─────────────────────────────────────────────────────────────┘
```
Grafana 是一个开源可观测性可视化平台，不负责采集和存储数据，通过 Data Source 连接 Prometheus、Loki、Tempo、ES、SQL 等数据源，将 Metrics、Logs、Traces 统一展示。

核心组件：

Data Source：连接外部数据源，如 Prometheus、Loki、Tempo。
Dashboard：监控大盘，由多个 Panel 组成。
Panel：具体图表组件，如折线图、表格、日志展示。
Query：向数据源发送查询（如 PromQL、LogQL、SQL）。
Alerting：基于查询结果配置告警规则，触发通知。

典型架构：

应用
 ↓
OpenTelemetry / Exporter
 ↓
Prometheus + Loki + Tempo
 ↓
Grafana
 ↓
Dashboard / Alert
## 三大支柱的关系

```
日志：发生了什么？（What happened?）
指标：系统状态如何？（How is the system?）
追踪：请求经过了哪里？（Where did the request go?）
```

**实际应用：**
1. **发现问题**：通过指标发现异常（如错误率上升）
2. **定位原因**：通过追踪找到问题服务（如某个服务延迟高）
3. **分析细节**：通过日志查看具体错误信息（如数据库连接失败）

## 可观测性工具栈

### 日志
- **ELK Stack**：Elasticsearch + Logstash + Kibana
- **EFK Stack**：Elasticsearch + Fluentd + Kibana
- **Loki + Grafana**：轻量级日志聚合

### 指标
- **Prometheus**：时序数据库 + 查询语言
- **Grafana**：可视化仪表板
- **InfluxDB**：另一种时序数据库

### 追踪
- **Jaeger**：Uber 开源的分布式追踪系统
- **Zipkin**：Twitter 开源的分布式追踪系统
- **OpenTelemetry**：可观测性框架标准

## 微服务中的可观测性挑战

### 1. 服务数量多
- 每个服务都需要暴露指标
- 需要统一的监控平台
- 配置管理复杂

### 2. 调用链路复杂
- 请求可能经过多个服务
- 需要分布式追踪
- 延迟难以定位

### 3. 故障排查困难
- 日志分散在不同服务
- 需要关联分析
- 根因定位耗时

### 4. 性能监控
- 需要实时监控
- 需要历史数据对比
- 需要告警机制

## 最佳实践

### 1. 统一标准
- 使用 OpenTelemetry 标准
- 统一日志格式（JSON）
- 统一指标命名规范

### 2. 自动化
- 自动采集指标
- 自动关联追踪
- 自动告警

### 3. 可视化
- 创建服务拓扑图
- 创建依赖关系图
- 创建性能仪表板

### 4. 告警
- 设置合理的阈值
- 避免告警疲劳
- 分级告警策略

## 在我们的项目中

我们将为书店系统添加以下可观测性能力：

1. **指标监控**：Prometheus + Grafana
   - HTTP 请求指标
   - gRPC 请求指标
   - 业务指标（用户注册数、订单数）
   - 基础设施指标（数据库、Redis、RabbitMQ）

2. **日志聚合**：Loki + Grafana（后续 Day 8）
   - 结构化日志
   - 日志搜索和分析

3. **分布式追踪**：Jaeger + OpenTelemetry（后续 Day 8）
   - 跨服务追踪
   - 延迟分析

## 总结

可观测性是微服务架构的三大支柱之一（另外两个是自动化和弹性）。通过日志、指标和追踪，我们可以：
- 实时了解系统状态
- 快速发现和定位问题
- 深入分析性能瓶颈
- 持续优化系统

在 Day 7 中，我们将重点学习指标监控，使用 Prometheus 收集指标，Grafana 进行可视化。
