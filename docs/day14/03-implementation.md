# Day 14 - 代码实现：Vault 集成详解

## 新增文件结构

```
internal/vaultclient/
├── client.go          # Vault 客户端封装（使用官方 SDK）

configs/vault/
└── vault.hcl          # Vault 服务器配置

scripts/
└── vault-init.sh      # 初始化脚本（写入密钥到 KV v2）
```

## 核心实现

### 1. Vault 客户端封装

`internal/vaultclient/client.go` 封装了 Vault 官方 Go SDK 的常用操作：

```go
package vaultclient

import (
    vault "github.com/hashicorp/vault/api"
)

// NewClientFromEnv 从环境变量创建 Vault 客户端
// 需要 VAULT_ADDR 和 VAULT_TOKEN 环境变量
func NewClientFromEnv() *vault.Client {
    addr := os.Getenv("VAULT_ADDR")
    token := os.Getenv("VAULT_TOKEN")
    if addr == "" || token == "" {
        return nil
    }

    cfg := vault.DefaultConfig()
    cfg.Address = addr

    client, err := vault.NewClient(cfg)
    if err != nil {
        return nil
    }
    client.SetToken(token)
    return client
}

// GetKVSecret 读取 KV v2 密钥，返回 data.data 下的 map
func GetKVSecret(client *vault.Client, path string) (map[string]string, error) {
    secret, err := client.Logical().Read(path)
    if err != nil {
        return nil, fmt.Errorf("read %s: %w", path, err)
    }
    if secret == nil {
        return nil, fmt.Errorf("secret not found: %s", path)
    }

    // KV v2 返回结构：data.data 包含实际数据
    data, ok := secret.Data["data"].(map[string]interface{})
    if !ok {
        return nil, fmt.Errorf("invalid format at %s", path)
    }

    result := make(map[string]string, len(data))
    for k, v := range data {
        result[k] = fmt.Sprintf("%v", v)
    }
    return result, nil
}
```

**关键点：**

- 使用官方 `github.com/hashicorp/vault/api` SDK，不重复造轮子
- 使用 `VAULT_TOKEN` 直接认证（Dev 模式简化方案）
- KV v2 的响应结构是 `data.data`（外层 `data` 是 Vault 包装）
- 函数直接操作 `*vault.Client`，无需额外封装 struct

### 2. 环境变量读取

```go
// NewClientFromEnv 从环境变量创建 Vault 客户端
func NewClientFromEnv() *vault.Client {
    addr := os.Getenv("VAULT_ADDR")
    token := os.Getenv("VAULT_TOKEN")
    if addr == "" || token == "" {
        return nil  // Vault 未配置，返回 nil 允许 fallback
    }

    cfg := vault.DefaultConfig()
    cfg.Address = addr

    client, err := vault.NewClient(cfg)
    if err != nil {
        return nil
    }
    client.SetToken(token)
    return client
}
```

### 3. 配置覆盖（Overlay）

```go
// OverlayDatabaseConfig 从 Vault 覆盖数据库配置
func OverlayDatabaseConfig(client *vault.Client, kvPath string,
    host *string, port *int, user *string, password *string, dbname *string,
    logger *zap.Logger) {

    if client == nil {
        return  // Vault 不可用，跳过
    }

    secrets, err := GetKVSecret(client, kvPath)
    if err != nil {
        logger.Warn("vault read failed, using local config",
            zap.String("path", kvPath), zap.Error(err))
        return  // 读取失败，保留本地配置
    }

    // 逐字段覆盖（只覆盖 Vault 中有的值）
    if v := secrets["host"]; v != "" {
        *host = v
    }
    if v := secrets["port"]; v != "" {
        if p, err := strconv.Atoi(v); err == nil {
            *port = p
        }
    }
    if v := secrets["user"]; v != "" {
        *user = v
    }
    if v := secrets["password"]; v != "" {
        *password = v
    }
    if v := secrets["dbname"]; v != "" {
        *dbname = v
    }
    logger.Info("database overlaid from vault", zap.String("path", kvPath))
}

// OverlayString 从 Vault 密钥中覆盖单个字符串字段
func OverlayString(secrets map[string]string, key string, target *string) {
    if val, ok := secrets[key]; ok && val != "" {
        *target = val
    }
}
```

### 4. 服务集成（book-service）

```go
// cmd/book-service/main.go

// 1. 加载本地配置
cfg, err := config.Load("configs/book-service.yaml")

// 2. 尝试连接 Vault
vaultClient := vaultclient.NewClientFromEnv()
if vaultClient != nil {
    logger.Info("vault client connected, overlaying secrets")
    kvPath := os.Getenv("VAULT_KV_PATH")
    // 3. 从 Vault 覆盖数据库配置
    vaultclient.OverlayDatabaseConfig(
        vaultClient,
        kvPath,                // "secret/data/database/book"
        &cfg.Database.Host,
        &cfg.Database.Port,
        &cfg.Database.User,
        &cfg.Database.Password,
        &cfg.Database.DBName,
        logger,
    )

    // 4. 从 Vault 覆盖服务间认证密钥
    if secrets, err := vaultclient.GetKVSecret(vaultClient, "secret/data/service-auth"); err == nil {
        vaultclient.OverlayString(secrets, "shared_secret", &cfg.ServiceAuth.SharedSecret)
    }
} else {
    logger.Info("vault not available, using local config")
}

// 5. 使用最终配置初始化数据库连接
db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
```

