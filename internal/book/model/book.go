package model

import "time"

type Book struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Title       string    `json:"title" gorm:"size:200;not null"`
	Author      string    `json:"author" gorm:"size:100;not null"`
	Price       float64   `json:"price" gorm:"not null"`
	Stock       int       `json:"stock" gorm:"not null;default:0"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Book) TableName() string {
	return "books"
}

type CreateBookRequest struct {
	Title       string  `json:"title" binding:"required,max=200"`
	Author      string  `json:"author" binding:"required,max=100"`
	Price       float64 `json:"price" binding:"required,gt=0"`
	Stock       int     `json:"stock" binding:"required,gte=0"`
	Description string  `json:"description" binding:"max=2000"`
}

type UpdateBookRequest struct {
	Title       string  `json:"title" binding:"omitempty,max=200"`
	Author      string  `json:"author" binding:"omitempty,max=100"`
	Price       float64 `json:"price" binding:"omitempty,gt=0"`
	Stock       *int    `json:"stock" binding:"omitempty,gte=0"`
	Description string  `json:"description" binding:"omitempty,max=2000"`
}

type BookQuery struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword  string `form:"keyword"`
	Author   string `form:"author"`
}

func (q *BookQuery) GetPage() int {
	if q.Page <= 0 {
		q.Page = 1
	}
	return q.Page
}

func (q *BookQuery) GetPageSize() int {
	if q.PageSize <= 0 {
		q.PageSize = 10
	}
	return q.PageSize
}
