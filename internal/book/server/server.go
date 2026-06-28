package server

import (
	"fmt"
	"net"

	bookv1 "modern-micro-services/gen/bookstore/book/v1"
	"modern-micro-services/internal/book/handler"

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
	bookv1.RegisterBookServiceServer(grpcServer, grpcHandler)

	return &GRPCServer{
		grpcServer: grpcServer,
		listener:   lis,
		logger:     logger,
	}, nil
}

func (s *GRPCServer) Start() error {
	s.logger.Info("book-service gRPC server starting", zap.String("addr", s.listener.Addr().String()))
	return s.grpcServer.Serve(s.listener)
}

func (s *GRPCServer) Stop() {
	s.logger.Info("book-service gRPC server stopping")
	s.grpcServer.GracefulStop()
}
