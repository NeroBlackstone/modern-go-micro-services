# Prometheus 核心概念

## Prometheus 是什么

Prometheus 是一个开源的系统监控和报警工具包，最初由 SoundCloud 开发，现已成为云原生计算基金会（CNCF）的毕业项目。

## 核心架构

```
┌─────────────────────────────────────────────────────────────┐
│                      Prometheus Server                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Retrieval  │  │   TSDB      │  │   HTTP API  │        │
│  │   (拉取)     │  │  (存储)     │  │  (查询)     │        │
│  └──────┬──────┘  └─────────────┘  └──────┬──────┘        │
│         │                                  │                │
│         │         ┌─────────────┐          │                │
│         │         │   Rules     │          │                │
│         │         │  (告警规则)  │          │                │
│         │         └─────────────┘          │                │
└─────────┼──────────────────────────────────┼────────────────┘
          │                                  │
          │ 拉取指标                          │ 查询数据
          │                                  │
┌─────────▼──────────┐          ┌────────────▼────────────┐
│   Targets          │          │      Grafana            │
│  (被监控服务)       │          │    (可视化仪表板)       │
└────────────────────┘          └─────────────────────────┘
```

## Pull 模型

Prometheus 使用 **Pull 模型** 采集指标，与 Push 模型（如 StatsD）不同：

### Pull 模型（Prometheus）
```
┌─────────────┐         ┌─────────────┐
│ Prometheus  │ ──拉取─→ │   Service   │
│   Server    │ ←─────── │   /metrics  │
└─────────────┘  HTTP    └─────────────┘
```

**优点：**
- 服务不需要知道 Prometheus 的地址
- Prometheus 主动控制采集频率
- 可以轻松监控多个实例
- 服务故障时，Prometheus 可以检测到

**缺点：**
- 需要服务暴露 HTTP 端点
- 防火墙需要允许 Prometheus 访问服务

### Push 模型（StatsD）
```
┌─────────────┐         ┌─────────────┐
│   Service   │ ──推送─→ │  StatsD     │
│             │         │   Server    │
└─────────────┘         └─────────────┘
```

**优点：**
- 服务主动推送，不需要暴露端点
- 适合短生命周期的任务

**缺点：**
- 服务需要知道 StatsD 地址
- StatsD 故障时，指标会丢失

## 指标类型详解

### 1. Counter（计数器）

只增不减的累计值，重启后重置。

```go
// 定义
var httpRequestsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total number of HTTP requests",
    },
    []string{"method", "endpoint", "status"},
)

// 使用
httpRequestsTotal.WithLabelValues("GET", "/api/v1/users", "200").Inc()
httpRequestsTotal.WithLabelValues("POST", "/api/v1/users", "201").Inc()
```

**查询示例：**
```promql
# 总请求数
http_requests_total

# 每秒请求数（速率）
rate(http_requests_total[5m])

# 特定方法的请求数
http_requests_total{method="GET"}
```

### 2. Gauge（仪表盘）

可增可减的瞬时值。

```go
// 定义
var cpuUsage = prometheus.NewGauge(
    prometheus.GaugeOpts{
        Name: "cpu_usage_percent",
        Help: "Current CPU usage percentage",
    },
)

// 使用
cpuUsage.Set(65.5)
cpuUsage.Inc()
cpuUsage.Dec()
```

**查询示例：**
```promql
# 当前 CPU 使用率
cpu_usage_percent

# CPU 使用率超过 80% 的时间
cpu_usage_percent > 80
```

### 3. Histogram（直方图）

统计值的分布，自动创建多个 bucket。

```go
// 定义
var httpRequestDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Help:    "Duration of HTTP requests in seconds",
        Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
    },
    []string{"method", "endpoint"},
)

// 使用
httpRequestDuration.WithLabelValues("GET", "/api/v1/users").Observe(0.25)
```

**自动创建的指标：**
```
http_request_duration_seconds_bucket{le="0.01"} 100
http_request_duration_seconds_bucket{le="0.05"} 200
http_request_duration_seconds_bucket{le="0.1"} 300
http_request_duration_seconds_bucket{le="0.5"} 400
http_request_duration_seconds_bucket{le="1"} 450
http_request_duration_seconds_bucket{le="2"} 480
http_request_duration_seconds_bucket{le="5"} 490
http_request_duration_seconds_bucket{le="10"} 500
http_request_duration_seconds_bucket{le="+Inf"} 500
http_request_duration_seconds_sum 125.5
http_request_duration_seconds_count 500
```

**查询示例：**
```promql
# P50 延迟（中位数）
histogram_quantile(0.5, rate(http_request_duration_seconds_bucket[5m]))

# P95 延迟
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# P99 延迟
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# 平均延迟
rate(http_request_duration_seconds_sum[5m]) / rate(http_request_duration_seconds_count[5m])
```

### 4. Summary（摘要）

类似 Histogram，但在客户端计算分位数。

```go
// 定义
var httpRequestDuration = prometheus.NewSummaryVec(
    prometheus.SummaryOpts{
        Name:       "http_request_duration_seconds",
        Help:       "Duration of HTTP requests in seconds",
        Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
    },
    []string{"method", "endpoint"},
)

// 使用
httpRequestDuration.WithLabelValues("GET", "/api/v1/users").Observe(0.25)
```

**自动创建的指标：**
```
http_request_duration_seconds{quantile="0.5"} 0.25
http_request_duration_seconds{quantile="0.9"} 0.5
http_request_duration_seconds{quantile="0.99"} 1.2
http_request_duration_seconds_sum 125.5
http_request_duration_seconds_count 500
```

