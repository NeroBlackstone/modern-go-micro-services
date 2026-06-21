# Gin 单体应用项目结构

## 目录结构

```
modern-micro-services/
│
├── cmd/server/main.go           # 程序入口，启动流程：加载配置 → 连接DB → 注册路由 → 启动HTTP服务
│
├── internal/                     # 核心业务代码（不允许被外部项目引用）
│   ├── config/                   # 配置加载（读取 config.yaml）
│   ├── model/                    # 数据模型定义（纯数据结构，不含业务逻辑）
│   ├── repository/               # 数据库操作（封装所有 SQL/GORM 调用）
│   ├── service/                  # 业务逻辑（校验规则、流程编排、事务控制）
│   ├── handler/                  # HTTP 路由 + 请求解析 + 响应返回
│   └── middleware/                # 中间件（JWT认证、日志、跨域）
│
├── pkg/response/                 # 公共工具包（统一响应格式）
├── docs/                         # Swagger 文档（自动生成）
├── configs/config.yaml           # 配置文件
├── docker-compose.yml            # Docker 编排（App + PostgreSQL）
└── Dockerfile                    # 应用镜像构建
```

## 分层架构

```
HTTP Request
  ↓
Middleware（JWT认证、日志、跨域）
  ↓
Handler（解析请求参数，调用 Service，返回 JSON 响应）
  ↓
Service（业务逻辑：校验规则、流程编排、事务控制）
  ↓
Repository（数据库读写，封装 GORM 操作）
  ↓
PostgreSQL
```

**规则：** 每一层只调用直接下一层，不能跨层。

## 请求流转示例

以「创建订单」为例：

```
1. 客户端 POST /api/v1/orders
   请求体: { "items": [{ "book_id": 1, "quantity": 2 }] }

2. Middleware.JWTAuth()
   - 从 Header 取 Token，验证有效性
   - 解析出 user_id，存入上下文

3. Handler.CreateOrder()
   - 从上下文取 user_id
   - 解析 JSON 请求体到 CreateOrderRequest 结构
   - 调用 orderSvc.Create(userID, &req)
   - 根据返回结果调用 response.Success() 或 response.BadRequest()

4. Service.Create()
   - 根据 book_id 批量查询图书信息
   - 校验每本书的库存是否充足
   - 开启事务：
     a. 扣减库存
     b. 创建订单记录
   - 事务成功则返回订单，失败则回滚

5. Repository（被 Service 调用）
   - FindByIDs()：批量查图书
   - UpdateStock()：扣减库存（带 WHERE stock >= 条件防超卖）
   - Create()：写入订单和订单项

6. PostgreSQL 执行 SQL
```

## 各模块职责

### model — 数据模型
- 定义数据库表结构（对应数据库的表）
- 定义请求体结构（接收客户端参数）
- 定义响应体结构（返回给客户端的数据）
- **不含任何业务逻辑**

### repository — 数据库操作
- 每个模型对应一个 repository（UserRepository、BookRepository...）
- 只做数据库 CRUD，不做业务判断
- 使用接口定义，方便后续替换实现

### service — 业务逻辑
- 每个模块一个 service（UserService、BookService...）
- 编排多个 repository 调用完成一个业务操作
- 负责事务控制（如订单创建涉及多表写入）
- 负责业务校验（如库存是否充足、是否有权限）

### handler — HTTP 处理
- 每个模块一个 handler（UserHandler、BookHandler...）
- 职责单一：解析参数 → 调 service → 返回响应
- 不包含业务逻辑，只做 HTTP 层面的事情
- `router.go` 集中注册所有路由

### middleware — 中间件
- 在请求到达 handler 之前执行
- `jwt.go`：验证 Token，提取用户信息
- `logger.go`：记录每个请求的方法、路径、耗时
- `cors.go`：处理跨域请求

### pkg — 公共工具
- 可被多个模块复用的代码
- 当前只有一个 `response.go`，定义统一的 JSON 响应格式

## 路由注册

所有路由在 `router.go` 中集中注册：

```
/api/v1/
  /auth
    POST /register        → 公开
    POST /login           → 公开
  /books
    GET  /                → 公开（列表+搜索+分页）
    GET  /:id             → 公开（详情）
    POST /                → 需认证（创建图书）
    PUT  /:id             → 需认证（更新图书）
    DELETE /:id           → 需认证（删除图书）
  /user
    GET  /profile         → 需认证
    PUT  /profile         → 需认证
  /orders
    POST /                → 需认证（创建订单）
    GET  /                → 需认证（订单列表）
    GET  /:id             → 需认证（订单详情）
    PUT  /:id/status      → 需认证（更新状态）
  /reviews
    GET  /book/:book_id        → 公开（评价列表）
    GET  /book/:book_id/stats  → 公开（评分统计）
    POST /                     → 需认证（发表评价）
```
