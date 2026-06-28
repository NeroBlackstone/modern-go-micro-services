# Phase 1.5: Zap 日志库基础

## 1. Zap 简介

Zap 是 Uber 开源的高性能 Go 日志库，提供结构化日志和高性能输出。

### 核心特点

| 特点 | 说明 |
|------|------|
| **高性能** | 比标准库 log 快 5-10 倍 |
| **结构化日志** | 支持键值对格式，便于解析 |
| **类型安全** | 编译时类型检查，避免运行时错误 |
| **零分配** | 高性能场景下避免内存分配 |
| **丰富的配置** | 支持多种输出格式和目标 |

## 2. 安装

```bash
# 核心库
go get go.uber.org/zap

# 可选：用于自定义编码器
go get go.uber.org/zap/zapcore
```

## 3. 基本使用

### 3.1 快速开始

```go
package main

import (
    "go.uber.org/zap"
)

func main() {
    // 生产环境（JSON 格式，高性能）
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // 开发环境（可读性更好）
    // logger, _ := zap.NewDevelopment()

    logger.Info("server started",
        zap.String("addr", ":8080"),
        zap.Int("port", 8080),
    )
}
```

### 3.2 日志级别

```go
logger.Debug("debug message")    // 调试信息（生产环境默认不输出）
logger.Info("info message")      // 一般信息
logger.Warn("warn message")      // 警告
logger.Error("error message")    // 错误
logger.Fatal("fatal message")    // 致命错误（会调用 os.Exit(1)）
logger.Panic("panic message")    // panic 错误（会触发 panic）
```

## 4. 结构化字段类型

### 4.1 基本类型

```go
logger.Info("user action",
    zap.String("username", "john"),
    zap.Int("user_id", 123),
    zap.Float64("amount", 99.5),
    zap.Bool("is_admin", true),
    zap.Duration("latency", time.Since(start)),
    zap.Time("timestamp", time.Now()),
    zap.Error(err),
)
```

### 4.2 复合类型

```go
logger.Info("order details",
    zap.Strings("tags", []string{"premium", "urgent"}),
    zap.Int64s("order_ids", []int64{1001, 1002, 1003}),
    zap.Any("metadata", map[string]any{
        "key": "value",
        "nested": map[string]int{"a": 1},
    }),
)
```

### 4.3 反射序列化

```go
// 对于复杂对象，使用 zap.Reflect
type User struct {
    Name   string
    Email  string
    Orders []Order
}

logger.Info("user data",
    zap.Reflect("user", User{
        Name:  "John",
        Email: "john@example.com",
    }),
)
```

## 5. 自定义配置

### 5.1 基础配置

```go
package main

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func initLogger() *zap.Logger {
    config := zap.Config{
        Level:       zap.NewAtomicLevelAt(zap.InfoLevel),
        Development: false,
        Encoding:    "json", // 或 "console"
        EncoderConfig: zapcore.EncoderConfig{
            TimeKey:        "timestamp",
            LevelKey:       "level",
            NameKey:        "logger",
            CallerKey:      "caller",
            FunctionKey:    zapcore.OmitKey,
            MessageKey:     "msg",
            StacktraceKey:  "stacktrace",
            LineEnding:     zapcore.DefaultLineEnding,
            EncodeLevel:    zapcore.LowercaseLevelEncoder,
            EncodeTime:     zapcore.ISO8601TimeEncoder,
            EncodeDuration: zapcore.SecondsDurationEncoder,
            EncodeCaller:   zapcore.ShortCallerEncoder,
        },
        OutputPaths:      []string{"stdout"},
        ErrorOutputPaths: []string{"stderr"},
    }

    logger, err := config.Build()
    if err != nil {
        panic(err)
    }
    return logger
}
```

### 5.2 开发环境配置

```go
func initDevLogger() *zap.Logger {
    config := zap.NewDevelopmentConfig()
    config.Level.SetLevel(zap.DebugLevel)
    
    logger, _ := config.Build()
    return logger
}
```

## 6. 高级功能

### 6.1 添加字段（子日志器）

```go
// 创建带默认字段的日志器
logger := logger.With(
    zap.String("service", "user-service"),
    zap.String("version", "1.0.0"),
    zap.String("env", "production"),
)

// 所有日志都会包含这些字段
logger.Info("user logged in") // 自动包含 service, version, env 字段
```

### 6.2 命名日志器

```go
// 用于区分不同模块的日志
dbLogger := logger.Named("database")
dbLogger.Info("query executed") // 输出包含 "logger": "database"

httpLogger := logger.Named("http")
httpLogger.Info("request received") // 输出包含 "logger": "http"
```

### 6.3 添加调用者信息

```go
// 自动记录日志调用的文件名和行号
logger := logger.WithOptions(zap.AddCaller())

logger.Info("user created")
// 输出会包含 "caller": "main.go:42"
```

### 6.4 添加栈追踪

```go
// 在特定级别添加栈追踪
logger := logger.WithOptions(zap.AddStacktrace(zap.ErrorLevel))

logger.Error("something went wrong")
// 输出会包含完整的调用栈
```

## 7. 输出目标配置

### 7.1 多输出目标

```go
config := zap.Config{
    // ... 其他配置
    OutputPaths:      []string{"stdout", "/var/log/app.log"},
    ErrorOutputPaths: []string{"stderr", "/var/log/app-error.log"},
}
```

### 7.2 自定义输出

```go
import "go.uber.org/zap/zapcore"

func createLoggerWithFile() *zap.Logger {
    // 创建文件写入器
    file, _ := os.Create("app.log")
    writer := zapcore.AddSync(file)

    // 创建核心
    core := zapcore.NewCore(
        zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
        writer,
        zap.InfoLevel,
    )

    return zap.New(core)
}
```

