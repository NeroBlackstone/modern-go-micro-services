package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"modern-micro-services/internal/book/model"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var ctx = context.Background()

type BookRepository interface {
	Create(book *model.Book) error
	FindByID(id uint) (*model.Book, error)
	Update(book *model.Book) error
	Delete(id uint) error
	List(query *model.BookQuery) ([]model.Book, int64, error)
	FindByIDs(ids []uint) ([]model.Book, error)
	UpdateStock(id uint, delta int) error
}

// bookRepository 基础仓库（无缓存）
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

func (r *bookRepository) UpdateStock(id uint, delta int) error {
	return r.db.Model(&model.Book{}).
		Where("id = ? AND stock >= ?", id, -delta).
		Update("stock", gorm.Expr("stock + ?", delta)).Error
}

// ==================== CachedBookRepository ====================

const (
	bookCachePrefix = "book:"
	bookCacheTTL    = 30 * time.Minute
)

type CachedBookRepository struct {
	*bookRepository
	redis  *redis.Client
	logger *zap.Logger
}

func NewCachedBookRepository(db *gorm.DB, redisClient *redis.Client, logger *zap.Logger) BookRepository {
	return &CachedBookRepository{
		bookRepository: &bookRepository{db: db},
		redis:          redisClient,
		logger:         logger,
	}
}

func bookCacheKey(id uint) string {
	return fmt.Sprintf("%s%d", bookCachePrefix, id)
}

func (r *CachedBookRepository) getFromCache(id uint) (*model.Book, error) {
	data, err := r.redis.Get(ctx, bookCacheKey(id)).Result()
	if err != nil {
		return nil, err
	}

	var book model.Book
	if err := json.Unmarshal([]byte(data), &book); err != nil {
		return nil, err
	}

	r.logger.Debug("cache hit", zap.Uint("book_id", id))
	return &book, nil
}

func (r *CachedBookRepository) setToCache(book *model.Book) error {
	data, err := json.Marshal(book)
	if err != nil {
		return err
	}
	return r.redis.Set(ctx, bookCacheKey(book.ID), data, bookCacheTTL).Err()
}

func (r *CachedBookRepository) deleteFromCache(id uint) error {
	return r.redis.Del(ctx, bookCacheKey(id)).Err()
}

func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
	book, err := r.getFromCache(id)
	if err == nil {
		return book, nil
	}

	if err := r.bookRepository.db.First(&book, id).Error; err != nil {
		return nil, err
	}

	go func() {
		r.setToCache(book)
	}()

	return book, nil
}

func (r *CachedBookRepository) FindByIDs(ids []uint) ([]model.Book, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var books []model.Book
	var missedIDs []uint

	for _, id := range ids {
		book, err := r.getFromCache(id)
		if err == nil {
			books = append(books, *book)
		} else {
			missedIDs = append(missedIDs, id)
		}
	}

	if len(missedIDs) > 0 {
		var dbBooks []model.Book
		if err := r.bookRepository.db.Where("id IN ?", missedIDs).Find(&dbBooks).Error; err != nil {
			return nil, err
		}

		for i := range dbBooks {
			books = append(books, dbBooks[i])
			go func(book model.Book) {
				r.setToCache(&book)
			}(dbBooks[i])
		}
	}

	return books, nil
}

func (r *CachedBookRepository) Create(book *model.Book) error {
	return r.bookRepository.Create(book)
}

func (r *CachedBookRepository) List(query *model.BookQuery) ([]model.Book, int64, error) {
	return r.bookRepository.List(query)
}

func (r *CachedBookRepository) Update(book *model.Book) error {
	if err := r.bookRepository.Update(book); err != nil {
		return err
	}
	r.deleteFromCache(book.ID)
	return nil
}

func (r *CachedBookRepository) Delete(id uint) error {
	if err := r.bookRepository.Delete(id); err != nil {
		return err
	}
	r.deleteFromCache(id)
	return nil
}

func (r *CachedBookRepository) UpdateStock(id uint, delta int) error {
	if err := r.bookRepository.UpdateStock(id, delta); err != nil {
		return err
	}
	r.deleteFromCache(id)
	return nil
}
