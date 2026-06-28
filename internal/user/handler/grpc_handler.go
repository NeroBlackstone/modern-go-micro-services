package handler

import (
	"context"

	userv1 "modern-micro-services/gen/bookstore/user/v1"
	"modern-micro-services/internal/user/service"
)

type GRPCHandler struct {
	userv1.UnimplementedUserServiceServer
	userSvc service.UserService
}

func NewGRPCHandler(userSvc service.UserService) *GRPCHandler {
	return &GRPCHandler{userSvc: userSvc}
}

func (h *GRPCHandler) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	user, err := h.userSvc.GetUserByID(uint(req.GetUserId()))
	if err != nil {
		return nil, err
	}

	return &userv1.GetUserResponse{
		Id:       uint32(user.ID),
		Username: user.Username,
		Email:    user.Email,
	}, nil
}
