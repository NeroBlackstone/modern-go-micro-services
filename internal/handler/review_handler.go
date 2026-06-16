package handler

import (
	"strconv"

	"modern-micro-services/internal/middleware"
	"modern-micro-services/internal/model"
	"modern-micro-services/internal/service"
	"modern-micro-services/pkg/response"

	"github.com/gin-gonic/gin"
)

type ReviewHandler struct {
	reviewSvc service.ReviewService
}

func NewReviewHandler(reviewSvc service.ReviewService) *ReviewHandler {
	return &ReviewHandler{reviewSvc: reviewSvc}
}

// CreateReview 发表评价
// @Summary      发表评价
// @Description  用户对已购买的图书发表评价
// @Tags         评价模块
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body model.CreateReviewRequest true "创建评价请求"
// @Success      200 {object} response.Response{data=model.Review}
// @Failure      400 {object} response.Response
// @Failure      401 {object} response.Response
// @Router       /api/v1/reviews [post]
func (h *ReviewHandler) CreateReview(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		return
	}

	var req model.CreateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	review, err := h.reviewSvc.Create(userID, &req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	response.Success(c, review)
}

// ListReviews 查看图书评价列表
// @Summary      查看图书评价列表
// @Description  获取指定图书的评价列表
// @Tags         评价模块
// @Produce      json
// @Param        book_id path int true "图书ID"
// @Param        page query int false "页码" default(1)
// @Param        page_size query int false "每页数量" default(10)
// @Success      200 {object} response.Response{data=response.PageResult}
// @Router       /api/v1/reviews/book/{book_id} [get]
func (h *ReviewHandler) ListReviews(c *gin.Context) {
	bookID, err := strconv.ParseUint(c.Param("book_id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid book id")
		return
	}

	var query model.ReviewQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	reviews, total, err := h.reviewSvc.ListByBookID(uint(bookID), &query)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, response.PageResult{
		List:     reviews,
		Total:    total,
		Page:     query.GetPage(),
		PageSize: query.GetPageSize(),
	})
}

// GetReviewStats 获取图书评价统计
// @Summary      获取图书评价统计
// @Description  获取指定图书的评分统计信息
// @Tags         评价模块
// @Produce      json
// @Param        book_id path int true "图书ID"
// @Success      200 {object} response.Response{data=model.ReviewStats}
// @Router       /api/v1/reviews/book/{book_id}/stats [get]
func (h *ReviewHandler) GetReviewStats(c *gin.Context) {
	bookID, err := strconv.ParseUint(c.Param("book_id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid book id")
		return
	}

	stats, err := h.reviewSvc.GetStats(uint(bookID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, stats)
}
