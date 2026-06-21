package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"modern-micro-services/internal/model"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var ctx = context.Background()

// BookRepository 图书仓库接口
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

// UpdateStock 使用数据库行锁更新库存，防止超卖
func (r *bookRepository) UpdateStock(id uint, delta int) error {
	return r.db.Model(&model.Book{}).
		Where("id = ? AND stock >= ?", id, -delta).
		Update("stock", gorm.Expr("stock + ?", delta)).Error
}

// ==================== CachedBookRepository 带缓存的仓库 ====================

const (
	bookCachePrefix = "book:"
	bookCacheTTL    = 30 * time.Minute // 缓存过期时间30分钟
)

// CachedBookRepository 带Redis缓存的图书仓库
type CachedBookRepository struct {
	*bookRepository // 嵌入基础仓库
	redis           *redis.Client
	logger          *zap.Logger
}

// NewCachedBookRepository 创建带缓存的图书仓库
func NewCachedBookRepository(db *gorm.DB, redisClient *redis.Client, logger *zap.Logger) BookRepository {
	return &CachedBookRepository{
		bookRepository: &bookRepository{db: db},
		redis:          redisClient,
		logger:         logger,
	}
}

// bookCacheKey 生成缓存key
func bookCacheKey(id uint) string {
	return fmt.Sprintf("%s%d", bookCachePrefix, id)
}

// getFromCache 从缓存获取图书
func (r *CachedBookRepository) getFromCache(id uint) (*model.Book, error) {
	key := bookCacheKey(id)
	data, err := r.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var book model.Book
	if err := json.Unmarshal([]byte(data), &book); err != nil {
		r.logger.Warn("failed to unmarshal cached book", zap.Uint("id", id), zap.Error(err))
		return nil, err
	}

	r.logger.Debug("cache hit", zap.Uint("book_id", id))
	return &book, nil
}

// setToCache 写入缓存
func (r *CachedBookRepository) setToCache(book *model.Book) error {
	key := bookCacheKey(book.ID)
	data, err := json.Marshal(book)
	if err != nil {
		r.logger.Warn("failed to marshal book for cache", zap.Uint("id", book.ID), zap.Error(err))
		return err
	}

	if err := r.redis.Set(ctx, key, data, bookCacheTTL).Err(); err != nil {
		r.logger.Warn("failed to set cache", zap.Uint("book_id", book.ID), zap.Error(err))
		return err
	}

	r.logger.Debug("cache set", zap.Uint("book_id", book.ID), zap.Duration("ttl", bookCacheTTL))
	return nil
}

// deleteFromCache 删除缓存
func (r *CachedBookRepository) deleteFromCache(id uint) error {
	key := bookCacheKey(id)
	if err := r.redis.Del(ctx, key).Err(); err != nil {
		r.logger.Warn("failed to delete cache", zap.Uint("book_id", id), zap.Error(err))
		return err
	}

	r.logger.Debug("cache deleted", zap.Uint("book_id", id))
	return nil
}

// FindByID 带缓存的查询
func (r *CachedBookRepository) FindByID(id uint) (*model.Book, error) {
	// 1. 先尝试从缓存获取
	book, err := r.getFromCache(id)
	if err == nil {
		return book, nil
	}

	// 2. 缓存未命中，查询数据库
	if err := r.bookRepository.db.First(&book, id).Error; err != nil {
		return nil, err
	}

	// 3. 写入缓存（异步，不阻塞请求）
	go func() {
		if err := r.setToCache(book); err != nil {
			r.logger.Warn("failed to cache book after query", zap.Uint("id", id))
		}
	}()

	return book, nil
}

// Update 更新图书并清除缓存
func (r *CachedBookRepository) Update(book *model.Book) error {
	// 1. 更新数据库
	if err := r.bookRepository.Update(book); err != nil {
		return err
	}

	// 2. 清除缓存
	r.deleteFromCache(book.ID)

	return nil
}

// Delete 删除图书并清除缓存
func (r *CachedBookRepository) Delete(id uint) error {
	// 1. 删除数据库记录
	if err := r.bookRepository.Delete(id); err != nil {
		return err
	}

	// 2. 清除缓存
	r.deleteFromCache(id)

	return nil
}

// FindByIDs 批量查询带缓存
func (r *CachedBookRepository) FindByIDs(ids []uint) ([]model.Book, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var books []model.Book
	var missedIDs []uint

	// 1. 批量尝试从缓存获取
	for _, id := range ids {
		book, err := r.getFromCache(id)
		if err == nil {
			books = append(books, *book)
		} else {
			missedIDs = append(missedIDs, id)
		}
	}

	// 2. 缓存未命中的，批量查询数据库
	if len(missedIDs) > 0 {
		var dbBooks []model.Book
		if err := r.bookRepository.db.Where("id IN ?", missedIDs).Find(&dbBooks).Error; err != nil {
			return nil, err
		}

		// 3. 将数据库查询结果写入缓存
		for i := range dbBooks {
			books = append(books, dbBooks[i])
			go func(book model.Book) {
				r.setToCache(&book)
			}(dbBooks[i])
		}
	}

	return books, nil
}

// Create 创建图书（不需要缓存，因为刚创建的图书很少立即被查询）
func (r *CachedBookRepository) Create(book *model.Book) error {
	return r.bookRepository.Create(book)
}

// List 列表查询（不缓存，因为查询条件多变）
func (r *CachedBookRepository) List(query *model.BookQuery) ([]model.Book, int64, error) {
	return r.bookRepository.List(query)
}

// UpdateStock 更新库存（库存变化频繁，不缓存）
func (r *CachedBookRepository) UpdateStock(id uint, delta int) error {
	// 更新库存后清除缓存，确保下次查询获取最新库存
	if err := r.bookRepository.UpdateStock(id, delta); err != nil {
		return err
	}
	r.deleteFromCache(id)
	return nil
}
