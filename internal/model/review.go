package model

import "time"

// Review 评价模型
type Review struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;index"`
	BookID    uint      `json:"book_id" gorm:"not null;index"`
	Rating    int       `json:"rating" gorm:"not null"` // 1-5
	Comment   string    `json:"comment" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
}

func (Review) TableName() string {
	return "reviews"
}

// CreateReviewRequest 创建评价请求
type CreateReviewRequest struct {
	BookID  uint   `json:"book_id" binding:"required"`
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Comment string `json:"comment" binding:"max=2000"`
}

// ReviewResponse 评价响应（包含用户信息）
type ReviewResponse struct {
	ID        uint         `json:"id"`
	Rating    int          `json:"rating"`
	Comment   string       `json:"comment"`
	CreatedAt time.Time    `json:"created_at"`
	User      UserResponse `json:"user"`
}

// ReviewStats 评价统计
type ReviewStats struct {
	AverageRating float64 `json:"average_rating"`
	TotalReviews  int64   `json:"total_reviews"`
	Rating1Count  int64   `json:"rating_1_count"`
	Rating2Count  int64   `json:"rating_2_count"`
	Rating3Count  int64   `json:"rating_3_count"`
	Rating4Count  int64   `json:"rating_4_count"`
	Rating5Count  int64   `json:"rating_5_count"`
}

// ReviewQuery 评价查询参数
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
