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
	memorySvc     *services.ReviewMemoryService
	chatSvc       *services.ReviewChatService
	opponentSvc   *services.OpponentService
}

func NewReviewController() *ReviewController {
	return &ReviewController{
		reviewService: services.NewReviewService(),
		analysisSvc:   services.NewReviewAnalysisService(),
		memorySvc:     services.NewReviewMemoryService(),
		chatSvc:       services.NewReviewChatService(),
		opponentSvc:   &services.OpponentService{},
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
// SearchOpponents 搜索我的对手名单（添加对手弹窗的下拉框用）
func (c *ReviewController) SearchOpponents(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	list, err := c.opponentSvc.SearchOpponents(userID, ctx.Query("keyword"), limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取对手名单失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.OpponentListResponse{List: list}))
}

// GetLeakTags 获取启用的漏洞标签字典
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

// GetProfile 我的复盘画像（M4 长期记忆）
//
// 这里会顺带重算一次统计：漏洞排行是洞察表的派生数据，重算只是几次查询，
// 但能保证画像页永远反映当前洞察表 —— 否则删了手牌、或标签改名之后，
// 页面会一直显示上一次分析时的旧数字，用户无从判断哪个是真的。
// 语义上等同于刷新一次物化视图，不是"GET 顺手改业务数据"。
func (c *ReviewController) GetProfile(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)

	profile, err := c.memorySvc.RefreshProfile(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("读取画像失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ToReviewProfileResponse(profile)))
}

// RefreshProfileSummary 手动触发画像总结重写
//
// 异步：立刻返回，返回体里的 summaryStatus 是 pending，真正的模型调用在后台 ——
// 与「分析手牌」同一套，前端据 summaryStatus 轮询 GET /profile。
// 一次 K3 调用要跑几分钟，同步接口必然被前端或网关先掐断。
//
// 同步报错的只剩"没配模型""画像里还没有洞察"这类；模型调用失败会落在
// summaryStatus=failed 上，由前端展示
func (c *ReviewController) RefreshProfileSummary(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)

	profile, _, err := c.memorySvc.StartSummaryRewrite(userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ToReviewProfileResponse(profile)))
}

// GetInsights 某个漏洞的全部历史证据
//
// 不带 tag_code 时返回所有漏洞的洞察，便于以后做"最近犯的错"时间线
func (c *ReviewController) GetInsights(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)
	tagCode := ctx.Query("tag_code")

	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 {
		limit = 50
	}

	items, err := c.memorySvc.ListInsightsByTag(userID, tagCode, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("读取历史证据失败: "+err.Error()))
		return
	}

	list := make([]dto.ReviewInsightResponse, 0, len(items))
	for _, item := range items {
		list = append(list, dto.ReviewInsightResponse{
			InsightID: item.InsightID,
			HandID:    item.HandID,
			HandTitle: item.HandTitle,
			Position:  item.Position,
			TableSize: item.TableSize,
			HeroCards: item.HeroCards,
			Severity:  item.Severity,
			Evidence:  item.Evidence,
			CreatedAt: item.CreatedAt,
		})
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ReviewInsightListResponse{List: list}))
}

// AskQuestion 对某次分析追问（M5 追问对话）
//
// 异步：立刻返回一问一答两条记录，其中 answer 是 status=pending 的占位行，
// 真正的模型调用在后台 —— 与「分析手牌」同一套，前端据 status 轮询
// GET /analyses/:id/messages。K3 这类「始终推理」模型一次追问要跑几分钟，
// 同步接口必然被前端或网关先掐断。
//
// 追问绑定的是一次分析而不是手牌：手牌被改过并重新分析后是新的一条 analysis，
// 旧对话不会跟过来
//
// 走到 400 的只剩「参数错 / 分析不属于你 / 还没分析结论 / 没配模型」；
// **模型调用失败不再走这里**，它会落在 answer 的 status=failed 上由前端展示
func (c *ReviewController) AskQuestion(ctx *gin.Context) {
	analysisID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || analysisID <= 0 {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的分析ID"))
		return
	}

	var req dto.ReviewMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	question, answer, inflight, err := c.chatSvc.Ask(userID, uint(analysisID), req.Content)
	if err != nil {
		// 归属校验失败、没有分析结论、没配模型都走这里，
		// service 返回的文案已经是给用户看的
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.AskReviewMessageResponse{
		Question: dto.ToReviewMessageResponse(question),
		Answer:   dto.ToReviewMessageResponse(answer),
		Inflight: inflight,
	}))
}

// GetMessages 某次分析的对话历史
func (c *ReviewController) GetMessages(ctx *gin.Context) {
	analysisID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || analysisID <= 0 {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的分析ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	messages, err := c.chatSvc.ListMessages(userID, uint(analysisID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse(err.Error()))
		return
	}

	list := make([]dto.ReviewMessageResponse, 0, len(messages))
	for i := range messages {
		list = append(list, dto.ToReviewMessageResponse(&messages[i]))
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(dto.ReviewMessageListResponse{List: list}))
}
