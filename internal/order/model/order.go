package model

import "time"

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

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

type OrderItem struct {
	ID        uint    `json:"id" gorm:"primaryKey"`
	OrderID   uint    `json:"order_id" gorm:"not null;index"`
	BookID    uint    `json:"book_id" gorm:"not null"`
	BookTitle string  `json:"book_title" gorm:"size:200"`
	Quantity  int     `json:"quantity" gorm:"not null"`
	Price     float64 `json:"price" gorm:"not null"`
}

func (OrderItem) TableName() string {
	return "order_items"
}

type CreateOrderRequest struct {
	Items []OrderItemRequest `json:"items" binding:"required,min=1,dive"`
}

type OrderItemRequest struct {
	BookID   uint `json:"book_id" binding:"required"`
	Quantity int  `json:"quantity" binding:"required,min=1"`
}

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

// Review 相关模型（保留在 order-service 中，因为 HasPurchased 需要查 order_items）

type Review struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;index"`
	BookID    uint      `json:"book_id" gorm:"not null;index"`
	Rating    int       `json:"rating" gorm:"not null"`
	Comment   string    `json:"comment" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
}

func (Review) TableName() string {
	return "reviews"
}

type CreateReviewRequest struct {
	BookID  uint   `json:"book_id" binding:"required"`
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Comment string `json:"comment" binding:"max=2000"`
}

type ReviewStats struct {
	AverageRating float64 `json:"average_rating"`
	TotalReviews  int64   `json:"total_reviews"`
	Rating1Count  int64   `json:"rating_1_count"`
	Rating2Count  int64   `json:"rating_2_count"`
	Rating3Count  int64   `json:"rating_3_count"`
	Rating4Count  int64   `json:"rating_4_count"`
	Rating5Count  int64   `json:"rating_5_count"`
}

type ReviewQuery struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

func (q *ReviewQuery) GetPage() int {
	if q.Page <= 0 {
		q.Page = 1
	}
	return q.Page
}

func (q *ReviewQuery) GetPageSize() int {
	if q.PageSize <= 0 {
		q.PageSize = 10
	}
	return q.PageSize
}

// OrderCreatedEvent 事件模型
type OrderCreatedEvent struct {
	OrderID     uint           `json:"order_id"`
	UserID      uint           `json:"user_id"`
	TotalAmount float64        `json:"total_amount"`
	Status      OrderStatus    `json:"status"`
	Items       []OrderItemRef `json:"items"`
	CreatedAt   time.Time      `json:"created_at"`
}

type OrderItemRef struct {
	BookID    uint    `json:"book_id"`
	BookTitle string  `json:"book_title"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

func NewOrderCreatedEvent(order *Order) *OrderCreatedEvent {
	items := make([]OrderItemRef, len(order.Items))
	for i, item := range order.Items {
		items[i] = OrderItemRef{
			BookID:    item.BookID,
			BookTitle: item.BookTitle,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
	}
	return &OrderCreatedEvent{
		OrderID:     order.ID,
		UserID:      order.UserID,
		TotalAmount: order.TotalAmount,
		Status:      order.Status,
		Items:       items,
		CreatedAt:   order.CreatedAt,
	}
}
