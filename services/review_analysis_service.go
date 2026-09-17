package services

import (
	"call-go/config"
	"call-go/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ReviewAnalysisService 复盘分析编排
type ReviewAnalysisService struct {
	reviewService *ReviewService
	aiClient      *AIClient
	memoryService *ReviewMemoryService
	suggestionSvc *TagSuggestionService
}

func NewReviewAnalysisService() *ReviewAnalysisService {
	return &ReviewAnalysisService{
		reviewService: &ReviewService{},
		aiClient:      NewAIClient(),
		memoryService: NewReviewMemoryService(),
		suggestionSvc: &TagSuggestionService{},
	}
}

// AIStatus 前端用来判断能否触发分析
type AIStatus struct {
	Enabled    bool `json:"enabled"`
	DailyLimit int  `json:"dailyLimit"`
	UsedToday  int  `json:"usedToday"`
	Remaining  int  `json:"remaining"`
}

// GetAIStatus 返回 AI 可用状态与今日剩余额度。
//
// 前端据此把按钮置灰并说明原因，比让用户点了再看到失败要好
func (s *ReviewAnalysisService) GetAIStatus(userID uint) *AIStatus {
	settings := config.AIConfig()
	status := &AIStatus{
		Enabled:    settings.Enabled,
		DailyLimit: settings.DailyLimit,
	}
	status.UsedToday = s.countTodayUsage(userID)
	status.Remaining = status.DailyLimit - status.UsedToday
	if status.Remaining < 0 {
		status.Remaining = 0
	}
	return status
}

// countTodayUsage 统计用户今天发起过的分析次数
func (s *ReviewAnalysisService) countTodayUsage(userID uint) int {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var count int64
	config.DB.Model(&models.ReviewAnalysis{}).
		Where("user_id = ? AND created_at >= ?", userID, startOfDay).
		Count(&count)
	return int(count)
}

// RequestAnalysis 触发一次分析。
//
// 若手牌内容与上次分析完全一致，直接返回上次结果而不重复调用模型 ——
// 用户反复点"分析"不该反复烧钱。
func (s *ReviewAnalysisService) RequestAnalysis(userID, handID uint) (*models.ReviewAnalysis, bool, error) {
	if !config.AIConfig().Enabled {
		return nil, false, errors.New("服务端未配置 AI，暂时无法分析")
	}

	hand, err := s.reviewService.GetHand(userID, handID)
	if err != nil {
		return nil, false, err
	}

	// 内容没变就复用上次成功的结果
	var reused models.ReviewAnalysis
	err = config.DB.Where("hand_id = ? AND user_id = ? AND content_hash = ? AND status = ?",
		handID, userID, hand.ContentHash, models.AnalysisStatusDone).
		Order("id DESC").First(&reused).Error
	if err == nil {
		// 复用不消耗额度：本来就没调用模型
		return &reused, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	// 额度检查放在缓存判断之后：复用不花钱，不该被额度挡掉
	status := s.GetAIStatus(userID)
	if status.Remaining <= 0 {
		return nil, false, fmt.Errorf("今日分析次数已用完（上限 %d 次），明天再来", status.DailyLimit)
	}

	analysis := &models.ReviewAnalysis{
		HandID:        handID,
		UserID:        userID,
		Status:        models.AnalysisStatusPending,
		Model:         config.AIConfig().Model,
		PromptVersion: models.CurrentPromptVersion,
		ContentHash:   hand.ContentHash,
	}

	if err := config.DB.Create(analysis).Error; err != nil {
		return nil, false, err
	}

	// 同步更新手牌上的冗余状态，列表页据此显示角标
	config.DB.Model(&models.ReviewHand{}).Where("id = ?", handID).
		Update("analyze_status", models.AnalyzeStatusPending)

	// 传给后台的必须是副本：HTTP 响应正在读 analysis 序列化返回，
	// 后台同时改它的字段就是数据竞争。副本是值拷贝，两边互不影响
	snapshot := *analysis

	// 用独立的 context，不能用 HTTP 请求的 context ——
	// 请求一返回它就被取消了，goroutine 里的模型调用会被立刻中断
	go s.runAnalysis(&snapshot, hand)

	return analysis, false, nil
}

// runAnalysis 在后台执行分析并落库。
//
// 传入完整的 analysis 对象而不是 id：写回时要用 Save 走字段序列化器，
// 需要有一个带全部字段的模型实例
func (s *ReviewAnalysisService) runAnalysis(analysis *models.ReviewAnalysis, hand *models.ReviewHand) {
	start := time.Now()
	analysisID, handID := analysis.ID, hand.ID

	// 单独兜一层 recover：后台 goroutine 里 panic 会直接带走整个进程
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[分析 %d] panic: %v", analysisID, r)
			s.failAnalysis(analysisID, handID, fmt.Sprintf("内部错误: %v", r), time.Since(start))
		}
	}()

	analysis.Status = models.AnalysisStatusRunning
	config.DB.Model(analysis).Update("status", models.AnalysisStatusRunning)

	timeout := time.Duration(config.AIConfig().TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tags, err := s.reviewService.GetLeakTagModels()
	if err != nil {
		s.failAnalysis(analysisID, handID, "读取标签字典失败: "+err.Error(), time.Since(start))
		return
	}

	// 注入该用户的长期记忆。取不到时 BuildMemoryContext 返回 nil，
	// BuildMemoryBlock 退化成空串，这次就按"没有记忆"跑，不影响主流程
	memory := s.memoryService.BuildMemoryContext(hand.UserID)
	system, user := BuildAnalysisPrompt(hand, tags, memory)

	result, err := s.aiClient.CompleteJSON(ctx, system, user)
	if err != nil {
		s.failAnalysis(analysisID, handID, err.Error(), time.Since(start))
		return
	}

	parsed, parseErr := parseAnalysisResult(result.Content, tags)
	if parseErr != nil {
		// 解析失败时把原始返回存下来，否则事后完全无法排查模型到底吐了什么
		analysis.RawOutput = result.Content
		config.DB.Model(analysis).Update("raw_output", result.Content)
		s.failAnalysis(analysisID, handID, "解析模型输出失败: "+parseErr.Error(), time.Since(start))
		return
	}

	nowMs := time.Since(start).Milliseconds()

	analysis.Status = models.AnalysisStatusDone
	analysis.Result = parsed
	analysis.RawOutput = result.Content
	analysis.InputSnapshot = fmt.Sprintf("=== SYSTEM ===\n%s\n\n=== USER ===\n%s", system, user)
	analysis.TokensIn = result.TokensIn
	analysis.TokensOut = result.TokensOut
	analysis.DurationMs = nowMs
	analysis.ErrorMsg = ""

	// 必须用 Save（结构体形式）而不是 Updates(map)：
	// map 形式的更新不会经过字段序列化器，Result 这个 json 列会被原样扔给
	// 数据库驱动并报 "unsupported type ... a struct"，结果永远写不进去
	if err := config.DB.Save(analysis).Error; err != nil {
		s.failAnalysis(analysisID, handID, "保存分析结果失败: "+err.Error(), time.Since(start))
		return
	}

	// 结果写成功了才把状态同步到 done。
	// 顺序反过来的话，一旦写库失败就会出现"手牌显示已分析、但没有任何结论"的不一致
	config.DB.Model(&models.ReviewHand{}).Where("id = ?", handID).
		Update("analyze_status", models.AnalyzeStatusDone)

	log.Printf("[分析 %d] 完成，耗时 %dms，tokens %d/%d",
		analysisID, nowMs, result.TokensIn, result.TokensOut)

	s.recordMemory(analysis, hand)

	// 模型编出来的新标签进待审队列，等系统管理审批后才会进入所有人共用的字典。
	// 与 recordMemory 同样是"只记日志不返回错误"
	if analysis.Result != nil && len(analysis.Result.SuggestedTags) > 0 {
		s.suggestionSvc.RecordSuggestions(analysis.ID, analysis.Result.SuggestedTags)
	}
}

// recordMemory 把本次分析的产出并入长期记忆。
//
// 全程只记日志不返回错误：分析结果本身已经落库了，记忆是增量收益，
// 不该因为它出问题就把这手牌标成失败、让用户白花一次额度
func (s *ReviewAnalysisService) recordMemory(analysis *models.ReviewAnalysis, hand *models.ReviewHand) {
	if err := s.memoryService.RecordInsights(
		hand.UserID, hand.ID, analysis.ID, analysis.Result,
	); err != nil {
		log.Printf("[记忆] 写入洞察失败 analysis=%d: %v", analysis.ID, err)
		return
	}

	// 统计是纯计算，每次都刷；summary 的重写由 MaybeRewriteSummary 按阈值把关
	if _, err := s.memoryService.RefreshProfile(hand.UserID); err != nil {
		log.Printf("[记忆] 刷新画像失败 user=%d: %v", hand.UserID, err)
		return
	}

	s.memoryService.MaybeRewriteSummary(hand.UserID)
}

func (s *ReviewAnalysisService) failAnalysis(analysisID, handID uint, msg string, elapsed time.Duration) {
	log.Printf("[分析 %d] 失败: %s", analysisID, msg)
	config.DB.Model(&models.ReviewAnalysis{}).Where("id = ?", analysisID).
		Updates(map[string]interface{}{
			"status":      models.AnalysisStatusFailed,
			"error_msg":   shortenMsg(msg, 500),
			"duration_ms": elapsed.Milliseconds(),
		})
	config.DB.Model(&models.ReviewHand{}).Where("id = ?", handID).
		Update("analyze_status", models.AnalyzeStatusFailed)
}

// GetAnalysis 读一条分析，按 user_id 过滤，别人的分析表现为不存在
func (s *ReviewAnalysisService) GetAnalysis(userID, analysisID uint) (*models.ReviewAnalysis, error) {
	var analysis models.ReviewAnalysis
	err := config.DB.Where("id = ? AND user_id = ?", analysisID, userID).First(&analysis).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("分析记录不存在")
		}
		return nil, err
	}
	return &analysis, nil
}

