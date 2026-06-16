package model

import "time"

// OrderStatus 订单状态
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"   // 待支付
	OrderStatusPaid      OrderStatus = "paid"      // 已支付
	OrderStatusShipped   OrderStatus = "shipped"   // 已发货
	OrderStatusCompleted OrderStatus = "completed" // 已完成
	OrderStatusCancelled OrderStatus = "cancelled" // 已取消
)

// Order 订单模型
type Order struct {
	ID          uint        `json:"id" gorm:"primaryKey"`
	UserID      uint        `json:"user_id" gorm:"not null;index"`
	TotalAmount float64     `json:"total_amount" gorm:"not null"`
	Status      OrderStatus `json:"status" gorm:"size:20;not null;default:pending"`
	Items       []OrderItem `json:"items,omitempty" gorm:"foreignKey:OrderID"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func (Order) TableName() string {
	return "orders"
}

// OrderItem 订单项模型
type OrderItem struct {
	ID        uint    `json:"id" gorm:"primaryKey"`
	OrderID   uint    `json:"order_id" gorm:"not null;index"`
	BookID    uint    `json:"book_id" gorm:"not null"`
	BookTitle string  `json:"book_title" gorm:"size:200"` // 冗余存储，防止图书信息变更
	Quantity  int     `json:"quantity" gorm:"not null"`
	Price     float64 `json:"price" gorm:"not null"` // 下单时的价格
}

func (OrderItem) TableName() string {
	return "order_items"
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	Items []OrderItemRequest `json:"items" binding:"required,min=1,dive"`
}

// OrderItemRequest 订单项请求
type OrderItemRequest struct {
	BookID   uint `json:"book_id" binding:"required"`
	Quantity int  `json:"quantity" binding:"required,min=1"`
}

// OrderQuery 订单查询参数
type OrderQuery struct {
	Page     int         `form:"page" binding:"omitempty,min=1"`
	PageSize int         `form:"page_size" binding:"omitempty,min=1,max=100"`
	Status   OrderStatus `form:"status"`
}

func (q *OrderQuery) GetPage() int {
	if q.Page <= 0 {
		q.Page = 1
	}
	return q.Page
}

func (q *OrderQuery) GetPageSize() int {
	if q.PageSize <= 0 {
		q.PageSize = 10
	}
	return q.PageSize
}