## 8. 微服务最佳实践

### 8.1 全局日志器初始化

```go
package logger

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "sync"
)

var (
    globalLogger *zap.Logger
    once         sync.Once
)

// Init 初始化全局日志器
func Init(level string) {
    once.Do(func() {
        var zapLevel zapcore.Level
        if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
            zapLevel = zap.InfoLevel
        }

        config := zap.Config{
            Level:       zap.NewAtomicLevelAt(zapLevel),
            Development: false,
            Encoding:    "json",
            EncoderConfig: zapcore.EncoderConfig{
                TimeKey:        "timestamp",
                LevelKey:       "level",
                CallerKey:      "caller",
                MessageKey:     "msg",
                StacktraceKey:  "stacktrace",
                EncodeLevel:    zapcore.LowercaseLevelEncoder,
                EncodeTime:     zapcore.ISO8601TimeEncoder,
                EncodeCaller:   zapcore.ShortCallerEncoder,
            },
            OutputPaths:      []string{"stdout"},
            ErrorOutputPaths: []string{"stderr"},
        }

        var err error
        globalLogger, err = config.Build(zap.AddCallerSkip(1))
        if err != nil {
            panic(err)
        }
    })
}

// Get 获取全局日志器
func Get() *zap.Logger {
    if globalLogger == nil {
        Init("info")
    }
    return globalLogger
}

// Sync 同步日志缓冲区
func Sync() {
    if globalLogger != nil {
        globalLogger.Sync()
    }
}
```

### 8.2 HTTP 中间件日志

```go
package middleware

import (
    "time"
    "go.uber.org/zap"
)

func LoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        // 记录请求开始
        logger.Info("request started",
            zap.String("method", c.Request.Method),
            zap.String("path", c.Request.URL.Path),
            zap.String("client_ip", c.ClientIP()),
        )
        
        // 调用下一个处理器
        c.Next()
        
        // 记录请求结束
        logger.Info("request completed",
            zap.String("method", c.Request.Method),
            zap.String("path", c.Request.URL.Path),
            zap.Int("status", c.Writer.Status()),
            zap.Duration("latency", time.Since(start)),
            zap.Int("body_size", c.Writer.Size()),
        )
    }
}
```

### 8.3 业务逻辑日志

```go
package service

import (
    "context"
    "go.uber.org/zap"
)

type UserService struct {
    logger *zap.Logger
}

func NewUserService(logger *zap.Logger) *UserService {
    // 创建带模块名的日志器
    return &UserService{
        logger: logger.Named("user-service"),
    }
}

func (s *UserService) CreateUser(ctx context.Context, user *User) error {
    // 创建带请求上下文的日志器
    log := s.logger.With(
        zap.String("operation", "create_user"),
        zap.String("user_id", user.ID),
    )

    log.Info("creating user")

    // 业务逻辑...
    if err := s.db.Create(user); err != nil {
        log.Error("failed to create user",
            zap.Error(err),
            zap.Stack("stack"),
        )
        return err
    }

    log.Info("user created successfully")
    return nil
}
```

### 8.4 错误处理日志

```go
func handleError(logger *zap.Logger, err error, operation string) {
    logger.Error("operation failed",
        zap.String("operation", operation),
        zap.Error(err),
        zap.Stack("stack"),
        zap.Time("timestamp", time.Now()),
    )
}
```

## 9. 性能优化

### 9.1 避免不必要的日志

```go
// 使用条件判断避免不必要的日志开销
if logger.Core().Enabled(zap.DebugLevel) {
    logger.Debug("detailed debug info",
        zap.String("key", expensiveOperation()),
    )
}
```

### 9.2 使用内存池

```go
// 对于高频日志，使用对象池
var loggerPool = &sync.Pool{
    New: func() any {
        return globalLogger.WithOptions(zap.AddCallerSkip(1))
    },
}

func getLogger() *zap.Logger {
    return loggerPool.Get().(*zap.Logger)
}

func putLogger(l *zap.Logger) {
    loggerPool.Put(l)
}
```

## 10. 完整示例

```go
package main

import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func main() {
    // 初始化日志器
    config := zap.Config{
        Level:       zap.NewAtomicLevelAt(zap.InfoLevel),
        Development: false,
        Encoding:    "json",
        EncoderConfig: zapcore.EncoderConfig{
            TimeKey:        "timestamp",
            LevelKey:       "level",
            CallerKey:      "caller",
            MessageKey:     "msg",
            StacktraceKey:  "stacktrace",
            EncodeLevel:    zapcore.LowercaseLevelEncoder,
            EncodeTime:     zapcore.ISO8601TimeEncoder,
            EncodeCaller:   zapcore.ShortCallerEncoder,
        },
        OutputPaths:      []string{"stdout"},
        ErrorOutputPaths: []string{"stderr"},
    }

    logger, _ := config.Build(zap.AddCaller())
    defer logger.Sync()

    // 使用日志器
    logger.Info("application started",
        zap.String("version", "1.0.0"),
        zap.Int("port", 8080),
    )

    // 带字段的日志
    logger.Info("user action",
        zap.String("user_id", "123"),
        zap.String("action", "login"),
    )
}
```

## 11. 参考资源

- [Zap 官方文档](https://pkg.go.dev/go.uber.org/zap)
- [Zap GitHub 仓库](https://github.com/uber-go/zap)
- [Zap 性能基准测试](https://github.com/uber-go/zap#performance)
