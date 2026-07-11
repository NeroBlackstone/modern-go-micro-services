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

	// 数据库指标
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Duration of database queries in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "operation"},
	)

	// Redis 指标
	RedisOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_operations_total",
			Help: "Total number of Redis operations",
		},
		[]string{"service", "operation", "status"},
	)

	// RabbitMQ 指标
	RabbitMQMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rabbitmq_messages_total",
			Help: "Total number of RabbitMQ messages",
		},
		[]string{"service", "operation", "status"},
	)
)
