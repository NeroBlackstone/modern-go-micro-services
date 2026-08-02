#!/bin/bash
# 通过 hydra import client 导入 OAuth2 Client（幂等：已存在则跳过）
# 使用方法: ./scripts/create-hydra-client.sh

set -euo pipefail

HYDRA_ADMIN_URL="${HYDRA_ADMIN_URL:-http://localhost:4445}"
CLIENT_NAME="bookstore-web-client"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLIENTS_JSON="$SCRIPT_DIR/../configs/hydra/clients.json"

echo "=========================================="
echo "  导入 OAuth2 Client (幂等)"
echo "=========================================="
echo ""
echo "Hydra Admin API: $HYDRA_ADMIN_URL"
echo "Client 配置文件: $CLIENTS_JSON"
echo ""

# 检查配置文件是否存在
if [ ! -f "$CLIENTS_JSON" ]; then
    echo "❌ Client 配置文件不存在: $CLIENTS_JSON"
    exit 1
fi

# 检查 Hydra 是否可访问
echo "检查 Hydra Admin API..."
if ! curl -sf "$HYDRA_ADMIN_URL/health/alive" > /dev/null 2>&1; then
    echo "❌ 无法连接到 Hydra Admin API: $HYDRA_ADMIN_URL"
    echo "请确保 Hydra 服务已启动: docker compose up -d hydra"
    exit 1
fi
echo "✅ Hydra Admin API 可访问"
echo ""

# 幂等：查询是否已存在同名 client
echo "检查 Client 是否已存在: $CLIENT_NAME"
EXISTING=$(curl -sf "$HYDRA_ADMIN_URL/admin/clients" | jq -r --arg name "$CLIENT_NAME" \
    '.[] | select(.client_name == $name) | .client_id' 2>/dev/null || true)

if [ -n "$EXISTING" ]; then
    CLIENT_ID="$EXISTING"
    echo "✅ Client 已存在，跳过导入"
    echo ""
    echo "  Client ID: $CLIENT_ID"
    echo "  ⚠️  Client Secret 仅在首次创建时返回，请查看 docker-compose.yml 或首次运行时的输出"
else
    echo "⏳ Client 不存在，导入中..."
    echo ""

    # 使用 hydra import client 导入（容器内路径），从输出中解析 secret
    IMPORT_OUTPUT=$(docker compose exec hydra \
        hydra import client /etc/config/hydra/clients.json \
        --endpoint http://127.0.0.1:4445 \
        --format json 2>&1)

    # 从 import 输出解析 client_id 和 client_secret
    CLIENT_ID=$(echo "$IMPORT_OUTPUT" | jq -r '.client_id')
    CLIENT_SECRET=$(echo "$IMPORT_OUTPUT" | jq -r '.client_secret')

    echo ""
    echo "✅ OAuth2 Client 导入成功！"
    echo ""
    echo "  Client ID:     $CLIENT_ID"
    echo "  Client Secret: $CLIENT_SECRET"
fi

echo ""
echo "=========================================="
echo "  配置信息"
echo "=========================================="
echo ""
echo "将以下环境变量更新到 docker-compose.yml 的 oauth-client-demo 服务："
echo ""
echo "  - CLIENT_ID=$CLIENT_ID"
if [ -n "${CLIENT_SECRET:-}" ] && [ "$CLIENT_SECRET" != "null" ]; then
    echo "  - CLIENT_SECRET=$CLIENT_SECRET"
else
    echo "  - CLIENT_SECRET=<从首次导入时获取>"
fi
echo ""
echo "=========================================="
echo "  测试步骤"
echo "=========================================="
echo ""
echo "1. 启动服务: docker compose up -d"
echo "2. 先通过 Kratos 注册/登录用户"
echo "3. 访问 http://localhost:8082"
echo "4. 点击「使用 OAuth2 登录」"
echo "5. 完成授权流程"
echo ""
