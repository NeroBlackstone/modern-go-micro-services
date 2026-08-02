# Day 12: Ory Keto 配置与集成

## 架构概览

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│   Client     │───→│   Traefik    │───→│  Oathkeeper  │
│ (浏览器/API) │    │ (API Gateway)│    │ (认证+授权)   │
└─────────────┘    └──────────────┘    └──────┬───────┘
                                              │
                          ┌───────────────────┤
                          │                   │
                    ┌─────▼─────┐      ┌──────▼──────┐
                    │  Kratos   │      │    Keto     │
                    │ (身份管理) │      │ (权限控制)   │
                    └─────┬─────┘      └──────┬──────┘
                          │                   │
                    after registration        │
                          │                   │
                    ┌─────▼─────┐             │
                    │  Webhook  │─────────────┘
                    │  (胶水层) │  PUT /admin/relation-tuples
                    └───────────┘
```

**关键集成点：**
- Oathkeeper 内置 `keto_engine_acp_ory` authorizer，直接调用 Keto Check API
- Kratos 注册后通过 webhook 将用户添加到 `Group:users`，自动获得基础查看权限

## Docker Compose 服务

Keto 相关新增 4 个服务（`compose.yml`）：

| 服务 | 镜像 | 端口 | 职责 |
|------|------|------|------|
| `postgres-keto` | postgres:18-alpine | 5437 | Keto 数据库 |
| `keto-migrate` | oryd/keto:v26.2.0 | - | 一次性数据库迁移 |
| `keto` | oryd/keto:v26.2.0 | 4466/4467/4468 | Read/Write/Metrics API |
| `keto-init` | oryd/keto:v26.2.0 | - | 启动时批量导入 relation tuples |

启动顺序由 `depends_on` 控制：

```
postgres-keto (healthy)
  → keto-migrate (completed)
    → keto (started)
      → keto-init (completed) — 导入 relation tuples
```

## Keto 配置文件

`configs/keto/keto.yml`：

```yaml
dsn: "postgres://keto:keto_secret@postgres-keto:5432/keto?sslmode=disable"

serve:
  read:
    port: 4466    # Read API（查询 + 健康检查）
  write:
    port: 4467    # Write API（写入 relation tuple）
  metrics:
    port: 4468    # Metrics API（Prometheus 抓取）

namespaces:
  location: file:///etc/config/keto/namespaces.keto.ts  # OPL 定义
```

## Namespace 设计（OPL）

使用 Ory Permission Language 定义权限模型，文件 `configs/keto/namespaces.keto.ts`：

```typescript
import { Namespace, SubjectSet, Context } from "@ory/keto-namespace-types"

class User implements Namespace {}          // 用户身份
class Group implements Namespace {          // 用户组
  related: { members: User[] }
}
class Role implements Namespace {           // 角色
  related: { members: User[] }
}
class Book implements Namespace {           // 书籍
  related: {
    managers: SubjectSet<Role, "members">[]
    editors: SubjectSet<Role, "members">[]
    viewers: (User | SubjectSet<Role, "members"> | SubjectSet<Group, "members">)[]
  }
  permits = {
    manage: (ctx) => this.related.managers.includes(ctx.subject),
    edit:   (ctx) => this.related.managers.includes(ctx.subject) || this.related.editors.includes(ctx.subject),
    read:   (ctx) => this.related.viewers.includes(ctx.subject),
  }
}
class Order implements Namespace {          // 订单
  related: {
    managers: SubjectSet<Role, "members">[]
    viewers: (User | SubjectSet<Role, "members"> | SubjectSet<Group, "members">)[]
  }
  permits = {
    manage: (ctx) => this.related.managers.includes(ctx.subject),
    read:   (ctx) => this.related.viewers.includes(ctx.subject),
  }
}
class Review implements Namespace {         // 书评
  related: {
    managers: SubjectSet<Role, "members">[]
    writers: (User | SubjectSet<Role, "members"> | SubjectSet<Group, "members">)[]
  }
  permits = {
    manage: (ctx) => this.related.managers.includes(ctx.subject),
    write:  (ctx) => this.related.writers.includes(ctx.subject),
  }
}
```

## 关系元组初始化

权限数据以 JSON 文件形式存储在 `configs/keto/relation-tuples/`，由 `keto-init` 容器在启动时批量导入。

### 文件结构

```
configs/keto/relation-tuples/
├── roles.json     # 角色成员: Role:admin → charlie
├── books.json     # 书籍权限: Book:* managers/editors/viewers
├── orders.json    # 订单权限: Order:* managers/viewers
├── reviews.json   # 书评权限: Review:* managers/writers
└── admin.json     # 管理权限: Order:* managers（admin 角色）
```

### 示例：roles.json

```json
[{"namespace":"Role","object":"admin","relation":"members","subject_id":"charlie"}]
```

### 示例：books.json

```json
[
  {"namespace":"Book","object":"*","relation":"managers","subject_set":{"namespace":"Role","object":"admin","relation":"members"}},
  {"namespace":"Book","object":"*","relation":"editors","subject_set":{"namespace":"Role","object":"author","relation":"members"}},
  {"namespace":"Book","object":"*","relation":"viewers","subject_set":{"namespace":"Group","object":"users","relation":"members"}}
]
```

### 导入命令

```bash
# keto-init 容器自动执行（启动时）
relation-tuple create -f ./relation-tuples --insecure-disable-transport-security
# 环境变量: KETO_WRITE_REMOTE=keto:4467, KETO_READ_REMOTE=keto:4466
```

## 默认权限汇总

| 用户 | 角色/组 | 获得的权限 |
|------|---------|-----------|
| charlie | `Role:admin#members` | 管理所有资源（通过 admin 角色传递） |
| bob | `Role:author#members` | 编辑书籍（通过 author 角色传递） |
| 新注册用户 | `Group:users#members`（webhook 自动添加） | 查看书籍、订单，发布书评 |

