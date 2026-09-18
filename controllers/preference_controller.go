package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PreferenceController 用户默认设置（目前只有复盘用的盲注默认值）。
// 设置是按用户私有的，用户 ID 一律取自 JWT，不接受前端传入。
type PreferenceController struct {
	preferenceService *services.PreferenceService
}

func NewPreferenceController() *PreferenceController {
	return &PreferenceController{
		preferenceService: &services.PreferenceService{},
	}
}

// Get 读取我的默认设置。没设置过的用户拿到一套默认值，不是 404
func (c *PreferenceController) Get(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)

	result, err := c.preferenceService.Get(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("读取设置失败"))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// Update 保存我的默认设置
func (c *PreferenceController) Update(ctx *gin.Context) {
	var req dto.UpdateUserPreferenceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	result, err := c.preferenceService.Update(userID, &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}
