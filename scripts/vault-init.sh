#!/bin/sh
set -e

echo "=== Vault 初始化脚本 ==="

# 等待 Vault 就绪
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
