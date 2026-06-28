package service

import (
	"errors"

	"modern-micro-services/internal/order/model"
	"modern-micro-services/internal/order/repository"
)

type ReviewService interface {
	Create(userID uint, req *model.CreateReviewRequest) (*model.Review, error)
	ListByBookID(bookID uint, query *model.ReviewQuery) ([]model.Review, int64, error)
	GetStats(bookID uint) (*model.ReviewStats, error)
}

type reviewService struct {
	reviewRepo repository.ReviewRepository
}

func NewReviewService(reviewRepo repository.ReviewRepository) ReviewService {
	return &reviewService{reviewRepo: reviewRepo}
}

func (s *reviewService) Create(userID uint, req *model.CreateReviewRequest) (*model.Review, error) {
	purchased, err := s.reviewRepo.HasPurchased(userID, req.BookID)
	if err != nil {
		return nil, err
	}
	if !purchased {
		return nil, errors.New("you can only review books you have purchased")
	}

	reviewed, err := s.reviewRepo.HasReviewed(userID, req.BookID)
	if err != nil {
		return nil, err
	}
	if reviewed {
		return nil, errors.New("you have already reviewed this book")
	}

	review := &model.Review{
		UserID:  userID,
		BookID:  req.BookID,
		Rating:  req.Rating,
		Comment: req.Comment,
	}

	if err := s.reviewRepo.Create(review); err != nil {
		return nil, err
	}

	return review, nil
}

func (s *reviewService) ListByBookID(bookID uint, query *model.ReviewQuery) ([]model.Review, int64, error) {
	return s.reviewRepo.FindByBookID(bookID, query)
}

func (s *reviewService) GetStats(bookID uint) (*model.ReviewStats, error) {
	return s.reviewRepo.GetStats(bookID)
}
