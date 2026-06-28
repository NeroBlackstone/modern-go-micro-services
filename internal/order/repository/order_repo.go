package repository

import (
	"modern-micro-services/internal/order/model"

	"gorm.io/gorm"
)

type OrderRepository interface {
	Create(order *model.Order) error
	FindByID(id uint) (*model.Order, error)
	FindByUserID(userID uint, query *model.OrderQuery) ([]model.Order, int64, error)
	UpdateStatus(id uint, status model.OrderStatus) error
}

type orderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{db: db}
}

func (r *orderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

func (r *orderRepository) FindByID(id uint) (*model.Order, error) {
	var order model.Order
	err := r.db.Preload("Items").First(&order, id).Error
	return &order, err
}

func (r *orderRepository) FindByUserID(userID uint, query *model.OrderQuery) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	db := r.db.Model(&model.Order{}).Where("user_id = ?", userID)

	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.GetPage() - 1) * query.GetPageSize()
	err := db.Preload("Items").Offset(offset).Limit(query.GetPageSize()).Order("id DESC").Find(&orders).Error
	return orders, total, err
}

func (r *orderRepository) UpdateStatus(id uint, status model.OrderStatus) error {
	return r.db.Model(&model.Order{}).Where("id = ?", id).Update("status", status).Error
}
