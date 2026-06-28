package client

import (
	"context"
	"fmt"

	userv1 "modern-micro-services/gen/bookstore/user/v1"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// UserClient 封装对 user-service 的 gRPC 调用
type UserClient struct {
	conn   *grpc.ClientConn
	client userv1.UserServiceClient
	logger *zap.Logger
}

func NewUserClient(addr string, logger *zap.Logger) (*UserClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to user-service: %w", err)
	}

	return &UserClient{
		conn:   conn,
		client: userv1.NewUserServiceClient(conn),
		logger: logger,
	}, nil
}

// NewUserClientFromConn 使用已有的 gRPC 连接创建 UserClient
func NewUserClientFromConn(conn *grpc.ClientConn, logger *zap.Logger) *UserClient {
	return &UserClient{
		conn:   conn,
		client: userv1.NewUserServiceClient(conn),
		logger: logger,
	}
}

func (c *UserClient) GetUser(userID uint) (*userv1.GetUserResponse, error) {
	resp, err := c.client.GetUser(context.Background(), &userv1.GetUserRequest{
		UserId: uint32(userID),
	})
	if err != nil {
		return nil, fmt.Errorf("get user %d: %w", userID, err)
	}
	return resp, nil
}

func (c *UserClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