**Histogram vs Summary：**
| 特性 | Histogram | Summary |
|------|-----------|---------|
| 计算位置 | 服务端（查询时） | 客户端（推送时） |
| 聚合性 | 支持（可以跨实例聚合） | 不支持（无法聚合） |
| 精确度 | 近似值 | 精确值 |
| 性能开销 | 较低 | 较高 |

**推荐使用 Histogram**，因为：
1. 可以跨实例聚合
2. 可以计算任意分位数
3. 性能开销更低

## 标签（Labels）

标签是键值对，用于区分不同的指标维度。

```go
// 定义带标签的指标
var httpRequestsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total number of HTTP requests",
    },
    []string{"method", "endpoint", "status"},
)

// 使用标签
httpRequestsTotal.WithLabelValues("GET", "/api/v1/users", "200").Inc()
httpRequestsTotal.WithLabelValues("POST", "/api/v1/users", "201").Inc()
httpRequestsTotal.WithLabelValues("GET", "/api/v1/users", "404").Inc()
```

**查询示例：**
```promql
# 所有 GET 请求
http_requests_total{method="GET"}

# 所有 200 状态码的请求
http_requests_total{status="200"}

# 特定端点的 GET 请求
http_requestsTotal{method="GET", endpoint="/api/v1/users"}

# 正则匹配
http_requests_total{status=~"2.."}  # 所有 2xx 状态码
http_requests_total{endpoint!~"/health"}  # 排除健康检查端点
```

**标签最佳实践：**
1. 标签值应该是低基数的（不要使用用户ID、请求ID等）
2. 标签名称应该有意义
3. 避免过多标签（会影响性能）
4. 保持标签一致性

## 服务发现

Prometheus 支持多种服务发现方式：

### 1. 静态配置
```yaml
scrape_configs:
  - job_name: 'user-service'
    static_configs:
      - targets: ['user-service:8081']
```

### 2. 文件服务发现
```yaml
scrape_configs:
  - job_name: 'user-service'
    file_sd_configs:
      - files:
        - 'targets/user-service.json'
```

### 3. DNS 服务发现
```yaml
scrape_configs:
  - job_name: 'user-service'
    dns_sd_configs:
      - names: ['user-service']
        type: 'A'
        port: 8081
```

### 4. Consul 服务发现
```yaml
scrape_configs:
  - job_name: 'user-service'
    consul_sd_configs:
      - server: 'consul:8500'
        services: ['user-service']
```

### 5. Kubernetes 服务发现
```yaml
scrape_configs:
  - job_name: 'user-service'
    kubernetes_sd_configs:
      - role: pod
```

## 告警规则

Prometheus 支持基于 PromQL 查询的告警规则。

### 告警规则示例
```yaml
groups:
  - name: bookstore-alerts
    rules:
      # 高错误率告警
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate on {{ $labels.service }}"
          description: "Error rate is {{ $value }} requests per second"

      # 高延迟告警
      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High latency on {{ $labels.service }}"
          description: "P95 latency is {{ $value }} seconds"

      # 服务宕机告警
      - alert: ServiceDown
        expr: up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Service {{ $labels.job }} is down"
```

### 告警状态
- **pending**：告警条件已满足，等待 `for` 时间
- **firing**：告警已触发
- **resolved**：告警已恢复

## Prometheus 查询语言（PromQL）

### 基础查询
```promql
# 查询指标
http_requests_total

# 带标签查询
http_requests_total{method="GET"}

# 范围查询（过去5分钟）
http_requests_total[5m]
```

### 聚合操作
```promql
# 求和
sum(http_requests_total)

# 按标签分组求和
sum by (method) (http_requests_total)

# 平均值
avg(http_request_duration_seconds)

# 最大值
max(http_requests_total)

# 最小值
min(http_requests_total)

# 计数
count(http_requests_total)
```

### 数学运算
```promql
# 加法
http_requests_total + 100

# 减法
http_requests_total - 10

# 乘法
http_requests_total * 2

# 除法
http_requests_total / 100
```

### 函数
```promql
# 速率（每秒增长速率）
rate(http_requests_total[5m])

# 增量（区间内增长值）
increase(http_requests_total[5m])

# 时间戳
time()

# 标签替换
label_replace(http_requests_total, "new_label", "value", "old_label", ".*")
```

## Grafana 集成

Grafana 是一个开源的可视化工具，支持 Prometheus 作为数据源。

### 数据源配置
1. 访问 Grafana：http://localhost:3000
2. 登录：admin/admin
3. 添加数据源：Configuration → Data Sources → Add
4. 选择 Prometheus
5. 配置 URL：http://prometheus:9090
6. 保存并测试

### 创建仪表板
1. 创建新仪表板
2. 添加面板
3. 选择数据源：Prometheus
4. 编写 PromQL 查询
5. 配置可视化选项

### 常用面板类型
- **Time Series**：时间序列图，显示指标随时间变化
- **Stat**：单值统计，显示当前值
- **Gauge**：仪表盘，显示当前值和阈值
- **Table**：表格，显示详细数据
- **Heatmap**：热力图，显示分布

## 总结

Prometheus 是一个强大的监控工具，核心概念包括：
1. **Pull 模型**：主动拉取指标
2. **指标类型**：Counter、Gauge、Histogram、Summary
3. **标签**：区分不同维度
4. **服务发现**：自动发现服务
5. **告警规则**：基于 PromQL 的告警
6. **PromQL**：强大的查询语言

在 Day 7 中，我们将使用 Prometheus 监控书店系统的三个微服务。
