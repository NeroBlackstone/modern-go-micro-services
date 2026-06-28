package handler

import (
	"context"

	bookv1 "modern-micro-services/gen/bookstore/book/v1"
	"modern-micro-services/internal/book/service"
)

type GRPCHandler struct {
	bookv1.UnimplementedBookServiceServer
	bookSvc service.BookService
}

func NewGRPCHandler(bookSvc service.BookService) *GRPCHandler {
	return &GRPCHandler{bookSvc: bookSvc}
}

func (h *GRPCHandler) GetBook(ctx context.Context, req *bookv1.GetBookRequest) (*bookv1.GetBookResponse, error) {
	book, err := h.bookSvc.GetByID(uint(req.GetId()))
	if err != nil {
		return nil, err
	}

	return &bookv1.GetBookResponse{
		Id:          uint32(book.ID),
		Title:       book.Title,
		Author:      book.Author,
		Price:       book.Price,
		Stock:       int32(book.Stock),
		Description: book.Description,
	}, nil
}

func (h *GRPCHandler) GetBooks(ctx context.Context, req *bookv1.GetBooksRequest) (*bookv1.GetBooksResponse, error) {
	ids := make([]uint, len(req.GetIds()))
	for i, id := range req.GetIds() {
		ids[i] = uint(id)
	}

	books, err := h.bookSvc.FindByIDs(ids)
	if err != nil {
		return nil, err
	}

	resp := &bookv1.GetBooksResponse{
		Books: make([]*bookv1.GetBookResponse, 0, len(books)),
	}

	for _, book := range books {
		resp.Books = append(resp.Books, &bookv1.GetBookResponse{
			Id:          uint32(book.ID),
			Title:       book.Title,
			Author:      book.Author,
			Price:       book.Price,
			Stock:       int32(book.Stock),
			Description: book.Description,
		})
	}

	return resp, nil
}

func (h *GRPCHandler) DeductStock(ctx context.Context, req *bookv1.DeductStockRequest) (*bookv1.DeductStockResponse, error) {
	err := h.bookSvc.DeductStock(uint(req.GetBookId()), int(req.GetQuantity()))
	if err != nil {
		return &bookv1.DeductStockResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &bookv1.DeductStockResponse{
		Success: true,
		Message: "stock deducted successfully",
	}, nil
}

func (h *GRPCHandler) RestoreStock(ctx context.Context, req *bookv1.RestoreStockRequest) (*bookv1.RestoreStockResponse, error) {
	err := h.bookSvc.RestoreStock(uint(req.GetBookId()), int(req.GetQuantity()))
	if err != nil {
		return &bookv1.RestoreStockResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &bookv1.RestoreStockResponse{
		Success: true,
		Message: "stock restored successfully",
	}, nil
}
