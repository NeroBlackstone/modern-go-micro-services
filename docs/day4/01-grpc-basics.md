# Day 4 - gRPC 基础

## 什么是 gRPC？

gRPC 是 Google 开发的高性能远程过程调用（RPC）框架，基于 HTTP/2 协议和 Protocol Buffers 序列化。

### gRPC vs REST

| 特性 | gRPC | REST |
|------|------|------|
| 协议 | HTTP/2 | HTTP/1.1 |
| 序列化 | Protocol Buffers（二进制） | JSON（文本） |
| 性能 | 高（二进制序列化 + HTTP/2 多路复用） | 中等 |
| 接口定义 | `.proto` 文件强类型 | 通常用 Swagger/OpenAPI |
| 流式传输 | 原生支持（unary, server/client/bidirectional streaming） | 需要 WebSocket 或 SSE |
| 浏览器支持 | 需要 gRPC-Web 代理 | 原生支持 |
| 适用场景 | 微服务间内部通信 | 对外 API、浏览器客户端 |

## Protocol Buffers (Protobuf)

Protobuf 是 gRPC 的默认序列化格式，也是一种接口定义语言（IDL）。

### 基本语法

```protobuf
edition = "2024";

package bookstore.book.v1;

option go_package = "modern-micro-services/gen/bookstore/book/v1;bookv1";

// 定义服务
service BookService {
  rpc GetBook(GetBookRequest) returns (GetBookResponse);
  rpc GetBooks(GetBooksRequest) returns (GetBooksResponse);
  rpc DeductStock(DeductStockRequest) returns (DeductStockResponse);
}

// 定义消息（相当于 struct）
message GetBookRequest {
  uint32 id = 1;
}

message GetBookResponse {
  uint32 id = 1;
  string title = 2;
  string author = 3;
  double price = 4;
  int32 stock = 5;
}
```

### Protobuf 类型映射

| Protobuf 类型 | Go 类型 | 说明 |
|---------------|---------|------|
| `int32` | `int32` | 32位整数 |
| `int64` | `int64` | 64位整数 |
| `uint32` | `uint32` | 无符号32位 |
| `uint64` | `uint64` | 无符号64位 |
| `float` | `float32` | 单精度浮点 |
| `double` | `float64` | 双精度浮点 |
| `string` | `string` | 字符串 |
| `bool` | `bool` | 布尔值 |
| `bytes` | `[]byte` | 字节数组 |
| `repeated` | `[]T` | 数组/切片 |

### 字段编号

Protobuf 使用字段编号（而非字段名）来标识字段，这是实现前向/后向兼容性的关键：

```protobuf
message User {
  uint32 id = 1;      // 字段编号 1
  string name = 2;    // 字段编号 2
  // 新增字段用新编号
  string avatar = 3;  // 字段编号 3
}
```

**规则：**
- 字段编号一旦使用就不能更改
- 1-15 用 1 字节编码，优先用于高频字段
- 16-2047 用 2 字节编码
- 删除字段时保留编号（`reserved`）

## Go gRPC 开发流程

### 1. 安装工具

```bash
# 安装 protoc 编译器
# Linux: apt install protobuf-compiler
# macOS: brew install protobuf

# 安装 Go 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 2. 编写 .proto 文件

```protobuf
edition = "2024";
package mypackage;
option go_package = "myproject/gen/mypackage;mypackage";

service MyService {
  rpc MyMethod(MyRequest) returns (MyResponse);
}

message MyRequest { string name = 1; }
message MyResponse { string message = 1; }
```

### 3. 生成 Go 代码

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       mypackage.proto
```

### 4. 实现 gRPC Server

```go
type server struct {
    pb.UnimplementedMyServiceServer
}

func (s *server) MyMethod(ctx context.Context, req *pb.MyRequest) (*pb.MyResponse, error) {
    return &pb.MyResponse{Message: "Hello " + req.Name}, nil
}

func main() {
    lis, _ := net.Listen("tcp", ":9090")
    grpcServer := grpc.NewServer()
    pb.RegisterMyServiceServer(grpcServer, &server{})
    grpcServer.Serve(lis)
}
```

### 5. gRPC Client 调用

```go
conn, _ := grpc.NewClient("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := pb.NewMyServiceClient(conn)
resp, _ := client.MyMethod(context.Background(), &pb.MyRequest{Name: "World"})
fmt.Println(resp.Message) // "Hello World"
```

## 项目中的 Proto 定义

### user/v1/user.proto

```protobuf
service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
}
```

**用途：** 其他微服务通过 gRPC 调用获取用户信息（如 order-service 需要显示用户信息）

### book/v1/book.proto

```protobuf
service BookService {
  rpc GetBook(GetBookRequest) returns (GetBookResponse);
  rpc GetBooks(GetBooksRequest) returns (GetBooksResponse);
  rpc DeductStock(DeductStockRequest) returns (DeductStockResponse);
  rpc RestoreStock(RestoreStockRequest) returns (RestoreStockResponse);
}
```

**用途：**
- `GetBook`/`GetBooks`: 查询图书信息
- `DeductStock`: 下单时扣减库存
- `RestoreStock`: Saga 补偿时恢复库存