// ListByHand 某手牌的全部分析记录，最新在前
func (s *ReviewAnalysisService) ListByHand(userID, handID uint) ([]models.ReviewAnalysis, error) {
	// 先确认手牌归属，避免用手牌 id 探测别人的数据
	if _, err := s.reviewService.GetHand(userID, handID); err != nil {
		return nil, err
	}

	var list []models.ReviewAnalysis
	err := config.DB.Where("hand_id = ? AND user_id = ?", handID, userID).
		Order("id DESC").Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// parseAnalysisResult 解析模型输出，并做防御性清洗。
func parseAnalysisResult(content string, tags []models.ReviewLeakTag) (*models.AnalysisResult, error) {
	cleaned := stripCodeFence(content)

	var result models.AnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, err
	}

	// 即使提示词明确要求"只能用给定标签"，模型仍可能自造 code。
	// 这里做一次白名单过滤：不认识的 code 从 leaks 里剔除，
	// 但把名字挪到 suggestedTags，信息不丢失，也不会污染后续的漏洞聚合统计
	valid := make(map[string]bool, len(tags))
	for _, t := range tags {
		valid[t.Code] = true
	}

	result.Leaks = sanitizeLeaks(result.Leaks, valid, &result.SuggestedTags)
	result.Strengths = sanitizeStrengths(result.Strengths)

	return &result, nil
}

