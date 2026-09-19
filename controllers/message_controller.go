package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/models"
	"call-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type MessageController struct {
	messageService *services.MessageService
}

func NewMessageController() *MessageController {
	return &MessageController{
		messageService: &services.MessageService{},
	}
}

// GetList 获取当前用户的消息列表
func (c *MessageController) GetList(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	var isRead *bool
	if raw := ctx.Query("is_read"); raw != "" {
		value := raw == "true" || raw == "1"
		isRead = &value
	}

	// 审批状态：all（或省略）/ pending 待我处理 / handled 其余。
	// 未知取值直接报错，不静默当成全部
	scope := ctx.DefaultQuery("scope", models.MessageScopeAll)
	if !models.IsValidMessageScope(scope) {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的 scope："+scope))
		return
	}

	userID := middleware.GetUserID(ctx)

	list, err := c.messageService.GetList(userID, services.MessageListFilter{
		IsRead: isRead,
		Scope:  scope,
	}, page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取消息失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(list))
}

// GetDetail 获取单条消息详情（只能看自己的）
func (c *MessageController) GetDetail(ctx *gin.Context) {
	messageID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的消息ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	detail, err := c.messageService.GetByID(userID, uint(messageID))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(detail))
}

// GetUnreadCount 获取未读消息数（前端轮询用的廉价接口）
func (c *MessageController) GetUnreadCount(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)

	count, err := c.messageService.GetUnreadCount(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取未读数失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.UnreadCountResponse{Count: count}))
}

// MarkRead 标记单条已读
func (c *MessageController) MarkRead(ctx *gin.Context) {
	messageID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的消息ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	if err := c.messageService.MarkRead(userID, uint(messageID)); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// MarkAllRead 标记全部已读，可传 gameId 只清某个场次的
func (c *MessageController) MarkAllRead(ctx *gin.Context) {
	var req struct {
		GameID *uint `json:"gameId"`
	}
	// 没有请求体时按"全部已读"处理，不报错
	_ = ctx.ShouldBindJSON(&req)

	userID := middleware.GetUserID(ctx)

	if err := c.messageService.MarkAllRead(userID, req.GameID); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}
