# Day 12: Keto 权限控制测试指南

## 启动服务

```bash
docker compose up -d

# 等待所有服务就绪
curl http://localhost:4466/health/alive    # Keto
curl http://localhost:4433/health/alive    # Kratos
curl http://localhost:4444/health/alive    # Hydra
curl http://localhost:4455/health/alive    # Oathkeeper
curl http://localhost:8083/health          # Webhook
curl http://localhost:8084/health          # Admin-service
```

## 测试 1：直接测试 Keto Check API

最直接的验证方式，确认 relation tuple 和传递关系是否正确：

```bash
# charlie (admin) 管理所有书籍 — 通过 Role:admin#members 传递
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Book","object":"*","relation":"managers","subject_id":"charlie"}'
# 预期: {"allowed":true}

# charlie 查看所有订单 — 通过 Group:users#members 传递
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Order","object":"*","relation":"viewers","subject_id":"charlie"}'
# 预期: {"allowed":true}

# bob (author) 编辑书籍 — 通过 Role:author#members 传递
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Book","object":"*","relation":"editors","subject_id":"bob"}'
# 预期: {"allowed":true}

# bob 查看订单 — 通过 Group:users#members 传递
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Order","object":"*","relation":"viewers","subject_id":"bob"}'
# 预期: {"allowed":true}

# 未授权用户无法管理书籍
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Book","object":"*","relation":"managers","subject_id":"unknown-user"}'
# 预期: {"allowed":false}

# charlie (admin) 管理订单 — 通过 Role:admin#members 传递（Order:manage）
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Order","object":"*","relation":"managers","subject_id":"charlie"}'
# 预期: {"allowed":true}

# bob (非 admin) 无法管理订单
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Order","object":"*","relation":"managers","subject_id":"bob"}'
# 预期: {"allowed":false}
```

## 测试 2：Webhook 自动注册

验证新用户注册后自动获得基础权限：

```bash
# 1. 获取注册流程 ID
FLOW_ID=$(curl -s -H "Accept: application/json" \
  http://localhost:4433/self-service/registration/api | jq -r '.id')

# 2. 注册新用户
curl -s -X POST "http://localhost:4433/self-service/registration?flow=$FLOW_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "method": "password",
    "password": "SecureP@ssw0rd!",
    "traits": {
      "email": "newuser@example.com",
      "username": "newuser"
    }
  }'

# 3. 验证用户已被添加到 users 组
curl -s "http://localhost:4466/relation-tuples?namespace=Group&object=users&relation=members" \
  | jq '.relation_tuples[].subject_id'

# 4. 验证新用户可以查看书籍（通过 Group:users 传递）
NEW_USER_ID="<上一步返回的用户 ID>"
curl -s -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d "{\"namespace\":\"Book\",\"object\":\"*\",\"relation\":\"viewers\",\"subject_id\":\"$NEW_USER_ID\"}"
# 预期: {"allowed":true}
```

## 测试 3：查看权限树展开

```bash
# 展开 Book:* 的 managers 关系（可以看到传递链）
curl -s "http://localhost:4466/relation-tuples/expand?namespace=Book&object=*&relation=managers" \
  | python3 -m json.tool

# 展开 Order:* 的 viewers 关系
curl -s "http://localhost:4466/relation-tuples/expand?namespace=Order&object=*&relation=viewers" \
  | python3 -m json.tool
```

## 测试 4：通过 Oathkeeper 测试完整流程

```bash
# 1. 先在浏览器登录获取 session cookie
#    或通过 Kratos API 登录获取 session

# 2. 使用 session cookie 访问受保护的订单 API
curl -v -b "ory_kratos_session=<session-token>" \
  http://localhost/api/v1/orders
# 预期: 200 OK（用户有 Order:read 权限）

# 3. 访问公开的书评查看 API（无需登录）
curl http://localhost/api/v1/reviews/book/123
# 预期: 200 OK（noop authenticator，allow authorizer）
```

