package handler

import (
	"modern-micro-services/internal/middleware"
	"modern-micro-services/internal/model"
	"modern-micro-services/internal/service"
	"modern-micro-services/pkg/response"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userSvc service.UserService
}

func NewUserHandler(userSvc service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// Register 用户注册
// @Summary      用户注册
// @Description  用户通过邮箱和密码注册新账号
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body model.RegisterRequest true "注册请求"
// @Success      200 {object} response.Response{data=model.LoginResponse}
// @Failure      400 {object} response.Response
// @Router       /api/v1/auth/register [post]
func (h *UserHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.userSvc.Register(&req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, result)
}

// Login 用户登录
// @Summary      用户登录
// @Description  用户通过邮箱和密码登录获取Token
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Param        request body model.LoginRequest true "登录请求"
// @Success      200 {object} response.Response{data=model.LoginResponse}
// @Failure      400 {object} response.Response
// @Router       /api/v1/auth/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.userSvc.Login(&req)
	if err != nil {
		response.Unauthorized(c, err.Error())
		return
	}

	response.Success(c, result)
}

// GetProfile 获取用户信息
// @Summary      获取用户信息
// @Description  获取当前登录用户的个人信息
// @Tags         用户模块
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=model.UserResponse}
// @Failure      401 {object} response.Response
// @Router       /api/v1/user/profile [get]
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	user, err := h.userSvc.GetProfile(userID)
	if err != nil {
		response.NotFound(c, "user not found")
		return
	}

	response.Success(c, user)
}

// UpdateProfile 更新用户信息
// @Summary      更新用户信息
// @Description  更新当前登录用户的个人信息
// @Tags         用户模块
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body model.UpdateUserRequest true "更新请求"
// @Success      200 {object} response.Response{data=model.UserResponse}
// @Failure      400 {object} response.Response
// @Router       /api/v1/user/profile [put]
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	user, err := h.userSvc.UpdateProfile(userID, &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, user)
}
