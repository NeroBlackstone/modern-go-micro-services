# Day 14 - 测试指南：验证 Vault 集成

## 前置条件

```bash
# 确保 Docker 和 Docker Compose 已安装
docker --version
docker compose version

# 确保项目根目录是当前目录
cd /path/to/modern-micro-services
```

## 步骤一：启动 Vault 和初始化密钥

```bash
# 1. 只启动 Vault 和 vault-init
docker compose up -d vault vault-init

# 2. 等待初始化完成
docker compose logs -f vault-init
```

初始化完成后，日志会输出：

```
=== 初始化完成 ===
Vault UI: http://localhost:8200/ui
Root Token: my-root-token

密钥已写入：
Keys
----
database/
jwt
rabbitmq
service-auth
```

## 步骤二：启动所有服务

```bash
docker compose up -d
```

## 步骤四：验证 Vault UI

```bash
# 打开浏览器访问 Vault UI
open http://localhost:8200/ui
# 或
xdg-open http://localhost:8200/ui
```

登录 Token: `my-root-token`

在 UI 中可以查看：
- **Secrets** → `secret/` → 查看所有写入的密钥
- **Policies** → 查看每个服务的权限策略（生产环境目标）

## 步骤五：验证密钥读取

```bash
# 使用 Vault CLI 验证密钥已写入
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='my-root-token'

# 查看书库数据库密钥
vault kv get secret/database/book

# 查看订单数据库密钥
vault kv get secret/database/order

# 查看 RabbitMQ 密钥
vault kv get secret/rabbitmq

# 查看 JWT 密钥
vault kv get secret/jwt

# 查看服务间认证密钥
vault kv get secret/service-auth
```

## 步骤六：验证服务日志

```bash
# 查看 book-service 日志，确认 Vault 连接成功
docker compose logs book-service | grep -i vault

# 期望输出：
# vault client connected, overlaying secrets
# database config overlaid from vault path=secret/data/database/book

# 查看 order-service 日志
docker compose logs order-service | grep -i vault

# 查看 admin-service 日志
docker compose logs admin-service | grep -i vault
```

## 步骤七：验证业务功能

```bash
# 测试图书 API
curl http://localhost/api/v1/books

# 测试订单 API（需要认证）
curl -H "Authorization: Bearer <token>" http://localhost/api/v1/orders

# 测试健康检查
curl http://localhost:9093/health  # book-service metrics
```

## 步骤八：验证优雅降级

```bash
# 停止 Vault 服务
docker compose stop vault

# 重启 book-service（此时 Vault 不可用）
docker compose restart book-service

# 查看日志，应该看到 fallback 到本地配置
docker compose logs book-service | grep -i vault

# 期望输出：
# vault not available, using local config
# （服务仍然正常启动，使用 YAML 中的本地配置）
```

## 步骤九：验证 Vault Token 认证

```bash
# 使用 root token 直接读取密钥
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='my-root-token'

# 验证可以读取所有密钥
vault kv get secret/database/book
vault kv get secret/database/order
vault kv get secret/service-auth

# 查看 token 信息
vault token lookup
```

## 常见问题

### Q: vault-init 一直等待 Vault 启动

```bash
# 检查 Vault 健康状态（注意：dev 模式需要设置 VAULT_ADDR）
docker compose exec vault env VAULT_ADDR=http://127.0.0.1:8200 vault status

# 如果 Vault 未就绪，等待更长时间
docker compose logs vault
```

### Q: Vault 健康检查一直失败（unhealthy）

```bash
# 原因：vault status 默认走 HTTPS，但 dev 模式监听 HTTP
# 解决：healthcheck 中设置 VAULT_ADDR
# compose.yml 中的正确写法：
#   test: ["CMD-SHELL", "VAULT_ADDR=http://127.0.0.1:8200 vault status -format=json | grep -q initialized"]
```

### Q: 服务启动时 Vault 连接失败

```bash
# 检查环境变量是否正确设置
docker compose exec book-service env | grep VAULT

# 检查 Vault 是否可达
docker compose exec book-service wget -qO- http://vault:8200/v1/sys/health
```

### Q: Vault Token 认证失败

```bash
# 检查 Vault 日志
docker compose logs vault | grep -i error

# 手动测试 Token 认证
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='my-root-token'
vault token lookup
```

### Q: 密钥读取返回空

```bash
# 验证密钥路径（KV v2 需要加 data/ 前缀）
vault kv get secret/data/database/book

# 列出所有密钥路径
vault kv list secret/
```

## 清理

```bash
# 停止所有服务
docker compose down

# 清除 Vault 数据（重新开始）
docker volume rm modern-micro-services_vault_data

# 完全清理（包括数据库数据）
docker compose down -v
```

## 学习检查清单

- [ ] 能解释 Vault 的核心概念（Server、Auth、Secrets Engine、Policy）
- [ ] 能说明 Token 认证的流程（VAULT_TOKEN → 直接访问）
- [ ] 能解释 KV v2 引擎的路径结构（secret/data/xxx）
- [ ] 能解释最小权限原则在策略中的体现（生产环境目标）
- [ ] 能说明优雅降级的工作原理
- [ ] 能独立完成 Vault 集成的测试验证
- [ ] 了解 Vault healthcheck 中 VAULT_ADDR 的必要性
- [ ] 了解 Docker 网络配置对容器间通信的影响
