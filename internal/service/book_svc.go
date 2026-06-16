package service

import (
	"modern-micro-services/internal/model"
	"modern-micro-services/internal/repository"

	"gorm.io/gorm"
)

type BookService interface {
	Create(req *model.CreateBookRequest) (*model.Book, error)
	GetByID(id uint) (*model.Book, error)
	Update(id uint, req *model.UpdateBookRequest) (*model.Book, error)
	Delete(id uint) error
	List(query *model.BookQuery) ([]model.Book, int64, error)
}

type bookService struct {
	bookRepo repository.BookRepository
}

func NewBookService(bookRepo repository.BookRepository) BookService {
	return &bookService{bookRepo: bookRepo}
}

func (s *bookService) Create(req *model.CreateBookRequest) (*model.Book, error) {
	book := &model.Book{
		Title:       req.Title,
		Author:      req.Author,
		Price:       req.Price,
		Stock:       req.Stock,
		Description: req.Description,
	}

	if err := s.bookRepo.Create(book); err != nil {
		return nil, err
	}

	return book, nil
}

func (s *bookService) GetByID(id uint) (*model.Book, error) {
	book, err := s.bookRepo.FindByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, err
	}
	return book, nil
}

func (s *bookService) Update(id uint, req *model.UpdateBookRequest) (*model.Book, error) {
	book, err := s.bookRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if req.Title != "" {
		book.Title = req.Title
	}
	if req.Author != "" {
		book.Author = req.Author
	}
	if req.Price > 0 {
		book.Price = req.Price
	}
	if req.Stock != nil {
		book.Stock = *req.Stock
	}
	if req.Description != "" {
		book.Description = req.Description
	}

	if err := s.bookRepo.Update(book); err != nil {
		return nil, err
	}

	return book, nil
}

func (s *bookService) Delete(id uint) error {
	_, err := s.bookRepo.FindByID(id)
	if err != nil {
		return err
	}
	return s.bookRepo.Delete(id)
}

func (s *bookService) List(query *model.BookQuery) ([]model.Book, int64, error) {
	return s.bookRepo.List(query)
}
