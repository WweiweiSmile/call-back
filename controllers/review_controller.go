package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ReviewController struct {
	reviewService *services.ReviewService
	analysisSvc   *services.ReviewAnalysisService
}

func NewReviewController() *ReviewController {
	return &ReviewController{
		reviewService: &services.ReviewService{},
		analysisSvc:   services.NewReviewAnalysisService(),
	}
}

// CreateHand 创建复盘手牌
func (c *ReviewController) CreateHand(ctx *gin.Context) {
	var req dto.ReviewHandRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	hand, err := c.reviewService.CreateHand(userID, &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ToReviewHandResponse(hand, "")))
}

// GetHandList 获取复盘手牌列表
func (c *ReviewController) GetHandList(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	filter := services.ReviewHandFilter{
		Position:      ctx.Query("position"),
		Tag:           ctx.Query("tag"),
		AnalyzeStatus: ctx.Query("analyze_status"),
		Keyword:       ctx.Query("keyword"),
		Page:          page,
		PageSize:      pageSize,
	}

	if raw := ctx.Query("game_id"); raw != "" {
		gameID, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的场次ID"))
			return
		}
		id := uint(gameID)
		filter.GameID = &id
	}

	userID := middleware.GetUserID(ctx)

	result, err := c.reviewService.GetHandList(userID, filter)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取复盘列表失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(result))
}

// GetHand 获取手牌详情
func (c *ReviewController) GetHand(ctx *gin.Context) {
	handID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的手牌ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	resp, err := c.reviewService.GetHandDetail(userID, uint(handID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// UpdateHand 更新手牌
func (c *ReviewController) UpdateHand(ctx *gin.Context) {
	handID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的手牌ID"))
		return
	}

	var req dto.ReviewHandRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	hand, err := c.reviewService.UpdateHand(userID, uint(handID), &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ToReviewHandResponse(hand, "")))
}

// DeleteHand 删除手牌
func (c *ReviewController) DeleteHand(ctx *gin.Context) {
	handID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的手牌ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	if err := c.reviewService.DeleteHand(userID, uint(handID)); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// GetLeakTags 获取漏洞标签字典
func (c *ReviewController) GetLeakTags(ctx *gin.Context) {
	tags, err := c.reviewService.GetLeakTags()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取标签字典失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ReviewLeakTagListResponse{List: tags}))
}

// AnalyzeHand 触发 AI 分析。
//
// 立即返回 pending 状态的记录，真正的模型调用在后台跑 ——
// 一次分析要 20~60 秒，同步接口必然超时，而小程序端也不支持 SSE
func (c *ReviewController) AnalyzeHand(ctx *gin.Context) {
	handID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的手牌ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	analysis, reused, err := c.analysisSvc.RequestAnalysis(userID, uint(handID))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.RequestAnalysisResponse{
		Analysis: dto.ToReviewAnalysisResponse(analysis),
		Reused:   reused,
	}))
}

// GetAnalysis 查询分析状态与结果（前端轮询用）
func (c *ReviewController) GetAnalysis(ctx *gin.Context) {
	analysisID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的分析ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	analysis, err := c.analysisSvc.GetAnalysis(userID, uint(analysisID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ToReviewAnalysisResponse(analysis)))
}

// GetHandAnalyses 某手牌的历史分析列表
func (c *ReviewController) GetHandAnalyses(ctx *gin.Context) {
	handID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的手牌ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	// 取当前手牌的内容指纹，用来判断每条历史结论是否还对应现在的内容
	hand, err := c.reviewService.GetHand(userID, uint(handID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse(err.Error()))
		return
	}

	list, err := c.analysisSvc.ListByHand(userID, uint(handID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse(err.Error()))
		return
	}

	items := make([]dto.ReviewAnalysisResponse, 0, len(list))
	for i := range list {
		resp := dto.ToReviewAnalysisResponse(&list[i])
		// 指纹不一致说明这手牌改过了，这条结论不再对应当前内容
		resp.Stale = list[i].ContentHash != "" && list[i].ContentHash != hand.ContentHash
		items = append(items, resp)
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ReviewAnalysisListResponse{List: items}))
}

// GetAIStatus AI 是否可用、今日还剩几次
func (c *ReviewController) GetAIStatus(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)
	status := c.analysisSvc.GetAIStatus(userID)

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.AIStatusResponse{
		Enabled:    status.Enabled,
		DailyLimit: status.DailyLimit,
		UsedToday:  status.UsedToday,
		Remaining:  status.Remaining,
	}))
}
