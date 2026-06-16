package handler

import (
	"strconv"

	"modern-micro-services/internal/model"
	"modern-micro-services/internal/service"
	"modern-micro-services/pkg/response"

	"github.com/gin-gonic/gin"
)

type BookHandler struct {
	bookSvc service.BookService
}

func NewBookHandler(bookSvc service.BookService) *BookHandler {
	return &BookHandler{bookSvc: bookSvc}
}

// CreateBook 创建图书
// @Summary      创建图书
// @Description  创建一本新图书（管理员接口）
// @Tags         图书模块
// @Accept       json
// @Produce      json
// @Param        request body model.CreateBookRequest true "创建图书请求"
// @Success      200 {object} response.Response{data=model.Book}
// @Failure      400 {object} response.Response
// @Router       /api/v1/books [post]
func (h *BookHandler) CreateBook(c *gin.Context) {
	var req model.CreateBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	book, err := h.bookSvc.Create(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, book)
}

// GetBook 获取图书详情
// @Summary      获取图书详情
// @Description  根据ID获取图书详细信息
// @Tags         图书模块
// @Produce      json
// @Param        id path int true "图书ID"
// @Success      200 {object} response.Response{data=model.Book}
// @Failure      404 {object} response.Response
// @Router       /api/v1/books/{id} [get]
func (h *BookHandler) GetBook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid book id")
		return
	}

	book, err := h.bookSvc.GetByID(uint(id))
	if err != nil {
		response.NotFound(c, "book not found")
		return
	}

	response.Success(c, book)
}

// UpdateBook 更新图书
// @Summary      更新图书
// @Description  更新图书信息（管理员接口）
// @Tags         图书模块
// @Accept       json
// @Produce      json
// @Param        id path int true "图书ID"
// @Param        request body model.UpdateBookRequest true "更新图书请求"
// @Success      200 {object} response.Response{data=model.Book}
// @Failure      400 {object} response.Response
// @Router       /api/v1/books/{id} [put]
func (h *BookHandler) UpdateBook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid book id")
		return
	}

	var req model.UpdateBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	book, err := h.bookSvc.Update(uint(id), &req)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.Success(c, book)
}

// DeleteBook 删除图书
// @Summary      删除图书
// @Description  根据ID删除图书（管理员接口）
// @Tags         图书模块
// @Produce      json
// @Param        id path int true "图书ID"
// @Success      200 {object} response.Response
// @Failure      404 {object} response.Response
// @Router       /api/v1/books/{id} [delete]
func (h *BookHandler) DeleteBook(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid book id")
		return
	}

	if err := h.bookSvc.Delete(uint(id)); err != nil {
		response.NotFound(c, "book not found")
		return
	}

	response.SuccessWithMessage(c, "book deleted", nil)
}

// ListBooks 图书列表
// @Summary      图书列表
// @Description  获取图书列表，支持分页和搜索
// @Tags         图书模块
// @Produce      json
// @Param        page query int false "页码" default(1)
// @Param        page_size query int false "每页数量" default(10)
// @Param        keyword query string false "搜索关键词（标题或作者）"
// @Param        author query string false "按作者筛选"
// @Success      200 {object} response.Response{data=response.PageResult}
// @Router       /api/v1/books [get]
func (h *BookHandler) ListBooks(c *gin.Context) {
	var query model.BookQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	books, total, err := h.bookSvc.List(&query)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, response.PageResult{
		List:     books,
		Total:    total,
		Page:     query.GetPage(),
		PageSize: query.GetPageSize(),
	})
}
