# Day 12: Ory Keto 权限控制 — 核心概念

## 为什么需要授权控制

Day 10-11 我们完成了完整的认证链路：

```
Kratos（身份管理） → Hydra（OAuth2/OIDC） → Oathkeeper（API 网关拦截）
```

但所有受保护路由的 authorizer 都是 `allow`，意味着**通过认证即可访问任何资源**。在实际业务中这是不可接受的：

- 普通用户不应该能管理其他人的订单
- 非管理员不应该能编辑所有书籍
- 未登录用户不应该能发布书评

**授权（Authorization）** — "你能做什么" — 就是 Day 12 要解决的问题。

## Ory Keto 是什么

Ory Keto 是 Ory 生态中的**授权服务**，实现了 Google Zanzibar 的关系模型：

```
身份管理（Kratos） → 认证（Hydra） → 网关拦截（Oathkeeper） → 权限控制（Keto）
```

Keto 的核心数据结构是**关系元组（Relation Tuple）**，格式为：

```
namespace:object#relation@subject
```

## Google Zanzibar 模型

Zanzibar 是 Google 内部使用的授权系统（用于 Google Drive、YouTube 等），其核心思想是通过关系元组描述"谁对什么资源有什么权限"。

### 关系元组的结构

| 组成部分 | 含义 | 示例 |
|----------|------|------|
| `namespace` | 资源类型 | `Book`, `Order`, `Group` |
| `object` | 具体资源实例（`*` 表示通配） | `book:123`, `Order:*` |
| `relation` | 权限关系 | `viewers`, `managers`, `members` |
| `subject` | 谁拥有这个关系 | `User:charlie`, `Role:admin#members` |

### 具体用户 vs 间接引用

```json
// 具体用户 — 直接用 subject_id
{"namespace":"Role","object":"admin","relation":"members","subject_id":"charlie"}

// 间接引用 — 用 subject_set 引用其他命名空间的关系
{"namespace":"Book","object":"*","relation":"managers",
 "subject_set":{"namespace":"Role","object":"admin","relation":"members"}}
```

### 关系继承

这是 Zanzibar 最强大的特性。系统会自动推导传递关系：

```
Role:admin#members@User:charlie          # charlie 是 admin 角色成员
Book:*#managers@Role:admin#members       # admin 可管理所有书籍

推理结果: User:charlie 可以管理 Book:*
```

你只需要定义少量基础关系，系统会自动展开完整的权限图。

## 书店场景中的权限矩阵

| 资源 | 未登录 | 所有用户 | admin | author |
|------|--------|----------|-------|--------|
| 书籍浏览 | ❌ | ✅ | ✅ | ✅ |
| 书籍编辑 | ❌ | ❌ | ✅ 所有 | ✅ 自己的 |
| 订单查看 | ❌ | ✅ | ✅ | ✅ |
| 订单管理 | ❌ | ❌ | ✅ | ❌ |
| 书评发布 | ❌ | ✅ | ✅ | ✅ |
| 书评管理 | ❌ | ❌ | ✅ | ✅ 自己的 |

## 授权流程

```
Client 发送请求（携带 cookie）
  → Traefik 转发到 Oathkeeper Decision API
  → Oathkeeper cookie_session 验证身份 → 得到 subject (用户 ID)
  → Oathkeeper keto_engine_acp_ory 检查权限
  → 调用 Keto Check API，自动解析 subject sets 和传递关系
  → 通过 → 转发到后端服务
  → 拒绝 → 返回 403 Forbidden
```

**不同路由使用不同的 Keto action：**

| 路由前缀 | 后端服务 | Keto action | 说明 |
|----------|---------|-------------|------|
| `/api/v1/orders/*` | order-service | `Order:read` | 用户查看/创建订单 |
| `/api/v1/admin/orders/*` | admin-service | `Order:manage` | 管理员更新订单状态 |
| `/api/v1/reviews/*` | order-service | `Review:write` | 用户发布书评 |

## 与其他方案的对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| **Ory Keto** | Zanzibar 模型、关系继承、Ory 生态集成 | 学习曲线陡、配置复杂 |
| **Casbin** | 轻量、支持多种模型、Go 原生 | 缺少关系继承、需自行集成 |
| **OPA** | 通用策略引擎、Rego 语言强大 | 过于通用、配置复杂 |
| **RBAC in DB** | 简单直接 | 缺少关系继承、难以跨服务 |

## 下一步

- [02-keto-configuration.md](./02-keto-configuration.md) — 配置详解与集成
- [03-testing-guide.md](./03-testing-guide.md) — 测试与调试
- [04-architecture-overview.md](./04-architecture-overview.md) — 全局架构总览