## 测试 5：Admin API 测试

```bash
# 使用 admin 用户的 session cookie 访问管理 API
curl -v -X PUT -b "ory_kratos_session=<admin-session-token>" \
  -H "Content-Type: application/json" \
  -d '{"status":"shipped"}' \
  http://localhost/api/v1/admin/orders/1/status
# 预期: 200 OK（admin 有 Order:manage 权限）

# 使用普通用户的 session cookie 访问管理 API
curl -v -X PUT -b "ory_kratos_session=<user-session-token>" \
  -H "Content-Type: application/json" \
  -d '{"status":"shipped"}' \
  http://localhost/api/v1/admin/orders/1/status
# 预期: 403 Forbidden（普通用户没有 Order:manage 权限）
```

## 一键验证脚本

```bash
#!/bin/bash
echo "=== Keto 权限验证 ==="

check() {
  local label=$1
  local ns=$2 obj=$3 rel=$4 sub=$5
  local result=$(curl -s -X POST "http://localhost:4466/relation-tuples/check" \
    -H "Content-Type: application/json" \
    -d "{\"namespace\":\"$ns\",\"object\":\"$obj\",\"relation\":\"$rel\",\"subject_id\":\"$sub\"}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('allowed',False))")
  echo "  $label: $result"
}

check "charlie 管理书籍"   Book  "*" managers  charlie
check "charlie 管理订单"   Order "*" managers  charlie
check "charlie 查看订单"   Order "*" viewers    charlie
check "bob 编辑书籍"       Book  "*" editors    bob
check "bob 查看订单"       Order "*" viewers    bob
check "bob 管理订单"       Order "*" managers   bob    # 预期: False
check "unknown 查看书籍"   Book  "*" viewers    unknown # 预期: False
```

## 常见问题

### Check API 返回 false 但预期是 allowed？

1. 确认 relation tuple 已写入：
   ```bash
   curl http://localhost:4466/relation-tuples | python3 -m json.tool
   ```
2. 检查 namespace 和 relation 名称是否与 OPL 定义一致
3. 检查 subject_id 格式：具体用户用 `"subject_id": "xxx"`，不是 `subject_set`
4. 用 expand 查看传递关系是否生效：
   ```bash
   curl "http://localhost:4466/relation-tuples/expand?namespace=Book&object=*&relation=managers" | python3 -m json.tool
   ```

### Oathkeeper 报错 "authorizer not found"？

检查 `oathkeeper.yml` 中 `keto_engine_acp_ory` 是否已启用：

```yaml
authorizers:
  keto_engine_acp_ory:
    enabled: true
    config:
      base_url: http://keto:4466
      flavor: exact
```

### Webhook 没有自动将用户添加到 users 组？

1. 检查 Kratos 日志：`docker compose logs kratos | grep webhook`
2. 检查 webhook 日志：`docker compose logs webhook`
3. 手动测试 webhook：
   ```bash
   curl -X POST http://localhost:8083/webhooks/registration \
     -H "Content-Type: application/json" \
     -d '{"identity":{"id":"test-123","traits":{"email":"test@example.com","username":"test"}}}'
   ```

### Group 成员关系不生效？

1. 确认 Group 关系元组已写入：
   ```bash
   curl "http://localhost:4466/relation-tuples?namespace=Group" | python3 -m json.tool
   ```
2. 检查 `Group:users#members` 关系是否正确（注意：没有 `groups.json` 文件，Group 成员由 webhook 动态写入）

### Keto 迁移失败？

```bash
# 检查迁移状态
docker exec bookstore-keto-migrate-1 keto migrate status -c /etc/config/keto/keto.yml
```

### PostgreSQL 启动失败？

PostgreSQL 18+ 需要挂载到 `/var/lib/postgresql`（不是 `/var/lib/postgresql/data`）：
```yaml
volumes:
  - postgres_keto_data:/var/lib/postgresql
```
