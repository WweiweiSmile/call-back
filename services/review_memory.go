package services

import (
	"call-go/config"
	"call-go/models"
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 画像相关阈值与容量上限
const (
	// SummaryRewriteMinNewInsights 距上次重写新增这么多条洞察就触发重写
	SummaryRewriteMinNewInsights = 5
	// SummaryRewriteMaxAgeDays 距上次重写超过这么多天、且至少有一条新增，也触发重写
	SummaryRewriteMaxAgeDays = 7
	// ProfileTopEvidenceCount 每个漏洞在画像里保留几条证据
	ProfileTopEvidenceCount = 3
	// ProfileStrengthCount 画像里保留几条最近的优点
	ProfileStrengthCount = 5
	// ProfileTopLeaksInPrompt 注入提示词的高频漏洞条数上限
	ProfileTopLeaksInPrompt = 5
	// SummaryMaxRunes 总结字数上限（设计文档定为 500 字）
	SummaryMaxRunes = 500
	// memoryRecentThoughtCount 记忆块里带几条玩家自己的近期原话
	memoryRecentThoughtCount = 3
)

// ReviewMemoryService 长期记忆：洞察落库、画像聚合、总结重写
type ReviewMemoryService struct {
	aiClient *AIClient
}

func NewReviewMemoryService() *ReviewMemoryService {
	return &ReviewMemoryService{aiClient: NewAIClient()}
}

// RecordInsights 把一次分析产出的 leaks / strengths 落成洞察行。
//
// 一手牌只保留一套洞察，来自"对它当前内容的那次分析"：先按 hand_id 清掉旧的，
// 再写入本次的。
//
// 为什么不能按 analysis_id 清：手牌改过之后重新分析会生成新的 analysis 行，
// 旧行名下的洞察原样留着，画像聚合时同一手牌就被统计两次 ——
// 用户填错了牌、改完重新分析，那手错误手牌仍然在画像里各计一份。
// （历史分析记录本身不删，完整保留在 review_analyses 里，只是不再参与记忆。）
func (s *ReviewMemoryService) RecordInsights(
	userID, handID, analysisID uint,
	result *models.AnalysisResult,
) error {
	if result == nil {
		return nil
	}

	insights := buildInsights(userID, handID, analysisID, result)

	// 清空与写入放在一个事务里：中途失败不该留下"旧的已删、新的没写"的空窗
	return config.DB.Transaction(func(tx *gorm.DB) error {
		if err := deleteInsightsForHand(tx, userID, handID); err != nil {
			return fmt.Errorf("清理旧洞察失败: %w", err)
		}

		// 本次一条洞察都没产出时也要清。手牌改对了、不再有漏洞，旧的那套必须跟着消失，
		// 否则画像会永远记着一个已经被改掉的问题
		if len(insights) == 0 {
			return nil
		}
		if err := tx.Create(&insights).Error; err != nil {
			return fmt.Errorf("写入洞察失败: %w", err)
		}
		return nil
	})
}

// buildInsights 把分析结果展开成待落库的洞察行。
// 抽成纯函数便于单测——哪些条目该丢、哪些该收敛，是这里最容易出错的判断
func buildInsights(
	userID, handID, analysisID uint,
	result *models.AnalysisResult,
) []models.ReviewInsight {
	insights := make([]models.ReviewInsight, 0, len(result.Leaks)+len(result.Strengths))

	for _, leak := range result.Leaks {
		tagCode := strings.TrimSpace(leak.TagCode)
		evidence := strings.TrimSpace(leak.Evidence)
		// evidence 是"点击漏洞钻取到具体手牌"的唯一抓手，空的直接丢掉。
		// 宁可少一条记录，也不要一条查无实据的标签
		if tagCode == "" || evidence == "" {
			continue
		}
		severity := leak.Severity
		if severity < 1 {
			severity = 1
		}
		if severity > 3 {
			severity = 3
		}
		insights = append(insights, models.ReviewInsight{
			UserID:     userID,
			HandID:     handID,
			AnalysisID: analysisID,
			Kind:       models.InsightKindLeak,
			TagCode:    tagCode,
			Severity:   severity,
			Evidence:   evidence,
		})
	}

	for _, strength := range result.Strengths {
		text := strings.TrimSpace(strength.Text)
		if text == "" {
			continue
		}
		// 优点不带标签：标签字典是"漏洞"字典，拿它说做对的地方会自相矛盾
		insights = append(insights, models.ReviewInsight{
			UserID:     userID,
			HandID:     handID,
			AnalysisID: analysisID,
			Kind:       models.InsightKindStrength,
			Severity:   0,
			Evidence:   text,
		})
	}

	return insights
}

// deleteInsightsForHand 清掉一手牌的全部洞察（内容变了，旧结论不再对应当前手牌）。
//
// 编辑手牌与写入洞察都走这里，让"一手牌只有一套洞察"这个不变量只有一处实现
func deleteInsightsForHand(tx *gorm.DB, userID, handID uint) error {
	return tx.Where("hand_id = ? AND user_id = ?", handID, userID).
		Delete(&models.ReviewInsight{}).Error
}

// CountInsightsAfter 统计水位线之后新增的洞察条数
func (s *ReviewMemoryService) CountInsightsAfter(userID, afterID uint) (int, error) {
	var count int64
	err := config.DB.Model(&models.ReviewInsight{}).
		Where("user_id = ? AND id > ?", userID, afterID).
		Count(&count).Error
	return int(count), err
}

// ShouldRewriteSummary 判断是否该重写画像总结。
//
// 为什么不是每次都重写：一是成本（每次分析多一次模型调用），
// 二是稳定性 —— 每次都重写会让总结随最新一手牌剧烈摆动，
// 用户看到"我的总结怎么天天变"，反而不再信任它。
// 批量增量更新让总结反映的是趋势，而不是最后一手牌。
func ShouldRewriteSummary(profile *models.ReviewProfile, newInsightCount int) bool {
	if newInsightCount <= 0 {
		return false
	}
	// 还没有过总结：第一手牌就先把画像建立起来，否则画像页长期是空的
	if profile.LastSummaryAt == nil || strings.TrimSpace(profile.Summary) == "" {
		return true
	}
	if newInsightCount >= SummaryRewriteMinNewInsights {
		return true
	}
	return time.Since(*profile.LastSummaryAt).Hours() >= SummaryRewriteMaxAgeDays*24
}

// RefreshProfile 重新聚合画像统计并落库（不碰 summary）。
//
// 统计部分是纯计算，不调用模型，所以每次分析完都可以刷新；
// 只有 summary 的生成才需要模型，那个由 MaybeRewriteSummary 把关。
func (s *ReviewMemoryService) RefreshProfile(userID uint) (*models.ReviewProfile, error) {
	profile, err := s.GetOrCreateProfile(userID)
	if err != nil {
		return nil, err
	}

	// 只统计手牌还在的洞察。删掉的手牌不该继续在画像里计数 ——
	// 否则画像说"出现 4 次"、点进去却只列得出 3 条，用户无从判断哪个是真的。
	// 子查询里的 Model(&ReviewHand{}) 自带软删除过滤，不用再写 deleted_at IS NULL
	var insights []models.ReviewInsight
	if err := config.DB.Where("user_id = ?", userID).
		Where("hand_id IN (?)",
			config.DB.Model(&models.ReviewHand{}).Select("id").Where("user_id = ?", userID)).
		Order("id ASC").Find(&insights).Error; err != nil {
		return nil, err
	}

	leaks, strengths := AggregateInsights(insights, s.leakTagNames())

	var handsReviewed int64
	if err := config.DB.Model(&models.ReviewHand{}).
		Where("user_id = ? AND analyze_status = ?", userID, models.AnalyzeStatusDone).
		Count(&handsReviewed).Error; err != nil {
		return nil, err
	}

	profile.Leaks = leaks
	profile.Strengths = strengths
	profile.HandsReviewed = int(handsReviewed)

	if err := config.DB.Save(profile).Error; err != nil {
		return nil, fmt.Errorf("保存画像失败: %w", err)
	}
	return profile, nil
}

// AggregateInsights 把洞察原子聚合成画像里的漏洞排行与优点列表。
//
// 抽成不依赖数据库的纯函数，是因为这是整个长期记忆里最容易出错的一环
// （分组、计数、排序、截断、标签改名），必须能脱离数据库单独测。
//
// tagNames 是 tag_code → 中文名的映射，取不到名字时退化成用 code 展示，
// 不能因为标签被停用就丢掉这段统计。
func AggregateInsights(
	insights []models.ReviewInsight,
	tagNames map[string]string,
) ([]models.ProfileLeakStat, []models.ProfileStrengthItem) {
	// 用 map 累计、用 slice 记住首次出现顺序，保证同样的输入每次聚合出的顺序一致
	// （画像页不该每次刷新都换个排法）
	type leakAcc struct {
		count    int
		sevSum   int
		lastSeen time.Time
		evidence []string
	}
	leakMap := make(map[string]*leakAcc)
	leakOrder := make([]string, 0, 8)
	strengths := make([]models.ProfileStrengthItem, 0, ProfileStrengthCount)

	for _, in := range insights {
		switch in.Kind {
		case models.InsightKindLeak:
			if in.TagCode == "" {
				continue
			}
			acc := leakMap[in.TagCode]
			if acc == nil {
				acc = &leakAcc{}
				leakMap[in.TagCode] = acc
				leakOrder = append(leakOrder, in.TagCode)
			}
			acc.count++
			acc.sevSum += in.Severity
			if in.CreatedAt.After(acc.lastSeen) {
				acc.lastSeen = in.CreatedAt
			}
			acc.evidence = append(acc.evidence, in.Evidence)
			// 只留最近几条：更早的证据用户翻不到，留着只是占空间
			if len(acc.evidence) > ProfileTopEvidenceCount {
				acc.evidence = acc.evidence[len(acc.evidence)-ProfileTopEvidenceCount:]
			}
		case models.InsightKindStrength:
			strengths = append(strengths, models.ProfileStrengthItem{
				Text:   in.Evidence,
				HandID: in.HandID,
				Date:   in.CreatedAt.Format("2006-01-02"),
			})
		}
	}

	leaks := make([]models.ProfileLeakStat, 0, len(leakOrder))
	for _, code := range leakOrder {
		acc := leakMap[code]
		name := tagNames[code]
		if name == "" {
			name = code
		}
		leaks = append(leaks, models.ProfileLeakStat{
			TagCode:     code,
			Name:        name,
			Count:       acc.count,
			LastSeenAt:  acc.lastSeen.Format("2006-01-02"),
			AvgSeverity: float64(acc.sevSum) / float64(acc.count),
			TopEvidence: acc.evidence,
		})
	}

	// 出现次数多的排前面；次数相同按最近出现时间排，让"最近老犯"的浮上来
	sort.SliceStable(leaks, func(i, j int) bool {
		if leaks[i].Count != leaks[j].Count {
			return leaks[i].Count > leaks[j].Count
		}
		return leaks[i].LastSeenAt > leaks[j].LastSeenAt
	})

	// 优点只留最近的若干条，按时间倒序
	if len(strengths) > ProfileStrengthCount {
		strengths = strengths[len(strengths)-ProfileStrengthCount:]
	}
	for i, j := 0, len(strengths)-1; i < j; i, j = i+1, j-1 {
		strengths[i], strengths[j] = strengths[j], strengths[i]
	}

	return leaks, strengths
}

// MaybeRewriteSummary 按阈值决定是否重写总结。
//
// 注意它是**同步**的：命中阈值会在这里真调一次模型（十几秒）。
// 现有唯一的调用方 recordMemory 跑在分析的后台 goroutine 里，所以不阻塞接口；
// 但从 HTTP 请求路径上调它会把接口卡住。
//
// 重写失败只记日志：分析任务已经跑完并落库了，总结是锦上添花，
// 不该影响这手牌的分析结果，也不该让接口报错。
func (s *ReviewMemoryService) MaybeRewriteSummary(userID uint) {
	profile, err := s.GetOrCreateProfile(userID)
	if err != nil {
		log.Printf("[记忆] 读取画像失败 user=%d: %v", userID, err)
		return
	}

	newCount, err := s.CountInsightsAfter(userID, profile.LastSummaryInsightID)
	if err != nil {
		log.Printf("[记忆] 统计新增洞察失败 user=%d: %v", userID, err)
		return
	}

	if !ShouldRewriteSummary(profile, newCount) {
		return
	}

	if _, err := s.RewriteSummary(context.Background(), userID); err != nil {
		// 总结重写失败不影响分析结果，记日志即可，下次分析会再触发
		log.Printf("[记忆] 重写总结失败 user=%d: %v", userID, err)
	}
}

// RewriteSummary 调模型增量重写画像总结。
//
// 传的是「已有总结 + 新增洞察 + 标签统计」，让模型做增量更新而不是从零重写：
// 从零重写会让它把注意力全放在最新几手牌上，总结随最新一手牌剧烈摆动。
func (s *ReviewMemoryService) RewriteSummary(ctx context.Context, userID uint) (*models.ReviewProfile, error) {
	profile, err := s.GetOrCreateProfile(userID)
	if err != nil {
		return nil, err
	}

	// 先刷统计，保证喂给模型的计数和画像页看到的一致
	profile, err = s.RefreshProfile(userID)
	if err != nil {
		return nil, err
	}

	if len(profile.Leaks) == 0 && len(profile.Strengths) == 0 {
		// 一条洞察都没有，没什么可总结的。不动 summary，避免产出空洞的套话
		return profile, nil
	}

	var newInsights []models.ReviewInsight
	if err := config.DB.Where("user_id = ? AND id > ?", userID, profile.LastSummaryInsightID).
		Order("id ASC").Find(&newInsights).Error; err != nil {
		return nil, err
	}

	system, user := BuildProfileSummaryPrompt(profile, newInsights)

	// 500 字中文留 800 token 足够。给太多会让模型忍不住写长
	completion, err := s.aiClient.Complete(ctx, system, user, 800)
	if err != nil {
		return nil, err
	}

	summary := truncateRunes(strings.TrimSpace(completion.Content), SummaryMaxRunes)
	if summary == "" {
		return nil, fmt.Errorf("模型返回的总结为空")
	}

	var maxInsightID uint
	if err := config.DB.Model(&models.ReviewInsight{}).
		Where("user_id = ?", userID).
		Select("COALESCE(MAX(id), 0)").Scan(&maxInsightID).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	profile.Summary = summary
	profile.SummaryVersion++
	profile.LastSummaryAt = &now
	profile.LastSummaryInsightID = maxInsightID

	if err := config.DB.Save(profile).Error; err != nil {
		return nil, fmt.Errorf("保存总结失败: %w", err)
	}

	log.Printf("[记忆] 画像总结已重写 user=%d 版本=%d 新增洞察=%d tokens=%d/%d",
		userID, profile.SummaryVersion, len(newInsights),
		completion.TokensIn, completion.TokensOut)

	return profile, nil
}

// GetOrCreateProfile 读画像，没有就建一条空的。
//
// 画像页在用户还没分析过任何手牌时也要能打开，不该因为没记录就报错。
func (s *ReviewMemoryService) GetOrCreateProfile(userID uint) (*models.ReviewProfile, error) {
	var profile models.ReviewProfile
	err := config.DB.Where("user_id = ?", userID).First(&profile).Error
	if err == nil {
		return &profile, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	profile = models.ReviewProfile{
		UserID:         userID,
		Leaks:          []models.ProfileLeakStat{},
		Strengths:      []models.ProfileStrengthItem{},
		Summary:        "",
		SummaryVersion: 0,
	}
	if err := config.DB.Create(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// BuildMemoryContext 把画像转成提示词用的记忆块。
//
// 返回 nil 表示没有可注入的记忆（用户还没积累），
// 此时 BuildMemoryBlock 会返回空串，提示词结构与 M3 保持一致。
func (s *ReviewMemoryService) BuildMemoryContext(userID uint) *MemoryContext {
	profile, err := s.GetOrCreateProfile(userID)
	if err != nil {
		// 记忆取不到不该让分析失败，退化成"没有记忆"跑一次完整分析
		log.Printf("[记忆] 构建记忆块失败 user=%d: %v", userID, err)
		return nil
	}

	if profile.HandsReviewed == 0 && len(profile.Leaks) == 0 && profile.Summary == "" {
		return nil
	}

	memory := &MemoryContext{
		Summary:       profile.Summary,
		HandsReviewed: profile.HandsReviewed,
	}

	for i, leak := range profile.Leaks {
		if i >= ProfileTopLeaksInPrompt {
			break
		}
		evidence := ""
		if len(leak.TopEvidence) > 0 {
			evidence = leak.TopEvidence[len(leak.TopEvidence)-1]
		}
		memory.TopLeaks = append(memory.TopLeaks, MemoryLeak{
			Name:       leak.Name,
			Count:      leak.Count,
			LastSeenAt: leak.LastSeenAt,
			Evidence:   evidence,
		})
	}

	// 玩家自己的原话：让模型能对上"你上次也是这么想的"，
	// 比只给一串标签更能点醒人
	var recentHands []models.ReviewHand
	if err := config.DB.Model(&models.ReviewHand{}).
		Where("user_id = ? AND hero_thought <> ''", userID).
		Order("id DESC").Limit(memoryRecentThoughtCount).Find(&recentHands).Error; err == nil {
		for _, h := range recentHands {
			thought := strings.TrimSpace(h.HeroThought)
			if thought == "" {
				continue
			}
			memory.RecentEvidences = append(memory.RecentEvidences,
				truncateRunes(thought, 120))
		}
	}

	return memory
}

// InsightWithHand 一条洞察带上它所属手牌的展示信息，供画像页钻取
type InsightWithHand struct {
	InsightID uint      `json:"insightId"`
	HandID    uint      `json:"handId"`
	HandTitle string    `json:"handTitle"`
	Position  string    `json:"position"`
	TableSize int       `json:"tableSize"`
	HeroCards string    `json:"heroCards"`
	Severity  int       `json:"severity"`
	Evidence  string    `json:"evidence"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListInsightsByTag 某个漏洞的全部历史证据。
//
// 这是画像页"点击漏洞 → 看历史上哪几手牌犯的"的落点：
// 没有它，画像就只是一句无据可查的结论。
func (s *ReviewMemoryService) ListInsightsByTag(userID uint, tagCode string, limit int) ([]InsightWithHand, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var insights []models.ReviewInsight
	query := config.DB.Where("user_id = ? AND kind = ?", userID, models.InsightKindLeak)
	if tagCode != "" {
		query = query.Where("tag_code = ?", tagCode)
	}
	if err := query.Order("id DESC").Limit(limit).Find(&insights).Error; err != nil {
		return nil, err
	}
	if len(insights) == 0 {
		return []InsightWithHand{}, nil
	}

	// 批量取手牌，避免逐条查库
	handIDs := make([]uint, 0, len(insights))
	for _, in := range insights {
		handIDs = append(handIDs, in.HandID)
	}
	var hands []models.ReviewHand
	if err := config.DB.Where("id IN ? AND user_id = ?", handIDs, userID).
		Find(&hands).Error; err != nil {
		return nil, err
	}
	handMap := make(map[uint]models.ReviewHand, len(hands))
	for _, h := range hands {
		handMap[h.ID] = h
	}

	result := make([]InsightWithHand, 0, len(insights))
	for _, in := range insights {
		hand, ok := handMap[in.HandID]
		if !ok {
			// 手牌已删。RefreshProfile 也不把它的洞察计入排行，
			// 这里跟着跳过，保证"出现 N 次"与点进去的实际条数一致
			continue
		}
		result = append(result, InsightWithHand{
			InsightID: in.ID,
			HandID:    in.HandID,
			HandTitle: hand.Title,
			Position:  hand.HeroPosition,
			TableSize: hand.TableSize,
			HeroCards: hand.HeroCards,
			Severity:  in.Severity,
			Evidence:  in.Evidence,
			CreatedAt: in.CreatedAt,
		})
	}
	return result, nil
}

// leakTagNames 取 code → 中文名 的映射
func (s *ReviewMemoryService) leakTagNames() map[string]string {
	var tags []models.ReviewLeakTag
	if err := config.DB.Find(&tags).Error; err != nil {
		// 取不到名字不该让统计失败，调用方会退化成用 code 展示
		log.Printf("[记忆] 读取标签字典失败: %v", err)
		return map[string]string{}
	}
	names := make(map[string]string, len(tags))
	for _, t := range tags {
		names[t.Code] = t.Name
	}
	return names
}

// truncateRunes 按字符（不是字节）截断，避免把中文截成半个字
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
