package repository

import (
	"modern-micro-services/internal/model"

	"gorm.io/gorm"
)

type BookRepository interface {
	Create(book *model.Book) error
	FindByID(id uint) (*model.Book, error)
	Update(book *model.Book) error
	Delete(id uint) error
	List(query *model.BookQuery) ([]model.Book, int64, error)
	FindByIDs(ids []uint) ([]model.Book, error)
	UpdateStock(id uint, delta int) error // 库存增减
}

type bookRepository struct {
	db *gorm.DB
}

func NewBookRepository(db *gorm.DB) BookRepository {
	return &bookRepository{db: db}
}

func (r *bookRepository) Create(book *model.Book) error {
	return r.db.Create(book).Error
}

func (r *bookRepository) FindByID(id uint) (*model.Book, error) {
	var book model.Book
	err := r.db.First(&book, id).Error
	return &book, err
}

func (r *bookRepository) Update(book *model.Book) error {
	return r.db.Save(book).Error
}

func (r *bookRepository) Delete(id uint) error {
	return r.db.Delete(&model.Book{}, id).Error
}

func (r *bookRepository) List(query *model.BookQuery) ([]model.Book, int64, error) {
	var books []model.Book
	var total int64

	db := r.db.Model(&model.Book{})

	if query.Keyword != "" {
		keyword := "%" + query.Keyword + "%"
		db = db.Where("title LIKE ? OR author LIKE ?", keyword, keyword)
	}
	if query.Author != "" {
		db = db.Where("author LIKE ?", "%"+query.Author+"%")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (query.GetPage() - 1) * query.GetPageSize()
	err := db.Offset(offset).Limit(query.GetPageSize()).Order("id DESC").Find(&books).Error
	return books, total, err
}

func (r *bookRepository) FindByIDs(ids []uint) ([]model.Book, error) {
	var books []model.Book
	err := r.db.Where("id IN ?", ids).Find(&books).Error
	return books, err
}

// UpdateStock 使用数据库行锁更新库存，防止超卖
func (r *bookRepository) UpdateStock(id uint, delta int) error {
	return r.db.Model(&model.Book{}).
		Where("id = ? AND stock >= ?", id, -delta).
		Update("stock", gorm.Expr("stock + ?", delta)).Error
}
