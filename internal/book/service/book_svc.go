package service

import (
	"fmt"

	"modern-micro-services/internal/book/model"
	"modern-micro-services/internal/book/repository"

	"gorm.io/gorm"
)

type BookService interface {
	Create(req *model.CreateBookRequest) (*model.Book, error)
	GetByID(id uint) (*model.Book, error)
	Update(id uint, req *model.UpdateBookRequest) (*model.Book, error)
	Delete(id uint) error
	List(query *model.BookQuery) ([]model.Book, int64, error)
	FindByIDs(ids []uint) ([]model.Book, error)
	DeductStock(bookID uint, quantity int) error
	RestoreStock(bookID uint, quantity int) error
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
	return s.bookRepo.FindByID(id)
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

func (s *bookService) FindByIDs(ids []uint) ([]model.Book, error) {
	return s.bookRepo.FindByIDs(ids)
}

func (s *bookService) DeductStock(bookID uint, quantity int) error {
	if err := s.bookRepo.UpdateStock(bookID, -quantity); err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("book %d not found or insufficient stock", bookID)
		}
		return fmt.Errorf("failed to deduct stock for book %d: %w", bookID, err)
	}
	return nil
}

func (s *bookService) RestoreStock(bookID uint, quantity int) error {
	return s.bookRepo.UpdateStock(bookID, quantity)
}
