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

// opponentProfileInflightWindow 一条 pending/running 超过它就当作中断残留，放行新任务。
// 与 analysisInflightWindow 同一个理由：进程被杀时后台 goroutine 一并消失，
// 但库里的状态不会自己变
const opponentProfileInflightWindow = 30 * time.Minute

// opponentProfileResult 模型返回的 JSON 形状。
// 与 buildOpponentProfileSystemPrompt 里给出的 Schema 一一对应，改名要两边一起改
type opponentProfileResult struct {
	Profile       string                    `json:"profile"`
	Confidence    string                    `json:"confidence"`
	ProfileReason string                    `json:"profileReason"`
	Tendencies    []models.OpponentTendency `json:"tendencies"`
	Exploits      []models.OpponentExploit  `json:"exploits"`
	Unknowns      []string                  `json:"unknowns"`
	WatchNext     []string                  `json:"watchNext"`
	Summary       string                    `json:"summary"`
}

// OpponentProfileService 对手画像：生成、读取、任务状态
type OpponentProfileService struct {
	aiClient     *AIClient
	aiSettingSvc *AISettingService
	opponentSvc  *OpponentService
	analysisSvc  *ReviewAnalysisService
}

func NewOpponentProfileService() *OpponentProfileService {
	return &OpponentProfileService{
		aiClient:     NewAIClient(),
		aiSettingSvc: &AISettingService{},
		opponentSvc:  &OpponentService{},
		analysisSvc:  NewReviewAnalysisService(),
	}
}

