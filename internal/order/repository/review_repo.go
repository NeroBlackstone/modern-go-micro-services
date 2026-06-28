package repository

import (
	"modern-micro-services/internal/order/model"

	"gorm.io/gorm"
)

type ReviewRepository interface {
	Create(review *model.Review) error
	FindByBookID(bookID uint, query *model.ReviewQuery) ([]model.Review, int64, error)
	GetStats(bookID uint) (*model.ReviewStats, error)
	HasPurchased(userID, bookID uint) (bool, error)
	HasReviewed(userID, bookID uint) (bool, error)
}

type reviewRepository struct {
	db *gorm.DB
}

func NewReviewRepository(db *gorm.DB) ReviewRepository {
	return &reviewRepository{db: db}
}

func (r *reviewRepository) Create(review *model.Review) error {
	return r.db.Create(review).Error
}

func (r *reviewRepository) FindByBookID(bookID uint, query *model.ReviewQuery) ([]model.Review, int64, error) {
	var reviews []model.Review
	var total int64

	db := r.db.Model(&model.Review{}).Where("book_id = ?", bookID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.GetPage() - 1) * query.GetPageSize()
	err := db.Offset(offset).Limit(query.GetPageSize()).Order("id DESC").Find(&reviews).Error
	return reviews, total, err
}

func (r *reviewRepository) GetStats(bookID uint) (*model.ReviewStats, error) {
	stats := &model.ReviewStats{}

	err := r.db.Model(&model.Review{}).
		Select("COALESCE(AVG(rating), 0) as average_rating, COUNT(*) as total_reviews").
		Where("book_id = ?", bookID).
		Scan(stats).Error
	if err != nil {
		return nil, err
	}

	for i := 1; i <= 5; i++ {
		var count int64
		err := r.db.Model(&model.Review{}).
			Where("book_id = ? AND rating = ?", bookID, i).
			Count(&count).Error
		if err != nil {
			return nil, err
		}
		switch i {
		case 1:
			stats.Rating1Count = count
		case 2:
			stats.Rating2Count = count
		case 3:
			stats.Rating3Count = count
		case 4:
			stats.Rating4Count = count
		case 5:
			stats.Rating5Count = count
		}
	}

	return stats, nil
}

func (r *reviewRepository) HasPurchased(userID, bookID uint) (bool, error) {
	var count int64
	err := r.db.Table("order_items").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.user_id = ? AND order_items.book_id = ? AND orders.status IN ?",
			userID, bookID, []model.OrderStatus{model.OrderStatusPaid, model.OrderStatusShipped, model.OrderStatusCompleted}).
		Count(&count).Error
	return count > 0, err
}

func (r *reviewRepository) HasReviewed(userID, bookID uint) (bool, error) {
	var count int64
	err := r.db.Model(&model.Review{}).
		Where("user_id = ? AND book_id = ?", userID, bookID).
		Count(&count).Error
	return count > 0, err
}
