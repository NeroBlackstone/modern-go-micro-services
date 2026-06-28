package service

import (
	"errors"
	"fmt"

	"modern-micro-services/internal/order/client"
	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/rabbitmq"
	"modern-micro-services/internal/order/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type OrderService interface {
	Create(userID uint, req *model.CreateOrderRequest) (*model.Order, error)
	GetByID(id, userID uint) (*model.Order, error)
	ListByUserID(userID uint, query *model.OrderQuery) ([]model.Order, int64, error)
	UpdateStatus(id, userID uint, status model.OrderStatus) error
}

type orderService struct {
	orderRepo repository.OrderRepository
	bookClient *client.BookClient
	producer   *rabbitmq.Producer
	logger     *zap.Logger
	db         *gorm.DB
}

func NewOrderService(
	orderRepo repository.OrderRepository,
	bookClient *client.BookClient,
	producer *rabbitmq.Producer,
	logger *zap.Logger,
	db *gorm.DB,
) OrderService {
	return &orderService{
		orderRepo:  orderRepo,
		bookClient: bookClient,
		producer:   producer,
		logger:     logger,
		db:         db,
	}
}

// Create 创建订单 —— Saga 模式
// 1. 本地创建订单 (status=pending)
// 2. gRPC 调用 book-service 扣减库存
// 3. 成功 → 更新订单为 paid，发布事件
// 4. 失败 → 调 book-service 恢复库存 + 本地取消订单
func (s *orderService) Create(userID uint, req *model.CreateOrderRequest) (*model.Order, error) {
	// 收集所有图书 ID
	bookIDs := make([]uint, 0, len(req.Items))
	for _, item := range req.Items {
		bookIDs = append(bookIDs, item.BookID)
	}

	// gRPC 批量获取图书信息
	books, err := s.bookClient.GetBooks(bookIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get books: %w", err)
	}

	// 建立 bookID -> book 的映射
	bookMap := make(map[uint]*bookInfo)
	for _, book := range books {
		bookMap[uint(book.Id)] = &bookInfo{
			id:       uint(book.Id),
			title:    book.Title,
			price:    book.Price,
			stock:    int(book.Stock),
		}
	}

	// 验证库存并构建订单项
	var totalAmount float64
	orderItems := make([]model.OrderItem, 0, len(req.Items))

	for _, item := range req.Items {
		book, exists := bookMap[item.BookID]
		if !exists {
			return nil, fmt.Errorf("book with id %d not found", item.BookID)
		}
		if book.stock < item.Quantity {
			return nil, fmt.Errorf("insufficient stock for book '%s' (available: %d, requested: %d)",
				book.title, book.stock, item.Quantity)
		}

		orderItems = append(orderItems, model.OrderItem{
			BookID:    book.id,
			BookTitle: book.title,
			Quantity:  item.Quantity,
			Price:     book.price,
		})
		totalAmount += book.price * float64(item.Quantity)
	}

	// ========== Saga Step 1: 本地创建订单 (status=pending) ==========
	order := &model.Order{
		UserID:      userID,
		TotalAmount: totalAmount,
		Status:      model.OrderStatusPending,
		Items:       orderItems,
	}

	if err := s.orderRepo.Create(order); err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	s.logger.Info("order created (pending)", zap.Uint("order_id", order.ID))

	// ========== Saga Step 2: gRPC 扣减库存 ==========
	for _, item := range req.Items {
		if err := s.bookClient.DeductStock(item.BookID, item.Quantity); err != nil {
			s.logger.Error("failed to deduct stock, compensating...",
				zap.Uint("book_id", item.BookID),
				zap.Error(err),
			)

			// ========== Saga Step 3 (失败): 补偿 —— 恢复已扣减的库存 + 取消订单 ==========
			s.compensate(order, req.Items)
			return nil, fmt.Errorf("failed to deduct stock: %w", err)
		}
	}

	// ========== Saga Step 3 (成功): 更新订单状态为 paid ==========
	if err := s.orderRepo.UpdateStatus(order.ID, model.OrderStatusPaid); err != nil {
		s.logger.Error("failed to update order status to paid", zap.Uint("order_id", order.ID), zap.Error(err))
		// 订单已扣库存但状态未更新，需要人工介入或重试
		return nil, fmt.Errorf("failed to update order status: %w", err)
	}

	order.Status = model.OrderStatusPaid

	// 异步发布订单创建事件（fire-and-forget）
	event := model.NewOrderCreatedEvent(order)
	if err := s.producer.PublishOrderCreated(event); err != nil {
		s.logger.Error("failed to publish order created event",
			zap.Uint("order_id", order.ID),
			zap.Error(err),
		)
	}

	return order, nil
}

// compensate Saga 补偿逻辑：恢复库存 + 取消订单
func (s *orderService) compensate(order *model.Order, items []model.OrderItemRequest) {
	// 恢复已扣减的库存
	for _, item := range items {
		if err := s.bookClient.RestoreStock(item.BookID, item.Quantity); err != nil {
			s.logger.Error("CRITICAL: failed to restore stock during compensation",
				zap.Uint("order_id", order.ID),
				zap.Uint("book_id", item.BookID),
				zap.Error(err),
			)
			// 补偿失败需要记录，后续人工介入或重试队列
		}
	}

	// 取消订单
	if err := s.orderRepo.UpdateStatus(order.ID, model.OrderStatusCancelled); err != nil {
		s.logger.Error("CRITICAL: failed to cancel order during compensation",
			zap.Uint("order_id", order.ID),
			zap.Error(err),
		)
	}

	s.logger.Info("saga compensation completed", zap.Uint("order_id", order.ID))
}

func (s *orderService) GetByID(id, userID uint) (*model.Order, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, errors.New("order not found")
	}
	return order, nil
}

func (s *orderService) ListByUserID(userID uint, query *model.OrderQuery) ([]model.Order, int64, error) {
	return s.orderRepo.FindByUserID(userID, query)
}

func (s *orderService) UpdateStatus(id, userID uint, status model.OrderStatus) error {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return err
	}
	if order.UserID != userID {
		return errors.New("order not found")
	}
	if !isValidStatusTransition(order.Status, status) {
		return fmt.Errorf("invalid status transition from %s to %s", order.Status, status)
	}
	return s.orderRepo.UpdateStatus(id, status)
}

func isValidStatusTransition(from, to model.OrderStatus) bool {
	transitions := map[model.OrderStatus][]model.OrderStatus{
		model.OrderStatusPending:   {model.OrderStatusPaid, model.OrderStatusCancelled},
		model.OrderStatusPaid:      {model.OrderStatusShipped, model.OrderStatusCancelled},
		model.OrderStatusShipped:   {model.OrderStatusCompleted},
		model.OrderStatusCompleted: {},
		model.OrderStatusCancelled: {},
	}
	allowed, exists := transitions[from]
	if !exists {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

type bookInfo struct {
	id    uint
	title string
	price float64
	stock int
}