## Webhook 服务

### 工作原理

1. 用户注册时，Kratos 触发 `web_hook`（`configs/kratos/kratos.yml`）
2. Webhook 服务调用 Keto Write API，将用户添加到 `Group:users`
3. 用户自动获得基础查看权限

### 代码实现

`internal/webhook/registration_hook.go`：

```go
func (k *KetoClient) AddUserToGroup(userID, group string) error {
    tuple := map[string]interface{}{
        "namespace":  "Group",
        "object":     group,
        "relation":   "members",
        "subject_id": userID,
    }
    jsonData, _ := json.Marshal(tuple)
    url := fmt.Sprintf("%s/admin/relation-tuples", k.WriteURL)
    req, _ := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))  // PUT 方法
    req.Header.Set("Content-Type", "application/json")
    resp, err := k.Client.Do(req)
    // ...
}
```

### Kratos 配置

`configs/kratos/kratos.yml` 中注册后 webhook：

```yaml
selfservice:
  flows:
    registration:
      after:
        password:
          hooks:
            - hook: web_hook
              config:
                url: http://webhook:8080/webhooks/registration
                method: POST
                body: base64://ZnVuY3Rpb24oY3R4KSB7CiAgaWRlbnRpdHk6IGN0eC5pZGVudGl0eQp9Cg==
                response:
                  ignore: true
```

## Oathkeeper 集成

### keto_engine_acp_ory 配置

`configs/oathkeeper/oathkeeper.yml` 中新增 authorizer：

```yaml
authorizers:
  keto_engine_acp_ory:
    enabled: true
    config:
      base_url: http://keto:4466
      flavor: exact
      required_action: ""
      required_resource: ""
```

### 访问规则

`configs/oathkeeper/rules.json` 中，受保护路由使用 `keto_engine_acp_ory`：

| 规则 ID | 匹配路径 | authorizer | required_action |
|---------|----------|-----------|----------------|
| `protected-order-routes` | `/api/v1/orders` (GET/POST) | `keto_engine_acp_ory` | `Order:read` |
| `admin-order-routes` | `/api/v1/admin/orders/*` (PUT) | `keto_engine_acp_ory` | `Order:manage` |
| `protected-review-routes` | `/api/v1/reviews` (POST) | `keto_engine_acp_ory` | `Review:write` |
| `public-review-routes` | `/api/v1/reviews/book/*` (GET) | `allow` | - |
| `order-health-check` | `/health` | `allow` | - |

### 端口汇总

| 端口 | 服务 | 用途 |
|------|------|------|
| 4466 | keto | Read API（查询 + 健康检查） |
| 4467 | keto | Write API（写入 relation tuple） |
| 4468 | keto | Metrics API |
| 8083 | webhook | 注册后 webhook 服务 |
| 8084 | admin-service | 管理 API（订单状态更新等） |
| 5437 | postgres-keto | Keto 数据库 |

## 添加管理员

管理员需要手动添加到角色，不是通过 webhook 自动分配：

```bash
# 将用户添加到 admin 角色（使用 PUT 方法）
curl -X PUT http://localhost:4467/admin/relation-tuples \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Role","object":"admin","relation":"members","subject_id":"用户ID"}'
```

## 调试命令

```bash
# 健康检查
curl http://localhost:4466/health/alive

# 查询所有 relation tuple
curl http://localhost:4466/relation-tuples | python3 -m json.tool

# 查询特定命名空间
curl "http://localhost:4466/relation-tuples?namespace=Order" | python3 -m json.tool

# 检查权限（具体用户用 subject_id）
curl -X POST "http://localhost:4466/relation-tuples/check" \
  -H "Content-Type: application/json" \
  -d '{"namespace":"Book","object":"*","relation":"managers","subject_id":"charlie"}'

# 展开权限树（查看传递关系）
curl "http://localhost:4466/relation-tuples/expand?namespace=Book&object=*&relation=managers" | python3 -m json.tool

# 查询 users 组的所有成员
curl "http://localhost:4466/relation-tuples?namespace=Group&object=users&relation=members" | python3 -m json.tool
```

> ⚠️ **注意**：Keto v26.2.0 的 Check API 使用 `subject_id` 字段（不是 `subject_set`）。
