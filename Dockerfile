# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 安装 protobuf 编译器（如果需要在容器内生成代码）
RUN apk add --no-cache git

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译所有服务
RUN CGO_ENABLED=0 GOOS=linux go build -o user-service-server ./cmd/user-service
RUN CGO_ENABLED=0 GOOS=linux go build -o book-service-server ./cmd/book-service
RUN CGO_ENABLED=0 GOOS=linux go build -o order-service-server ./cmd/order-service

# ==================== 用户服务 ====================
FROM alpine:3.19 AS user-service

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/user-service-server .
COPY --from=builder /app/configs/user-service.yaml ./configs/

EXPOSE 9091 8081

CMD ["./user-service-server"]

# ==================== 图书服务 ====================
FROM alpine:3.19 AS book-service

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/book-service-server .
COPY --from=builder /app/configs/book-service.yaml ./configs/

EXPOSE 9092

CMD ["./book-service-server"]

# ==================== 订单服务 ====================
FROM alpine:3.19 AS order-service

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/order-service-server .
COPY --from=builder /app/configs/order-service.yaml ./configs/

EXPOSE 8080

CMD ["./order-service-server"]
