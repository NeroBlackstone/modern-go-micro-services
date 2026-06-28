package client

import (
	"context"
	"fmt"

	bookv1 "modern-micro-services/gen/bookstore/book/v1"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// BookClient 封装对 book-service 的 gRPC 调用
type BookClient struct {
	conn   *grpc.ClientConn
	client bookv1.BookServiceClient
	logger *zap.Logger
}

func NewBookClient(addr string, logger *zap.Logger) (*BookClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to book-service: %w", err)
	}

	return &BookClient{
		conn:   conn,
		client: bookv1.NewBookServiceClient(conn),
		logger: logger,
	}, nil
}

// NewBookClientFromConn 使用已有的 gRPC 连接创建 BookClient
func NewBookClientFromConn(conn *grpc.ClientConn, logger *zap.Logger) *BookClient {
	return &BookClient{
		conn:   conn,
		client: bookv1.NewBookServiceClient(conn),
		logger: logger,
	}
}

// GetBook 获取图书信息
func (c *BookClient) GetBook(bookID uint) (*bookv1.GetBookResponse, error) {
	resp, err := c.client.GetBook(context.Background(), &bookv1.GetBookRequest{
		Id: uint32(bookID),
	})
	if err != nil {
		return nil, fmt.Errorf("get book %d: %w", bookID, err)
	}
	return resp, nil
}

// GetBooks 批量获取图书
func (c *BookClient) GetBooks(bookIDs []uint) ([]*bookv1.GetBookResponse, error) {
	ids := make([]uint32, len(bookIDs))
	for i, id := range bookIDs {
		ids[i] = uint32(id)
	}

	resp, err := c.client.GetBooks(context.Background(), &bookv1.GetBooksRequest{
		Ids: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("get books: %w", err)
	}
	return resp.Books, nil
}

// DeductStock 扣减库存
func (c *BookClient) DeductStock(bookID uint, quantity int) error {
	resp, err := c.client.DeductStock(context.Background(), &bookv1.DeductStockRequest{
		BookId:   uint32(bookID),
		Quantity: int32(quantity),
	})
	if err != nil {
		return fmt.Errorf("deduct stock for book %d: %w", bookID, err)
	}
	if !resp.Success {
		return fmt.Errorf("deduct stock failed: %s", resp.Message)
	}
	return nil
}

// RestoreStock 恢复库存（Saga 补偿）
func (c *BookClient) RestoreStock(bookID uint, quantity int) error {
	resp, err := c.client.RestoreStock(context.Background(), &bookv1.RestoreStockRequest{
		BookId:   uint32(bookID),
		Quantity: int32(quantity),
	})
	if err != nil {
		return fmt.Errorf("restore stock for book %d: %w", bookID, err)
	}
	if !resp.Success {
		return fmt.Errorf("restore stock failed: %s", resp.Message)
	}
	return nil
}

func (c *BookClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
