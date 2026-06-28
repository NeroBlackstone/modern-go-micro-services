package model

import "time"

// OrderCreatedEvent 订单创建事件
// 当订单创建成功后发布，下游消费者可以基于此事件执行异步操作
// 如：发送确认通知、记录审计日志、更新统计等
type OrderCreatedEvent struct {
	OrderID     uint           `json:"order_id"`
	UserID      uint           `json:"user_id"`
	TotalAmount float64        `json:"total_amount"`
	Status      OrderStatus    `json:"status"`
	Items       []OrderItemRef `json:"items"`
	CreatedAt   time.Time      `json:"created_at"`
}

// OrderItemRef 订单项引用（事件中只保留必要信息）
type OrderItemRef struct {
	BookID    uint    `json:"book_id"`
	BookTitle string  `json:"book_title"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

// NewOrderCreatedEvent 从 Order 构建事件
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
