# 服务代码实现 — Prometheus 指标接入

## 系统架构

```
                    ┌─────────────────────────────────────┐
                    │           Prometheus                 │
                    │  ┌─────────────┐  ┌─────────────┐  │
                    │  │  Retrieval   │  │    TSDB     │  │
                    │  │  (拉取指标)  │  │  (存储)     │  │
                    │  └──────┬──────┘  └─────────────┘  │
                    │         │                           │
                    │         │     ┌─────────────┐       │
                    │         │     │   Rules     │       │
                    │         │     │  (告警)     │       │
                    │         │     └─────────────┘       │
                    └─────────┼───────────────────────────┘
                              │
            ┌─────────────────┼─────────────────┐
            │                 │                 │
            ▼                 ▼                 ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │ user-service │  │ book-service │  │ order-service│
    │   :8081      │  │   :9093      │  │   :8080      │
    │  /metrics    │  │  /metrics    │  │  /metrics    │
    └──────────────┘  └──────────────┘  └──────────────┘
            │                 │                 │
            │                 │                 │
    ┌───────┴─────────────────┴─────────────────┴───────┐
    │                    Grafana                         │
    │              http://localhost:3000                 │
    └───────────────────────────────────────────────────┘
```

## 实现步骤

### 1. 添加依赖

首先，为项目添加 Prometheus 客户端库：

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

### 2. 创建指标文件

创建 `internal/metrics/metrics.go`，定义所有指标：

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // HTTP 请求指标
    HTTPRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"service", "method", "endpoint", "status"},
    )

    HTTPRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "Duration of HTTP requests in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"service", "method", "endpoint"},
    )

    // gRPC 指标
    GRPCRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "grpc_requests_total",
            Help: "Total number of gRPC requests",
        },
        []string{"service", "method", "code"},
    )

    // 业务指标
    UsersCreatedTotal = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "users_created_total",
            Help: "Total number of users created",
        },
    )

    OrdersCreatedTotal = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "orders_created_total",
            Help: "Total number of orders created",
        },
    )

    BooksViewedTotal = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "books_viewed_total",
            Help: "Total number of books viewed",
        },
    )
)
```

### 3. 创建中间件

创建 `internal/middleware/metrics.go`，自动收集 HTTP 请求指标：

```go
package middleware

import (
    "fmt"
    "time"

    "github.com/gin-gonic/gin"
    "modern-micro-services/internal/metrics"
)

// PrometheusMiddleware 创建一个 Prometheus 指标收集中间件
func PrometheusMiddleware(serviceName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        // 处理请求
        c.Next()

        // 记录指标
        duration := time.Since(start).Seconds()
        status := fmt.Sprintf("%d", c.Writer.Status())

        metrics.HTTPRequestsTotal.WithLabelValues(
            serviceName,
            c.Request.Method,
            c.FullPath(),
            status,
        ).Inc()

        metrics.HTTPRequestDuration.WithLabelValues(
            serviceName,
            c.Request.Method,
            c.FullPath(),
        ).Observe(duration)
    }
}
```

### 4. 更新 user-service

修改 `cmd/user-service/main.go`：

```go
import (
    "modern-micro-services/internal/middleware"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// 在路由设置中添加
gin.SetMode(cfg.Server.Mode)
r := gin.New()
r.Use(gin.Recovery())
r.Use(middleware.PrometheusMiddleware("user-service"))  // 添加中间件

// 在路由中添加 metrics 端点
r.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

在 `internal/user/handler/http_handler.go` 中添加业务指标：

```go
import "modern-micro-services/internal/metrics"

func (h *HTTPHandler) Register(c *gin.Context) {
    // ... 现有逻辑 ...
    
    // 记录业务指标
    metrics.UsersCreatedTotal.Inc()
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": result})
}
```

### 5. 更新 order-service

修改 `cmd/order-service/main.go`：

```go
import (
    "modern-micro-services/internal/middleware"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// 在路由设置中添加
gin.SetMode(cfg.Server.Mode)
r := gin.New()
r.Use(gin.Recovery())
r.Use(middleware.PrometheusMiddleware("order-service"))  // 添加中间件

// 在路由中添加 metrics 端点
r.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

在 `internal/order/handler/order_handler.go` 中添加业务指标：

```go
import "modern-micro-services/internal/metrics"

func (h *OrderHandler) CreateOrder(c *gin.Context) {
    // ... 现有逻辑 ...
    
    // 记录业务指标
    metrics.OrdersCreatedTotal.Inc()
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": order})
}
```

### 6. 更新 book-service

由于 book-service 只有 gRPC 服务器，需要添加一个轻量级的 HTTP 服务器。

修改 `internal/book/config/config.go`：

```go
type ServerConfig struct {
    GRPCPort    int    `mapstructure:"grpc_port"`
    MetricsPort int    `mapstructure:"metrics_port"`  // 新增
    Mode        string `mapstructure:"mode"`
}
```

修改 `configs/book-service.yaml`：

```yaml
server:
  grpc_port: 9092
  metrics_port: 9093  # 新增
  mode: debug
```

修改 `cmd/book-service/main.go`：

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// 在 gRPC 服务器启动后添加
go func() {
    metricsMux := http.NewServeMux()
    metricsMux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
    metricsMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "book-service"})
    })

    metricsAddr := fmt.Sprintf(":%d", cfg.Server.MetricsPort)
    logger.Info("book-service metrics server starting", zap.String("addr", metricsAddr))
    metricsServer := &http.Server{
        Addr:    metricsAddr,
        Handler: metricsMux,
    }

    // 优雅关闭
    go func() {
        quit := make(chan os.Signal, 1)
        signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
        <-quit
        logger.Info("shutting down metrics server...")
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        metricsServer.Shutdown(ctx)
    }()

    if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Fatal("metrics server error", zap.Error(err))
    }
}()
```