### 5. Vault 初始化脚本

`scripts/vault-init.sh` 在 Docker 容器中执行：

```bash
#!/bin/sh
set -e

echo "=== Vault 初始化脚本 ==="

# 等待 Vault 就绪（注意 JSON 中 "initialized": true 有空格）
until vault status -format=json 2>/dev/null | grep -q '"initialized":\s*true'; do
  echo "等待 Vault 启动..."
  sleep 2
done
echo "Vault 已就绪"

# 1. 启用 KV v2 引擎
vault secrets enable -path=secret kv-v2 2>/dev/null || echo "KV v2 已启用"

# 2. 写入密钥
vault kv put secret/database/book \
  host=postgres-book port=5432 user=bookstore password=bookstore123 dbname=book_db sslmode=disable

vault kv put secret/database/order \
  host=postgres-order port=5432 user=bookstore password=bookstore123 dbname=order_db sslmode=disable

vault kv put secret/rabbitmq \
  host=rabbitmq port=5672 user=guest password=guest

vault kv put secret/jwt \
  secret=your-secret-key-change-in-production expire_hours=72

vault kv put secret/service-auth \
  shared_secret=super-secret-key-change-in-production

echo ""
echo "=== 初始化完成 ==="
echo "Vault UI: http://localhost:8200/ui"
echo "Root Token: my-root-token"
echo ""
echo "密钥已写入："
vault kv list secret/
```

> **注意：** `vault status -format=json` 输出的 `"initialized": true` 中间有空格，
> 因此 grep 模式必须使用 `'"initialized":\s*true'` 而非 `'"initialized":true'`。

## Docker Compose 配置

```yaml
vault:
  image: hashicorp/vault:2.0.3
  container_name: bookstore-vault
  ports:
    - "8200:8200"
  cap_add:
    - IPC_LOCK
  environment:
    VAULT_DEV_ROOT_TOKEN_ID: "my-root-token"
    VAULT_DEV_LISTEN_ADDRESS: "0.0.0.0:8200"
  command: server -dev -dev-root-token-id=my-root-token
  healthcheck:
    # vault status 默认走 HTTPS，但 dev 模式监听 HTTP，需要设置 VAULT_ADDR
    test: ["CMD-SHELL", "VAULT_ADDR=http://127.0.0.1:8200 vault status -format=json | grep -q initialized"]
    interval: 10s
    timeout: 5s
    retries: 5
  networks:
    - bookstore

vault-init:
  image: hashicorp/vault:2.0.3
  environment:
    VAULT_ADDR: "http://vault:8200"
    VAULT_TOKEN: "my-root-token"
  volumes:
    - ./scripts/vault-init.sh:/vault-init.sh:ro
  command: sh /vault-init.sh
  depends_on:
    vault:
      condition: service_healthy
  # 必须和 vault 在同一网络中，否则无法通过 DNS 解析 "vault" 主机名
  networks:
    - bookstore

book-service:
  environment:
    VAULT_ADDR: http://vault:8200
    VAULT_TOKEN: my-root-token
    VAULT_KV_PATH: secret/data/database/book
  depends_on:
    vault-init:
      condition: service_completed_successfully
  # 所有需要访问 Vault 和其他基础设施的服务都必须加入 bookstore 网络
  networks:
    - bookstore
```

> **踩坑记录：**
> 1. Vault 的 `healthcheck` 使用 `vault status` 时，必须设置 `VAULT_ADDR=http://127.0.0.1:8200`，
>    否则命令默认走 HTTPS 导致健康检查一直失败。
> 2. `vault-init`、`book-service`、`order-service`、`admin-service` 以及它们依赖的
>    `postgres-book`、`postgres-order`、`redis`、`consul` 都必须加入同一个 `bookstore` 网络，
>    否则容器之间无法通过 Docker DNS 互相解析。

## 数据流

```
启动时：
  本地 YAML ──读取──▶ 内存中的 Config
                          │
  Vault ──Token认证──▶ 读取密钥
                          │
  密钥数据 ──Overlay──▶ 覆盖 Config 中的密码字段
                          │
  最终 Config ──初始化──▶ DB 连接、RabbitMQ 连接等

运行时：
  请求 ──▶ 服务 ──▶ 使用 Config 中的凭证连接 DB
                    （Vault 不参与运行时请求处理）
```

## 关键设计决策

| 决策 | 选择 | 原因 |
|------|------|------|
| Vault 模式 | Dev Mode | 学习项目，简化部署 |
| 认证方式 | VAULT_TOKEN 直接认证 | Dev 模式下最简单，生产环境应改用 AppRole |
| 密钥引擎 | KV v2 | 最简单，手动写入即可 |
| 集成方式 | 启动时覆盖 | 不影响运行时性能 |
| 降级策略 | 优雅降级 | Vault 不可用时仍能启动 |

## 下一步

在下一个文档中，我们将介绍如何测试和验证整个 Vault 集成。
