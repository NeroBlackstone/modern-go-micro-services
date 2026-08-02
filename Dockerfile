# Build stage
FROM golang:1.26-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY

WORKDIR /app

# 安装 protobuf 编译器（如果需要在容器内生成代码）
RUN apk add --no-cache git

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译所有服务
RUN CGO_ENABLED=0 GOOS=linux go build -o book-service-server ./cmd/book-service
RUN CGO_ENABLED=0 GOOS=linux go build -o order-service-server ./cmd/order-service
RUN CGO_ENABLED=0 GOOS=linux go build -o hydra-login-consent-server ./cmd/hydra-login-consent
RUN CGO_ENABLED=0 GOOS=linux go build -o oauth-client-demo-server ./cmd/oauth-client-demo
RUN CGO_ENABLED=0 GOOS=linux go build -o webhook-server ./cmd/webhook
RUN CGO_ENABLED=0 GOOS=linux go build -o admin-service-server ./cmd/admin-service

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

# ==================== Hydra Login/Consent 服务 ====================
FROM alpine:3.19 AS hydra-login-consent

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/hydra-login-consent-server .

EXPOSE 3001

CMD ["./hydra-login-consent-server"]

# ==================== OAuth2 客户端演示 ====================
FROM alpine:3.19 AS oauth-client-demo

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/oauth-client-demo-server .

EXPOSE 8082

CMD ["./oauth-client-demo-server"]

# ==================== Webhook 服务 ====================
FROM alpine:3.19 AS webhook

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/webhook-server .

EXPOSE 8080

CMD ["./webhook-server"]

# ==================== 管理服务 ====================
FROM alpine:3.19 AS admin-service

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/admin-service-server .
COPY --from=builder /app/configs/admin-service.yaml ./configs/

EXPOSE 8084

CMD ["./admin-service-server"]
