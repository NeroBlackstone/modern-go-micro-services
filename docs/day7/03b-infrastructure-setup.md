# 基础设施配置 — Prometheus + Grafana 部署

## 实现步骤

### 7. 创建 Prometheus 配置

创建 `configs/prometheus.yml`：

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "alert-rules.yml"

scrape_configs:
  - job_name: 'user-service'
    static_configs:
      - targets: ['user-service:8081']
    metrics_path: '/metrics'

  - job_name: 'book-service'
    static_configs:
      - targets: ['book-service:9093']
    metrics_path: '/metrics'

  - job_name: 'order-service'
    static_configs:
      - targets: ['order-service:8080']
    metrics_path: '/metrics'

  - job_name: 'traefik'
    static_configs:
      - targets: ['traefik:8080']
    metrics_path: '/metrics'
```

### 8. 创建告警规则

创建 `configs/alert-rules.yml`：

```yaml
groups:
  - name: bookstore-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate on {{ $labels.service }} - {{ $labels.endpoint }}"

      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High latency on {{ $labels.service }} - {{ $labels.endpoint }}"

      - alert: ServiceDown
        expr: up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Service {{ $labels.job }} is down"
```

### 9. 更新 Docker Compose

修改 `compose.yml`，添加 Prometheus 和 Grafana：

```yaml
  # ==================== 监控服务 ====================

  # Prometheus 指标监控
  prometheus:
    image: prom/prometheus:latest
    container_name: bookstore-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./configs/prometheus.yml:/etc/prometheus/prometheus.yml
      - ./configs/alert-rules.yml:/etc/prometheus/alert-rules.yml
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'
      - '--storage.tsdb.retention.time=200h'
      - '--web.enable-lifecycle'
    depends_on:
      - user-service
      - book-service
      - order-service

  # Grafana 可视化
  grafana:
    image: grafana/grafana:latest
    container_name: bookstore-grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana
    depends_on:
      - prometheus

# 在 volumes 部分添加
volumes:
  prometheus_data:
  grafana_data:
```

## 验证步骤

### 1. 启动服务

```bash
docker-compose up -d
```

### 2. 验证指标端点

```bash
# user-service
curl http://localhost:8081/metrics

# book-service
curl http://localhost:9093/metrics

# order-service
curl http://localhost:8080/metrics
```

### 3. 验证 Prometheus

1. 访问 http://localhost:9090
2. 进入 Status → Targets
3. 检查所有服务是否为 UP 状态
4. 进入 Graph，输入 PromQL 查询：
   ```
   http_requests_total
   ```

### 4. 验证 Grafana

1. 访问 http://localhost:3000
2. 登录：admin/admin
3. 添加数据源：Configuration → Data Sources → Add → Prometheus
4. 配置 URL：http://prometheus:9090
5. 保存并测试
6. 创建仪表板：Create → Dashboard
7. 添加面板，输入 PromQL 查询

## 常用 PromQL 查询

### 请求速率
```promql
# 每秒请求速率
rate(http_requests_total[5m])

# 按服务分组
sum by (service) (rate(http_requests_total[5m]))
```

### 错误率
```promql
# 错误请求速率
rate(http_requests_total{status=~"5.."}[5m])

# 错误率百分比
rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) * 100
```

### 延迟
```promql
# P50 延迟
histogram_quantile(0.5, rate(http_request_duration_seconds_bucket[5m]))

# P95 延迟
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# P99 延迟
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# 平均延迟
rate(http_request_duration_seconds_sum[5m]) / rate(http_request_duration_seconds_count[5m])
```

### 业务指标
```promql
# 用户注册总数
users_created_total

# 订单创建总数
orders_created_total

# 每分钟用户注册数
rate(users_created_total[1m]) * 60
```

## 总结

通过以上步骤，我们为书店系统添加了完整的监控能力：

1. **指标收集**：使用 Prometheus 收集 HTTP、gRPC 和业务指标
2. **可视化**：使用 Grafana 创建监控仪表板
3. **告警**：配置基本告警规则

现在你可以：
- 实时监控系统健康状况
- 快速发现和定位问题
- 分析性能瓶颈
- 为后续学习分布式追踪打下基础

**Day 8 预告**：我们将学习分布式追踪（Grafana Tempo/OpenTelemetry）和日志聚合（Loki）。
