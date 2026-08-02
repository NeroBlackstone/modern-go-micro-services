# Day 13 - 服务间认证：测试与调试

## 编译验证

```bash
# 确保代码编译通过
go build ./...

# 检查所有包
go vet ./...
```

## 启动服务

```bash
# 设置共享密钥（可选，默认有开发环境值）
export SERVICE_AUTH_SECRET="my-super-secret-key"

# 启动所有服务
docker compose up -d

# 检查服务状态
docker compose ps
```

## 验证服务间认证

### 1. 检查日志

查看 order-service 日志，确认拦截器已加载：

```bash
docker compose logs order-service | grep "service auth"
```

查看 book-service 日志，确认认证拦截器工作：

```bash
docker compose logs book-service | grep "service auth"
```

### 2. 正常流程测试

通过 Traefik 发送请求（需要先完成 Kratos 登录获取 session cookie）：

```bash
# 1. 注册用户
curl -X POST http://localhost:4433/self-service/registration/api \
  -H "Content-Type: application/json" \
  -d '{"method":"password","password":"Test1234!","traits":{"email":"test@example.com","name":"Test User"}}'

# 2. 登录获取 session cookie
curl -X POST http://localhost:4433/self-service/login/api \
  -H "Content-Type: application/json" \
  -d '{"method":"password","password":"Test1234!","identifier":"test@example.com"}' \
  -c cookies.txt

# 3. 创建订单（触发 order-service → book-service 调用）
curl -X POST http://localhost/api/v1/orders \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"items":[{"book_id":1,"quantity":1}]}'
```

### 3. 验证拦截器日志

在 order-service 日志中应该看到：

```
service auth token attached method=/bookstore.book.v1.BookService/GetBooks caller=order-service target=book-service
```

在 book-service 日志中应该看到：

```
service auth passed method=/bookstore.book.v1.BookService/GetBooks caller=order-service caller_user=xxx
```

### 4. 测试认证失败

尝试直接调用 book-service（不带 token）：

```bash
# 使用 grpcurl 直接调用 book-service（绕过 order-service）
grpcurl -plaintext localhost:9092 bookstore.book.v1.BookService/GetBook \
  -d '{"id": 1}'
```

预期结果：返回 `Unauthenticated` 错误。

### 5. 测试篡改 Token

使用错误的密钥生成 token：

```bash
# 生成错误的 JWT
python3 -c "
import jwt, time
payload = {
    'iss': 'order-service',
    'aud': 'book-service',
    'sub': 'order-service',
    'iat': int(time.time()),
    'exp': int(time.time()) + 300
}
token = jwt.encode(payload, 'wrong-secret', algorithm='HS256')
print(token)
" > wrong_token.txt

# 使用 grpcurl + 错误 token 调用
grpcurl -plaintext -H "authorization: Bearer $(cat wrong_token.txt)" \
  localhost:9092 bookstore.book.v1.BookService/GetBook \
  -d '{"id": 1}'
```

预期结果：返回 `Unauthenticated: invalid service token`。

## 调试技巧

### 1. 启用调试日志

修改配置文件，将日志级别改为 debug：

```yaml
log:
  level: debug
```

### 2. 查看 gRPC 拦截器执行顺序

在拦截器中添加日志，观察执行顺序：

```
Tracing interceptor started
ServiceAuth interceptor: generating token
CircuitBreaker interceptor: checking state
Retry interceptor: attempt 1
RateLimiter interceptor: checking limit
```

### 3. 检查 gRPC metadata

使用 grpcurl 的 `-v` 标志查看请求 metadata：

```bash
grpcurl -v -plaintext localhost:9092 ...
```

### 4. 常见问题排查

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| `missing authorization in metadata` | 客户端拦截器未加载 | 检查 main.go 中拦截器链配置 |
| `invalid service token` | 密钥不匹配 | 确保两个服务的 `SERVICE_AUTH_SECRET` 相同 |
| `token audience mismatch` | aud 字段错误 | 检查 `target_service` 配置 |
| `token is expired` | TTL 过短 | 调整 `ttl` 配置 |
| 拦截器未执行 | 拦截器链顺序错误 | 确保 serviceauth 在 tracing 之后 |

### 5. 使用 Prometheus 监控

服务间认证的失败会反映在 gRPC 指标中：

```
# 认证失败的请求
grpc_server_handled_total{grpc_code="Unauthenticated",grpc_method="GetBook",...}

# 正常请求
grpc_server_handled_total{grpc_code="OK",grpc_method="GetBook",...}
```

## 安全检查清单

- [ ] 共享密钥是否足够随机（至少 32 字符）
- [ ] 生产环境是否使用了强密钥（不是默认值）
- [ ] Token TTL 是否合理（不要太长）
- [ ] 是否记录了认证失败的日志
- [ ] 是否有监控告警（认证失败率）

## 下一步

在下一个文档中，我们将更新架构图并总结 Day 13 的学习内容。
