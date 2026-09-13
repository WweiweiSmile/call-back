package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ScoreRequestController struct {
	scoreRequestService *services.ScoreRequestService
}

func NewScoreRequestController() *ScoreRequestController {
	return &ScoreRequestController{
		scoreRequestService: &services.ScoreRequestService{},
	}
}

// Create 提交存取分申请
func (c *ScoreRequestController) Create(ctx *gin.Context) {
	var req dto.CreateScoreRequestRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	result, err := c.scoreRequestService.Create(userID, &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// GetList 申请列表。scope=mine 看自己提交的（默认），scope=review 看自己创建场次的待审
func (c *ScoreRequestController) GetList(ctx *gin.Context) {
	gameID, _ := strconv.ParseUint(ctx.Query("game_id"), 10, 32)
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	query := services.ScoreRequestQuery{
		GameID:   uint(gameID),
		Status:   ctx.Query("status"),
		Scope:    ctx.DefaultQuery("scope", "mine"),
		Page:     page,
		PageSize: pageSize,
	}

	userID := middleware.GetUserID(ctx)

	list, err := c.scoreRequestService.GetList(userID, query)
	if err != nil {
		// 权限类错误一律 400，不能返 401——前端见 401 会清 token 跳登录
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(list))
}

// Approve 审核通过
func (c *ScoreRequestController) Approve(ctx *gin.Context) {
	requestID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的申请ID"))
		return
	}

	var req dto.ReviewScoreRequestRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	result, err := c.scoreRequestService.Approve(userID, uint(requestID), &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// Reject 审核驳回
func (c *ScoreRequestController) Reject(ctx *gin.Context) {
	requestID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的申请ID"))
		return
	}

	var req dto.ReviewScoreRequestRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	result, err := c.scoreRequestService.Reject(userID, uint(requestID), &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// Cancel 撤销自己的待审申请
func (c *ScoreRequestController) Cancel(ctx *gin.Context) {
	requestID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的申请ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	if err := c.scoreRequestService.Cancel(userID, uint(requestID)); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}