func sanitizeLeaks(items []models.LeakItem, valid map[string]bool, suggestions *[]models.SuggestedTagItem) []models.LeakItem {
	kept := make([]models.LeakItem, 0, len(items))
	for _, item := range items {
		if !valid[item.TagCode] {
			// 模型编的 code 不进统计，但留个线索给运营决定是否入库
			if item.TagCode != "" {
				*suggestions = append(*suggestions, models.SuggestedTagItem{
					Name:   item.TagCode,
					Reason: fmt.Sprintf("模型使用了字典外的标签，证据：%s", shortenMsg(item.Evidence, 80)),
				})
			}
			continue
		}
		// severity 收敛到 1~3，模型偶尔会给出 0 或 5
		if item.Severity < 1 {
			item.Severity = 1
		}
		if item.Severity > 3 {
			item.Severity = 3
		}
		// 没有证据的漏洞标签没有价值，也无法钻取，直接丢弃
		if strings.TrimSpace(item.Evidence) == "" {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// sanitizeStrengths 清洗优点列表。
// 优点不带标签（见 models.StrengthItem 的说明），所以只需要丢掉空文本
func sanitizeStrengths(items []models.StrengthItem) []models.StrengthItem {
	kept := make([]models.StrengthItem, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		// 限长：模型偶尔会把优点写成一段小作文，卡片会被撑得很难看
		item.Text = shortenMsg(text, 200)
		kept = append(kept, item)
	}
	return kept
}

// stripCodeFence 去掉模型可能加上的 markdown 围栏。
// 已经开了 json_object 模式，但个别情况下模型仍会包一层 ```json
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

func shortenMsg(s string, max int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
