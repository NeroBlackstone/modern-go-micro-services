# GORM：Go 语言 ORM 框架

## 是什么

GORM 是 Go 语言最常用的 ORM（Object-Relational Mapping）框架，让你用 Go struct 操作数据库，不需要手写 SQL。核心思路：

```
Go struct  ←→  数据库表
struct 字段  ←→  表列
struct 实例  ←→  表行
```

本项目使用 `gorm.io/gorm` + `gorm.io/driver/postgres`（PostgreSQL 驱动）。

---

## 初始化

### 连接数据库（`cmd/server/main.go`）

```go
import (
    "gorm.io/gorm"
    "gorm.io/driver/postgres"
)

db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{})
if err != nil {
    logger.Fatal("failed to connect database", zap.Error(err))
}
```

`gorm.Open()` 返回一个 `*gorm.DB` 实例，后续所有操作都通过它执行。不同数据库换不同 driver 即可（`mysql`、`sqlite` 等），GORM 接口统一。

### 自动迁移

```go
db.AutoMigrate(
    &model.User{},
    &model.Book{},
    &model.Order{},
    &model.OrderItem{},
    &model.Review{},
)
```

根据 struct 定义自动创建/修改表结构（加列不会删列）。开发阶段方便，**生产环境慎用**。

---

## Model 定义

struct 通过 `gorm` tag 描述表结构（`internal/model/user.go`）：

```go
type User struct {
    ID        uint      `gorm:"primaryKey"`                  // 主键
    Username  string    `gorm:"size:50;uniqueIndex;not null"` // 长度限制、唯一索引、非空
    Email     string    `gorm:"size:100;uniqueIndex;not null"`
    Password  string    `gorm:"not null"`
    CreatedAt time.Time // GORM 自动维护创建时间
    UpdatedAt time.Time // GORM 自动维护更新时间
}
```

### 常用 tag

| Tag | 作用 | 示例 |
|-----|------|------|
| `primaryKey` | 设为主键 | `gorm:"primaryKey"` |
| `size` | 字符串最大长度 | `gorm:"size:50"` |
| `uniqueIndex` | 创建唯一索引 | `gorm:"uniqueIndex"` |
| `not null` | 非空约束 | `gorm:"not null"` |
| `default` | 默认值 | `gorm:"default:pending"` |
| `index` | 普通索引 | `gorm:"index"` |
| `foreignKey` | 指定外键 | `gorm:"foreignKey:OrderID"` |
| `json:"-"` | JSON 序列化时忽略 | 防止密码泄露 |

### 自定义表名

默认表名是 struct 名复数形式（User → users），可以覆盖：

```go
func (User) TableName() string {
    return "users"
}
```

---

## CRUD 操作

项目用 **Repository 模式**封装所有 GORM 调用（`internal/repository/`），每个 Repository 持有 `*gorm.DB`。

### Create — 插入

```go
func (r *userRepository) Create(user *model.User) error {
    return r.db.Create(user).Error
}
```

`Create()` 将 struct 插入数据库，执行后 `user.ID` 会被自动填充为自增主键值。

### Read — 查询

```go
// 按主键查一条
func (r *userRepository) FindByID(id uint) (*model.User, error) {
    var user model.User
    err := r.db.First(&user, id).Error  // SELECT * FROM users WHERE id = ? LIMIT 1
    return &user, err
}

// 条件查询
func (r *userRepository) FindByEmail(email string) (*model.User, error) {
    var user model.User
    err := r.db.Where("email = ?", email).First(&user).Error
    return &user, err
}

// 批量查询
func (r *bookRepository) FindByIDs(ids []uint) ([]model.Book, error) {
    var books []model.Book
    err := r.db.Where("id IN ?", ids).Find(&books).Error
    return books, err
}
```

| 方法 | 行为 |
|------|------|
| `First()` | 查一条，找不到返回 `ErrRecordNotFound` |
| `Find()` | 查多条，找不到返回空切片（不报错） |
| `Last()` | 按主键倒序取第一条 |

### Update — 更新

```go
// 全量保存（所有字段）
func (r *userRepository) Update(user *model.User) error {
    return r.db.Save(user).Error
}

// 更新单个字段
func (r *orderRepository) UpdateStatus(id uint, status model.OrderStatus) error {
    return r.db.Model(&model.Order{}).Where("id = ?", id).Update("status", status).Error
}
```

`Save()` 会更新**所有字段**（包括零值），`Update()` 只更新指定字段。

### Delete — 删除

```go
func (r *bookRepository) Delete(id uint) error {
    return r.db.Delete(&model.Book{}, id).Error
}
```

默认是软删除（如果 struct 里有 `DeletedAt` 字段）。本项目没有软删除，所以是物理删除。

---

## 关联查询 — Preload

Order 和 OrderItem 是一对多关系（`internal/model/order.go`）：

