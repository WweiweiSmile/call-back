package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"call-go/utils"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PreferenceController 用户默认设置：复盘用的盲注默认值 + BYOK 模型配置。
// 设置是按用户私有的，用户 ID 一律取自 JWT，不接受前端传入。
//
// 两块的写入口是分开的（PUT "" 与 PUT /ai）：盲注是"整体替换的值"，
// API Key 是"永不下发明文的秘密"，混在一个 PUT 里，"只改盲注"这条请求
// 在结构上就可能把 Key 写没
type PreferenceController struct {
	preferenceService *services.PreferenceService
	aiSettingService  *services.AISettingService
}

func NewPreferenceController() *PreferenceController {
	return &PreferenceController{
		preferenceService: &services.PreferenceService{},
		aiSettingService:  &services.AISettingService{},
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

// GetAI 读取我的模型配置。
//
// 没配置过的用户拿到默认预设 + hasApiKey=false，不是 404；响应里只有掩码，
// 任何情况下都不下发明文 Key
func (c *PreferenceController) GetAI(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)

	result, err := c.aiSettingService.Get(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("读取模型设置失败"))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// UpdateAI 保存我的模型配置。
//
// 状态码与相邻的 Update 有一处**有意的不一致**：主密钥缺失导致加密失败时返回
// 500 而不是 400。那不是用户参数的问题，是服务端配置的问题，
// 报 400 会让用户以为是自己填错了
func (c *PreferenceController) UpdateAI(ctx *gin.Context) {
	var req dto.UpdateAIModelSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	result, err := c.aiSettingService.Update(userID, &req)
	if err != nil {
		// 主密钥缺失是服务端配置问题，不是用户参数问题，所以这里报 500。
		// 与相邻的 Update"一律 400"有意不一致：报 400 会让用户以为是自己填错了
		if errors.Is(err, utils.ErrEncryptionUnavailable) {
			ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse(err.Error()))
			return
		}
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}
