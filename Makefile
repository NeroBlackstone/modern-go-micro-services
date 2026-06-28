.PHONY: proto build build-user build-book build-order docker-up docker-down clean

# ==================== Proto ====================

# 生成 protobuf Go 代码
proto:
	@echo "Generating protobuf code..."
	@mkdir -p gen/bookstore/user/v1 gen/bookstore/book/v1
	protoc --go_out=. --go_opt=Mproto/bookstore/user/v1/user.proto=modern-micro-services/gen/bookstore/user/v1 \
		--go_opt=Mproto/bookstore/book/v1/book.proto=modern-micro-services/gen/bookstore/book/v1 \
		--go-grpc_out=. --go-grpc_opt=Mproto/bookstore/user/v1/user.proto=modern-micro-services/gen/bookstore/user/v1 \
		--go-grpc_opt=Mproto/bookstore/book/v1/book.proto=modern-micro-services/gen/bookstore/book/v1 \
		proto/bookstore/user/v1/user.proto \
		proto/bookstore/book/v1/book.proto
	@echo "Proto generation complete!"

# ==================== Build ====================

build: build-user build-book build-order

build-user:
	@echo "Building user-service..."
	@go build -o bin/user-service ./cmd/user-service

build-book:
	@echo "Building book-service..."
	@go build -o bin/book-service ./cmd/book-service

build-order:
	@echo "Building order-service..."
	@go build -o bin/order-service ./cmd/order-service

# ==================== Docker ====================

docker-up:
	@echo "Starting all services..."
	@docker compose up --build -d
	@echo "Services starting..."
	@echo "  - user-service:  http://localhost:8081"
	@echo "  - book-service:  gRPC://localhost:9092"
	@echo "  - order-service: http://localhost:8080"
	@echo "  - RabbitMQ UI:   http://localhost:15672 (guest/guest)"

docker-down:
	@docker compose down

docker-logs:
	@docker compose logs -f

# ==================== Test ====================

# 测试所有服务
test:
	@go test ./...

# ==================== Clean ====================

clean:
	@rm -rf bin/
	@echo "Cleaned build artifacts"

# ==================== Help ====================

help:
	@echo "Usage:"
	@echo "  make proto          - Generate protobuf Go code"
	@echo "  make build          - Build all services"
	@echo "  make build-user     - Build user-service"
	@echo "  make build-book     - Build book-service"
	@echo "  make build-order    - Build order-service"
	@echo "  make docker-up      - Start all services with Docker"
	@echo "  make docker-down    - Stop all services"
	@echo "  make docker-logs    - View service logs"
	@echo "  make test           - Run all tests"
	@echo "  make clean          - Remove build artifacts"
