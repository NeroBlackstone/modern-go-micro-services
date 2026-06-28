package server

import (
	"fmt"
	"net"

	userv1 "modern-micro-services/gen/bookstore/user/v1"
	"modern-micro-services/internal/user/handler"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type GRPCServer struct {
	grpcServer *grpc.Server
	listener   net.Listener
	logger     *zap.Logger
}

func NewGRPCServer(grpcHandler *handler.GRPCHandler, port int, logger *zap.Logger) (*GRPCServer, error) {
	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	userv1.RegisterUserServiceServer(grpcServer, grpcHandler)

	return &GRPCServer{
		grpcServer: grpcServer,
		listener:   lis,
		logger:     logger,
	}, nil
}

func (s *GRPCServer) Start() error {
	s.logger.Info("user-service gRPC server starting", zap.String("addr", s.listener.Addr().String()))
	return s.grpcServer.Serve(s.listener)
}

func (s *GRPCServer) Stop() {
	s.logger.Info("user-service gRPC server stopping")
	s.grpcServer.GracefulStop()
}