```go
type Order struct {
    ID        uint        `gorm:"primaryKey"`
    UserID    uint        `gorm:"not null;index"`
    Items     []OrderItem `gorm:"foreignKey:OrderID"` // 一对多关联
}
```

查询时用 `Preload` 预加载关联数据（`internal/repository/order_repo.go`）：

```go
func (r *orderRepository) FindByID(id uint) (*model.Order, error) {
    var order model.Order
    err := r.db.Preload("Items").First(&order, id).Error
    // 先查 orders 表，再查 order_items 表，把 Items 填充进去
    return &order, err
}
```

**不用 Preload 的话 `order.Items` 是空切片**，GORM 不会自动加载关联数据。

也可以嵌套预加载：`Preload("Items.Book")`。

---

## 链式查询

GORM 的查询 API 是链式调用，像拼积木一样组合条件（`internal/repository/book_repo.go`）：

```go
func (r *bookRepository) List(query *model.BookQuery) ([]model.Book, int64, error) {
    var books []model.Book
    var total int64

    db := r.db.Model(&model.Book{})  // 从 Model 开始

    // 动态拼条件
    if query.Keyword != "" {
        keyword := "%" + query.Keyword + "%"
        db = db.Where("title LIKE ? OR author LIKE ?", keyword, keyword)
    }
    if query.Author != "" {
        db = db.Where("author LIKE ?", "%"+query.Author+"%")
    }

    // 查总数
    if err := db.Count(&total).Error; err != nil {
        return nil, 0, err
    }

    // 分页 + 排序
    offset := (query.GetPage() - 1) * query.GetPageSize()
    err := db.Offset(offset).Limit(query.GetPageSize()).Order("id DESC").Find(&books)
    return books, total, err
}
```

### 常用链式方法

| 方法 | SQL 等价 | 说明 |
|------|----------|------|
| `.Where("x = ?", v)` | `WHERE x = ?` | 条件过滤，支持多个 |
| `.And("y = ?", v)` | `AND y = ?` | 链式 AND |
| `.Or("z = ?", v)` | `OR z = ?` | 链式 OR |
| `.Select("col1, col2")` | `SELECT col1, col2` | 指定查询列 |
| `.Order("id DESC")` | `ORDER BY id DESC` | 排序 |
| `.Limit(n)` | `LIMIT n` | 限制返回条数 |
| `.Offset(n)` | `OFFSET n` | 跳过前 n 条 |
| `.Count(&total)` | `SELECT COUNT(*)` | 统计总数 |
| `.Pluck("col", &slice)` | `SELECT col FROM ...` | 提取单列 |

关键点：**每个方法都返回 `*gorm.DB`，可以一直点下去**。

---

## 原子操作 — 防超卖

更新库存时用 `gorm.Expr` 生成 SQL 表达式（`internal/repository/book_repo.go`）：

```go
func (r *bookRepository) UpdateStock(id uint, delta int) error {
    return r.db.Model(&model.Book{}).
        Where("id = ? AND stock >= ?", id, -delta).  // 库存必须够
        Update("stock", gorm.Expr("stock + ?", delta)).Error
}
```

生成的 SQL：
```sql
UPDATE books SET stock = stock + ? WHERE id = ? AND stock >= ?
```

`gorm.Expr("stock + ?", delta)` 让数据库做原子加减，`WHERE stock >= ?` 防止库存变负数。两步配合防止并发超卖。

---

## 错误处理

GORM 的错误通过 `.Error` 返回：

```go
err := r.db.First(&user, id).Error
if err != nil {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        // 记录不存在
    }
    // 其他错误（连接、权限等）
}
```

实际项目中 Repository 直接返回 `error`，由 Service 层决定如何处理（返回给 Handler → 返回给前端）。

---

## 调试模式

开启后会打印执行的 SQL（开发阶段有用）：

```go
db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
db.Debug()  // 开启调试日志
```

或通过 `gorm.Config` 全局配置 Logger。

---

## 数据流总结

```
Handler → Service → Repository → *gorm.DB → 数据库
                     ↑ 封装 GORM 调用
                     ↑ 每个模型对应一个 Repository
```

| 层 | 文件 | GORM 相关 |
|----|------|-----------|
| Model | `internal/model/*.go` | struct + gorm tag 定义表结构 |
| Repository | `internal/repository/*.go` | 封装所有 GORM CRUD 操作 |
| Service | `internal/service/*.go` | 调用 Repository，不含 GORM |
| Handler | `internal/handler/*.go` | 调用 Service，不含 GORM |
| 入口 | `cmd/server/main.go` | `gorm.Open()` + `AutoMigrate()` |

---

**文档版本**: v1.0
**创建时间**: 2026-06-16
**关联文档**: [phase-1.3 Gin Middleware](./phase-1.3-gin-middleware.md)、[phase-1.1 项目结构](./phase-1.1-project-structure.md)