// GetProfile 读某个对手的画像。没有返回 nil（不是错误）——
// 页面在还没生成过时要能正常打开
func (s *OpponentProfileService) GetProfile(userID, opponentID uint) (*models.OpponentProfile, error) {
	var profile models.OpponentProfile
	err := config.DB.Where("user_id = ? AND opponent_id = ?", userID, opponentID).First(&profile).Error
	if err == nil {
		return &profile, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

// StartGeneration 触发一次对手画像生成。
//
// 异步：置 pending、起后台 goroutine，立刻返回 —— 与「分析手牌」「重写总结」
// 同一套。K3 那类始终推理的模型一次要跑几分钟，同步接口必然被前端或网关先掐断。
//
// 第二个返回值表示"没有新建任务"（已有一条在跑），此时返回的是当前画像，
// 前端继续轮询即可
func (s *OpponentProfileService) StartGeneration(userID, opponentID uint) (*models.OpponentProfile, bool, error) {
	opponent, err := s.opponentSvc.GetOpponent(userID, opponentID)
	if err != nil {
		return nil, false, err
	}

	hands, err := s.opponentSvc.LoadHandsForStats(userID, opponentID)
	if err != nil {
		return nil, false, fmt.Errorf("读取对手手牌失败: %w", err)
	}
	if len(hands) == 0 {
		// 一手牌都没有就没什么可画像的。这里**不置 pending**：
		// 置了后台会立刻发现无事可做，白转一圈，前端还得多轮询一次
		return nil, false, errors.New("还没有与这位对手交手的手牌记录，先去复盘几手再回来")
	}

	// 凭据在这里同步解析，而不是留给后台 goroutine 去查库：
	// "没配模型"要变成一句立刻返回的报错，而不是几秒后才发现的一次失败
	settings, err := s.aiSettingSvc.ResolveCallSettings(userID)
	if err != nil {
		return nil, false, err
	}

	if inflight, err := s.profileInflight(userID, opponentID); err != nil {
		return nil, false, err
	} else if inflight {
		current, err := s.GetProfile(userID, opponentID)
		if err != nil {
			return nil, false, err
		}
		return current, true, nil
	}

	// 额度检查放在"有手牌"与"已配模型"之后：前面两条都过不了的话，
	// 用户该看到的是"没有手牌"而不是"额度用完了"
	if status := s.analysisSvc.GetAIStatus(userID); status.Remaining <= 0 {
		return nil, false, fmt.Errorf("今日 AI 调用次数已用完（上限 %d 次），明天再来", status.DailyLimit)
	}

	now := time.Now()
	profile, err := s.upsertPending(userID, opponentID, now)
	if err != nil {
		return nil, false, err
	}

	// 值拷贝传进后台：HTTP 响应正在读这个结构体序列化返回
	handsCopy := hands
	name := opponent.Name
	go s.runGeneration(userID, opponentID, name, *settings, handsCopy)

	return profile, false, nil
}

// upsertPending 把画像行置成 pending（没有就建一条）。
//
// 只更新标量列，不动 tendencies / summary 这些正文：重新生成期间页面要继续
// 显示上一版内容 + 一个"生成中"的角标，把正文清空会让页面闪一下白
func (s *OpponentProfileService) upsertPending(userID, opponentID uint, now time.Time) (*models.OpponentProfile, error) {
	var profile models.OpponentProfile
	err := config.DB.Where("user_id = ? AND opponent_id = ?", userID, opponentID).First(&profile).Error
	isNew := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !isNew {
		return nil, err
	}

	if isNew {
		profile = models.OpponentProfile{
			OpponentID: opponentID,
			UserID:     userID,
		}
	}
	profile.Status = models.SummaryStatusPending
	profile.ErrorMsg = ""
	profile.StartedAt = &now

	if isNew {
		if err := config.DB.Create(&profile).Error; err != nil {
			return nil, fmt.Errorf("创建画像失败: %w", err)
		}
		return &profile, nil
	}
	if err := config.DB.Model(&models.OpponentProfile{}).
		Where("user_id = ? AND opponent_id = ?", userID, opponentID).
		Updates(map[string]interface{}{
			"status":     models.SummaryStatusPending,
			"error_msg":  "",
			"started_at": now,
		}).Error; err != nil {
		return nil, fmt.Errorf("标记画像任务失败: %w", err)
	}
	return &profile, nil
}

// profileInflight 有没有一次生成正在跑。
//
// 判据用 started_at 而不是 updated_at：任何一次写库都会把 updated_at 顶到当下，
// 窗口就永远不过期了（ReviewProfile 那边踩过同一个坑）
func (s *OpponentProfileService) profileInflight(userID, opponentID uint) (bool, error) {
	var count int64
	err := config.DB.Model(&models.OpponentProfile{}).
		Where("user_id = ? AND opponent_id = ? AND status IN ? AND started_at > ?",
			userID, opponentID,
			[]string{models.SummaryStatusPending, models.SummaryStatusRunning},
			time.Now().Add(-opponentProfileInflightWindow)).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// runGeneration 在后台调模型生成画像
func (s *OpponentProfileService) runGeneration(
	userID, opponentID uint,
	opponentName string,
	settings AICallSettings,
	hands []models.ReviewHand,
) {
	// 单独兜一层 recover：后台 goroutine 里 panic 会直接带走整个进程
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[画像] 生成 panic user=%d opponent=%d: %v", userID, opponentID, r)
			s.failProfile(userID, opponentID, fmt.Sprintf("内部错误: %v", r))
		}
	}()

	config.DB.Model(&models.OpponentProfile{}).
		Where("user_id = ? AND opponent_id = ?", userID, opponentID).
		Update("status", models.SummaryStatusRunning)

	stats := ComputeOpponentStats(opponentName, hands, opponentID)

	previous, err := s.GetProfile(userID, opponentID)
	if err != nil {
		s.failProfile(userID, opponentID, "读取上一版画像失败")
		return
	}
	// 首先生成时 GetProfile 返回 nil；upsertPending 刚建的那条正文是空的，
	// 拿它当"上一版"会往提示词里塞一段空壳
	if previous != nil && previous.Summary == "" {
		previous = nil
	}

	system, user := BuildOpponentProfilePrompt(stats, hands, previous)

	// 不能用 HTTP 请求的 context：请求早已返回，ctx 一返回就被取消
	completion, err := s.aiClient.CompleteJSON(context.Background(), settings, system, user)
	if err != nil {
		s.failProfile(userID, opponentID, err.Error())
		return
	}

	result, err := parseOpponentProfileResult(completion.Content, stats)
	if err != nil {
		s.failProfile(userID, opponentID, err.Error())
		return
	}

	now := time.Now()
	// 用 Select + 结构体落库，**不要用 map**：map 里的 Go 切片不会走 serializer:json，
	// 而是被 MySQL 驱动当成"多列的值"展开，报 Error 1241 Operand should contain 1 column(s)。
	// tendencies / exploits / unknowns / watchNext 四个都是切片，全踩这一条。
	// （这不是推测出来的 —— 端到端跑第一次时就是这么失败的）
	//
	// 用 Select 而不是 Omit 限定列：将来给这张表加字段时，新字段默认不会被这里误写，
	// 而 Omit 会静默漏掉它。这个取舍与 review_memory.go 的 refreshProfile 一致。
	//
	// 同理也**不能**把整个 profile Save 回去：那个结构体是模型调用之前读的，
	// Save 会顺手写回它的 created_at 等旧值
	update := models.OpponentProfile{
		Profile:       result.Profile,
		Confidence:    result.Confidence,
		ProfileReason: result.ProfileReason,
		Tendencies:    result.Tendencies,
		Exploits:      result.Exploits,
		Unknowns:      result.Unknowns,
		WatchNext:     result.WatchNext,
		Summary:       result.Summary,
		// 记 len(hands) 而不是 stats.Hands：接口返回的 currentHands 也是
		// len(hands)，两者必须同源才能比出"画像比记录旧了"。
		// 用 stats.Hands 的话，极少数认不出人的脏数据会造成永远显示"旧了"
		HandsAtGeneration: len(hands),
		Status:            models.SummaryStatusDone,
		ErrorMsg:          "",
		// nil 会写成 NULL。它必须被清掉，否则在飞闸会一直认为这条还在跑
		StartedAt:       nil,
		LastGeneratedAt: &now,
	}
	if err := config.DB.Model(&models.OpponentProfile{}).
		Where("user_id = ? AND opponent_id = ?", userID, opponentID).
		Select("profile", "confidence", "profile_reason", "tendencies", "exploits",
			"unknowns", "watch_next", "summary", "hands_at_generation",
			"status", "error_msg", "started_at", "last_generated_at").
		Updates(update).Error; err != nil {
		s.failProfile(userID, opponentID, "保存画像失败: "+err.Error())
		return
	}

	log.Printf("[画像] 已生成 user=%d opponent=%d 手数=%d 形象=%s tokens=%d/%d",
		userID, opponentID, stats.Hands, result.Profile, completion.TokensIn, completion.TokensOut)
}

// parseOpponentProfileResult 解析模型输出，并把两条硬性约束**在代码里**落实。
//
// 提示词里已经写了这两条，这里还要再拦一道，是因为它们不是风格问题而是正确性问题：
// 模型偶尔会无视约束，给出一个样本不足却言之凿凿的形象，而用户没有任何办法
// 分辨那是推断还是编造。便宜的那道闸就该设上。
func parseOpponentProfileResult(content string, stats *models.OpponentStats) (*opponentProfileResult, error) {
	raw := stripCodeFence(content)
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("模型返回的内容为空")
	}

	var result opponentProfileResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("模型返回的不是合法 JSON: %w", err)
	}

	// 形象必须是五格之一。对不上就归 unknown，而不是原样存下来 ——
	// 前端按枚举渲染，一个没见过的值会让整块画像显示不出来
	if !isValidOpponentProfile(result.Profile) {
		result.Profile = models.ProfileUnknown
	}
	if !isValidConfidence(result.Confidence) {
		result.Confidence = models.ConfidenceLow
	}
	result.Summary = strings.TrimSpace(result.Summary)
	if result.Summary == "" {
		return nil, errors.New("模型没有给出画像总结")
	}

	// 样本不足时强制降级。这条在提示词的硬性约束第 3 条里写着，
	// 但它是本功能里最容易被绕过、后果又最严重的一条：
	// 2 手牌得出的"他是松凶"会被用户当成事实去执行
	if stats.ThinSample {
		result.Profile = models.ProfileUnknown
		result.Confidence = models.ConfidenceLow
		result.Exploits = []models.OpponentExploit{}
	}

	// 四个数组一律归一到非 nil。落库后 nil 会变成 JSON 的 null，而前端是拿
	// .length 与 .map 直接用的，一个 null 就会让整块画像白屏 ——
	// 这与 dto.ToReviewHandResponse 里"返回 nil 会让前端多写一层判空"是同一条约定
	if result.Tendencies == nil {
		result.Tendencies = []models.OpponentTendency{}
	}
	if result.Exploits == nil {
		result.Exploits = []models.OpponentExploit{}
	}
	if result.Unknowns == nil {
		result.Unknowns = []string{}
	}
	if result.WatchNext == nil {
		result.WatchNext = []string{}
	}

	return &result, nil
}

func isValidOpponentProfile(profile string) bool {
	switch profile {
	case models.ProfileLoosePassive, models.ProfileTightPassive,
		models.ProfileLooseAggressive, models.ProfileTightAggressive,
		models.ProfileUnknown:
		return true
	}
	return false
}

func isValidConfidence(confidence string) bool {
	switch confidence {
	case models.ConfidenceLow, models.ConfidenceMedium, models.ConfidenceHigh:
		return true
	}
	return false
}

// failProfile 把任务标成失败。msg 会被截到列宽
func (s *OpponentProfileService) failProfile(userID, opponentID uint, msg string) {
	log.Printf("[画像] 生成失败 user=%d opponent=%d: %s", userID, opponentID, msg)
	config.DB.Model(&models.OpponentProfile{}).
		Where("user_id = ? AND opponent_id = ?", userID, opponentID).
		Updates(map[string]interface{}{
			"status":    models.SummaryStatusFailed,
			"error_msg": shortenMsg(msg, 500),
		})
}
