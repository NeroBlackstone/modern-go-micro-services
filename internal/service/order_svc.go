package service

import (
	"errors"
	"fmt"

	"modern-micro-services/internal/model"
	"modern-micro-services/internal/repository"

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
	bookRepo  repository.BookRepository
	db        *gorm.DB
}

func NewOrderService(orderRepo repository.OrderRepository, bookRepo repository.BookRepository, db *gorm.DB) OrderService {
	return &orderService{
		orderRepo: orderRepo,
		bookRepo:  bookRepo,
		db:        db,
	}
}

func (s *orderService) Create(userID uint, req *model.CreateOrderRequest) (*model.Order, error) {
	// 收集所有图书ID
	bookIDs := make([]uint, 0, len(req.Items))
	for _, item := range req.Items {
		bookIDs = append(bookIDs, item.BookID)
	}

	// 批量查询图书信息
	books, err := s.bookRepo.FindByIDs(bookIDs)
	if err != nil {
		return nil, err
	}

	// 建立 bookID -> book 的映射
	bookMap := make(map[uint]*model.Book)
	for i := range books {
		bookMap[books[i].ID] = &books[i]
	}

	// 构建订单项，计算总金额
	var totalAmount float64
	orderItems := make([]model.OrderItem, 0, len(req.Items))

	for _, item := range req.Items {
		book, exists := bookMap[item.BookID]
		if !exists {
			return nil, fmt.Errorf("book with id %d not found", item.BookID)
		}

		if book.Stock < item.Quantity {
			return nil, fmt.Errorf("insufficient stock for book '%s' (available: %d, requested: %d)",
				book.Title, book.Stock, item.Quantity)
		}

		orderItems = append(orderItems, model.OrderItem{
			BookID:    book.ID,
			BookTitle: book.Title,
			Quantity:  item.Quantity,
			Price:     book.Price,
		})

		totalAmount += book.Price * float64(item.Quantity)
	}

	// 使用事务处理：扣库存 + 创建订单
	order := &model.Order{
		UserID:      userID,
		TotalAmount: totalAmount,
		Status:      model.OrderStatusPending,
		Items:       orderItems,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 扣减库存
		for _, item := range req.Items {
			if err := s.bookRepo.UpdateStock(item.BookID, -item.Quantity); err != nil {
				return fmt.Errorf("failed to update stock for book %d: %w", item.BookID, err)
			}
		}

		// 创建订单
		if err := s.orderRepo.Create(order); err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return order, nil
}

func (s *orderService) GetByID(id, userID uint) (*model.Order, error) {
	order, err := s.orderRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	// 验证订单归属
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

	// 状态流转校验
	if !isValidStatusTransition(order.Status, status) {
		return fmt.Errorf("invalid status transition from %s to %s", order.Status, status)
	}

	return s.orderRepo.UpdateStatus(id, status)
}

// isValidStatusTransition 校验订单状态流转是否合法
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
