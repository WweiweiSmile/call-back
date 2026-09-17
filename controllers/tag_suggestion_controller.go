package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// TagSuggestionController AI 新标签的入库审批。
// 权限判定（必须系统管理）在 service 层做，与其他模块的归属校验保持一致
type TagSuggestionController struct {
	tagSuggestionService *services.TagSuggestionService
}

func NewTagSuggestionController() *TagSuggestionController {
	return &TagSuggestionController{
		tagSuggestionService: &services.TagSuggestionService{},
	}
}

// Approve 审批通过，标签写入字典并对所有人生效
func (c *TagSuggestionController) Approve(ctx *gin.Context) {
	suggestionID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的标签建议ID"))
		return
	}

	var req dto.ApproveTagSuggestionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	operatorID := middleware.GetUserID(ctx)

	result, err := c.tagSuggestionService.Approve(operatorID, uint(suggestionID), &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// Reject 驳回标签建议
func (c *TagSuggestionController) Reject(ctx *gin.Context) {
	suggestionID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的标签建议ID"))
		return
	}

	var req dto.RejectTagSuggestionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	operatorID := middleware.GetUserID(ctx)

	result, err := c.tagSuggestionService.Reject(operatorID, uint(suggestionID), &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}
