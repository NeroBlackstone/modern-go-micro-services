package handler

import (
	"strconv"

	"modern-micro-services/internal/middleware"
	"modern-micro-services/internal/model"
	"modern-micro-services/internal/service"
	"modern-micro-services/pkg/response"

	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	orderSvc service.OrderService
}

func NewOrderHandler(orderSvc service.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

// CreateOrder 创建订单
// @Summary      创建订单
// @Description  用户提交订单，系统验证库存并扣减
// @Tags         订单模块
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body model.CreateOrderRequest true "创建订单请求"
// @Success      200 {object} response.Response{data=model.Order}
// @Failure      400 {object} response.Response
// @Failure      401 {object} response.Response
// @Router       /api/v1/orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	var req model.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	order, err := h.orderSvc.Create(userID, &req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, order)
}

// GetOrder 获取订单详情
// @Summary      获取订单详情
// @Description  根据ID获取订单详情
// @Tags         订单模块
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "订单ID"
// @Success      200 {object} response.Response{data=model.Order}
// @Failure      404 {object} response.Response
// @Router       /api/v1/orders/{id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid order id")
		return
	}

	order, err := h.orderSvc.GetByID(uint(id), userID)
	if err != nil {
		response.NotFound(c, "order not found")
		return
	}

	response.Success(c, order)
}

// ListOrders 订单列表
// @Summary      订单列表
// @Description  获取当前用户的订单列表
// @Tags         订单模块
// @Produce      json
// @Security     BearerAuth
// @Param        page query int false "页码" default(1)
// @Param        page_size query int false "每页数量" default(10)
// @Param        status query string false "订单状态筛选"
// @Success      200 {object} response.Response{data=response.PageResult}
// @Failure      401 {object} response.Response
// @Router       /api/v1/orders [get]
func (h *OrderHandler) ListOrders(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	var query model.OrderQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	orders, total, err := h.orderSvc.ListByUserID(userID, &query)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, response.PageResult{
		List:     orders,
		Total:    total,
		Page:     query.GetPage(),
		PageSize: query.GetPageSize(),
	})
}

// UpdateOrderStatus 更新订单状态
// @Summary      更新订单状态
// @Description  更新订单状态（如支付、发货等）
// @Tags         订单模块
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "订单ID"
// @Param        request body object true "状态更新请求"
// @Success      200 {object} response.Response
// @Failure      400 {object} response.Response
// @Router       /api/v1/orders/{id}/status [put]
func (h *OrderHandler) UpdateOrderStatus(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid order id")
		return
	}

	var req struct {
		Status model.OrderStatus `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.orderSvc.UpdateStatus(uint(id), userID, req.Status); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "order status updated", nil)
}
