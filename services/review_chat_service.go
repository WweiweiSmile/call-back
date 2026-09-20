package services

import (
	"call-go/config"
	"call-go/models"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// ChatMessageMaxRunes 单条追问的字数上限，与 HeroThought 同一口径。
	// 用户输入会进提示词，限长既是成本控制也是注入面的收敛
	ChatMessageMaxRunes = 1000
	// ChatHistoryLimit 拼进提示词的历史条数上限（**条数**，20 条 = 10 轮），
	// 防止上下文随对话无限增长
	ChatHistoryLimit = 20

	// chatInflightWindow 「这条追问还算在跑」的时间窗。
	//
	// 它不是超时 —— AI 调用本身不限时。这里只把"真的在跑"和"重启或卡死留下的
	// 孤儿行"区分开，所以取得比任何一次真实调用都长。调小会让慢调用被误判成孤儿，
	// 调大只会让残留多挡一会儿
	chatInflightWindow = 15 * time.Minute

	// chatInterruptedMsg 判定为中断后的 error_msg
	chatInterruptedMsg = "任务中断，请重新提问"
)

// ReviewChatService 追问对话：基于已有的分析结论回答学员的问题
type ReviewChatService struct {
	reviewService *ReviewService
	memoryService *ReviewMemoryService
	aiClient      *AIClient
	aiSettingSvc  *AISettingService
}

func NewReviewChatService() *ReviewChatService {
	return &ReviewChatService{
		reviewService: NewReviewService(),
		memoryService: NewReviewMemoryService(),
		aiClient:      NewAIClient(),
		aiSettingSvc:  &AISettingService{},
	}
}

// Ask 学员追问。
//
// 异步：这里只做校验与落库，立刻返回一问一答两条记录，真正的模型调用在后台 ——
// 与「分析手牌」同一套。K3 这类「始终推理」模型一次追问要跑几分钟，同步接口
// 必然被前端或网关先掐断。
//
// 返回的 assistant 记录 status=pending、content 为空，前端据 status 轮询。
// inflight=true 表示没有新建，返回的是这手牌正在跑的那对（重复提交被挡住了）
func (s *ReviewChatService) Ask(
	userID, handID uint,
	question string,
) (questionMsg, answerMsg *models.ReviewMessage, inflight bool, err error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, nil, false, fmt.Errorf("追问内容不能为空")
	}
	if len([]rune(question)) > ChatMessageMaxRunes {
		return nil, nil, false, fmt.Errorf("追问不能超过 %d 字", ChatMessageMaxRunes)
	}

	// GetHand 自带归属校验，拿着别人的 hand_id 只会得到"不存在"
	hand, err := s.reviewService.GetHand(userID, handID)
	if err != nil {
		return nil, nil, false, err
	}

	// 没有分析结论就不能追问：追问是"基于结论的提问"，没结论可问。
	// 这一次的 analysis_id 会写进下面两条消息，后台按 id 钉住它
	analysis, err := s.latestDoneAnalysis(userID, handID)
	if err != nil {
		return nil, nil, false, err
	}

	// 这手牌已有追问在跑：把那一对原样还回去，不要再起一次模型调用。
	// 追问不计额度，但一次 K3 追问是几分钟的和真金白银，重复提交不该并行烧两份
	if pair, found, err := s.inflightPair(userID, handID); err != nil {
		return nil, nil, false, err
	} else if found {
		return pair[0], pair[1], true, nil
	}

	// 凭据解析排在在飞闸之后：命中闸时不该因为"模型配置坏了"就不给用户看
	// 那条已经在跑的记录 —— 它跟配置没关系
	settings, err := s.aiSettingSvc.ResolveCallSettings(userID)
	if err != nil {
		return nil, nil, false, err
	}

	userMsg := &models.ReviewMessage{
		UserID:     userID,
		HandID:     handID,
		AnalysisID: analysis.ID,
		Role:       models.MessageRoleUser,
		Content:    question,
		// 显式赋值，不依赖 default tag：GORM 不回读数据库填的默认值，
		// 留空的话前端会把它当成非终态，输入框直接禁掉
		Status: models.MessageStatusDone,
	}
	assistantMsg := &models.ReviewMessage{
		UserID:     userID,
		HandID:     handID,
		AnalysisID: analysis.ID,
		Role:       models.MessageRoleAssistant,
		Content:    "",
		Status:     models.MessageStatusPending,
	}

	// 两条一起写：占位行与问题必须同时存在，否则前端会看到一轮没有提问的回答
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(userMsg).Error; err != nil {
			return err
		}
		return tx.Create(assistantMsg).Error
	}); err != nil {
		return nil, nil, false, fmt.Errorf("保存对话失败: %w", err)
	}

	// 值拷贝传进后台：HTTP 响应正在读这两条序列化返回，后台同时改它们就是数据竞争
	go s.answer(*userMsg, *assistantMsg, hand, *settings)

	return userMsg, assistantMsg, false, nil
}

// answer 在后台调模型并写回占位行。
//
// 传入消息副本而不是 id：写回时要用 Updates 定位自己那一行，而 AnalysisID 等
// 上下文靠副本带过来，避免再查一次库
func (s *ReviewChatService) answer(
	userMsg models.ReviewMessage,
	assistantMsg models.ReviewMessage,
	hand *models.ReviewHand,
	settings AICallSettings,
) {
	assistantID := assistantMsg.ID

	// 单独兜一层 recover：后台 goroutine 里 panic 会直接带走整个进程
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[追问 %d] panic: %v", assistantID, r)
			s.failMessage(assistantID, fmt.Sprintf("内部错误: %v", r))
		}
	}()

	config.DB.Model(&models.ReviewMessage{}).Where("id = ?", assistantID).
		Update("status", models.MessageStatusRunning)

	// 那次分析按 id 重读，不能拿 latestDoneAnalysis 现取：这几分钟里用户可能已经
	// 重跑分析并 done 了，那样这条回复就是"拿新结论答旧上下文"，
	// 而且与它自己记的 analysis_id 对不上
	var analysis models.ReviewAnalysis
	if err := config.DB.Where("id = ? AND user_id = ?", assistantMsg.AnalysisID, userMsg.UserID).
		First(&analysis).Error; err != nil {
		s.failMessage(assistantID, "分析结论已不存在，请重新分析后再追问")
		return
	}

	history, err := s.recentHistory(userMsg.UserID, userMsg.HandID)
	if err != nil {
		s.failMessage(assistantID, "读取对话历史失败")
		return
	}
	// 本轮问题作为最后一条拼进提示词。
	//
	// 它此刻已经落库，但 recentHistory 的配对规则会把"后面还没跟 done 回答的
	// user 行"过滤掉（本轮占位行正是 pending），所以这里显式补上不会重复
	history = append(history, models.ReviewMessage{
		Role:    models.MessageRoleUser,
		Content: userMsg.Content,
	})

	memory := s.memoryService.BuildMemoryContext(userMsg.UserID)
	system, userPrompt := BuildChatPrompt(hand, &analysis, memory, history)

	// 不能用 HTTP 请求的 context：请求早已返回，ctx 一返回就被取消，
	// 这里的模型调用会被立刻中断
	completion, err := s.aiClient.Complete(context.Background(), settings, system, userPrompt)
	if err != nil {
		s.failMessage(assistantID, err.Error())
		return
	}

	// 只更新这条自己的列，不重新创建：占位行的 id 恒小于后续消息，
	// 这是前端排序与 pairHistory 配对规则的地基
	if err := config.DB.Model(&models.ReviewMessage{}).Where("id = ?", assistantID).
		Updates(map[string]interface{}{
			"content":    completion.Content,
			"tokens_in":  completion.TokensIn,
			"tokens_out": completion.TokensOut,
			"status":     models.MessageStatusDone,
			"error_msg":  "",
		}).Error; err != nil {
		log.Printf("[追问 %d] 写回失败: %v", assistantID, err)
	}
}

// failMessage 把占位行标成失败。msg 会被截到 error_msg 的列宽
func (s *ReviewChatService) failMessage(msgID uint, msg string) {
	log.Printf("[追问 %d] 失败: %s", msgID, msg)
	config.DB.Model(&models.ReviewMessage{}).Where("id = ?", msgID).
		Updates(map[string]interface{}{
			"status":    models.MessageStatusFailed,
			"error_msg": shortenMsg(msg, 500),
			"content":   "",
		})
}

// ListMessages 读某手牌的完整对话，按时间升序。
// 同样先校验手牌归属，避免用别人的 hand_id 探到对话内容
func (s *ReviewChatService) ListMessages(userID, handID uint) ([]models.ReviewMessage, error) {
	if _, err := s.reviewService.GetHand(userID, handID); err != nil {
		return nil, err
	}

	s.reapStuckMessages(userID, handID)

	var messages []models.ReviewMessage
	if err := config.DB.Where("user_id = ? AND hand_id = ?", userID, handID).
		Order("id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	if messages == nil {
		messages = []models.ReviewMessage{}
	}
	return messages, nil
}

// reapStuckMessages 把超时仍未结束的追问判死。
//
// 读接口顺手写状态看起来越界，但语义上等同于刷新一次物化视图（同 GetProfile
// 的先例）：这些行的后台 goroutine 已经随重启消失或卡死了，不在这里收掉，
// 前端会因为"存在非终态消息"而永远禁用输入框 —— 用户既拿不到结果，也没法重问
func (s *ReviewChatService) reapStuckMessages(userID, handID uint) {
	if err := config.DB.Model(&models.ReviewMessage{}).
		Where("user_id = ? AND hand_id = ? AND role = ? AND status IN ? AND created_at < ?",
			userID, handID, models.MessageRoleAssistant,
			[]string{models.MessageStatusPending, models.MessageStatusRunning},
			time.Now().Add(-chatInflightWindow)).
		Updates(map[string]interface{}{
			"status":    models.MessageStatusFailed,
			"error_msg": chatInterruptedMsg,
		}).Error; err != nil {
		log.Printf("[追问] 回收中断任务失败 hand=%d: %v", handID, err)
	}
}

// inflightPair 该手牌正在跑的那一对消息，第二个返回值为 false 表示没有在跑的追问。
//
// 认 pending/running 但**加时间窗**：分析跑到一半重启服务，后台 goroutine 随进程
// 一起没了，那条记录会永远停在 running。无条件认它，这手牌就再也问不了了
func (s *ReviewChatService) inflightPair(userID, handID uint) ([]*models.ReviewMessage, bool, error) {
	var assistant models.ReviewMessage
	err := config.DB.Where(
		"user_id = ? AND hand_id = ? AND role = ? AND status IN ? AND created_at > ?",
		userID, handID, models.MessageRoleAssistant,
		[]string{models.MessageStatusPending, models.MessageStatusRunning},
		time.Now().Add(-chatInflightWindow),
	).Order("id DESC").First(&assistant).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	// 配对的问题行：占位行的 id 一定大于它对应的那一问
	var question models.ReviewMessage
	if err := config.DB.Where("user_id = ? AND hand_id = ? AND role = ? AND id < ?",
		userID, handID, models.MessageRoleUser, assistant.ID).
		Order("id DESC").First(&question).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		// 问题行读不到也照样把占位行还回去，否则前端会以为没提交成功而反复重试
		question = models.ReviewMessage{
			UserID:     userID,
			HandID:     handID,
			AnalysisID: assistant.AnalysisID,
			Role:       models.MessageRoleUser,
		}
	}

	return []*models.ReviewMessage{&question, &assistant}, true, nil
}

// recentHistory 取最近若干条**已完成**的对话，按时间升序返回。
//
// 先滤掉非 done 与空内容的行：它们要么是还没答完的占位行，要么是失败掉的那一问。
// Limit 取 ChatHistoryLimit 的两倍，因为 LIMIT 在配对**之前**生效 ——
// 截断边界上可能凑不成对（最老的那条是 assistant，它的问题被切在窗外），
// 配完再截回 ChatHistoryLimit。ChatHistoryLimit 是偶数，从尾部截不会切散成对的消息
func (s *ReviewChatService) recentHistory(userID, handID uint) ([]models.ReviewMessage, error) {
	var messages []models.ReviewMessage
	if err := config.DB.Where(
		"user_id = ? AND hand_id = ? AND status = ? AND content <> ''",
		userID, handID, models.MessageStatusDone,
	).Order("id DESC").Limit(ChatHistoryLimit * 2).Find(&messages).Error; err != nil {
		return nil, err
	}

	// 倒序取"最近 N 条"之后再翻回时间顺序，否则模型看到的对话是倒着的
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	paired := pairHistory(messages)
	if len(paired) > ChatHistoryLimit {
		paired = paired[len(paired)-ChatHistoryLimit:]
	}
	return paired, nil
}

// pairHistory 从消息流里挑出成对的一问一答。
//
// 只保留「一条 user 后面紧跟一条 assistant」的组合：孤立的 user 与孤立的
// assistant 都丢掉。
//
// 这条规则替代了改造前那句"调用失败时不要存下半截对话"的保证 —— 那时是先调模型、
// 成功了才落库，天然不会有半截；改成异步之后必然先落库，只能靠过滤，
// 否则模型会看到一句没有回答的提问，以为自己在跟一个没答完的问题对话
//
// 依赖前提：按 id 升序排列，且占位行**不会被复用**（否则 id 顺序不再等于对话顺序，
// 这个规则和前端渲染会同时失效）
func pairHistory(msgs []models.ReviewMessage) []models.ReviewMessage {
	paired := make([]models.ReviewMessage, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		if msgs[i].Role == models.MessageRoleUser &&
			i+1 < len(msgs) && msgs[i+1].Role == models.MessageRoleAssistant {
			paired = append(paired, msgs[i], msgs[i+1])
			i++
		}
	}
	return paired
}

// latestDoneAnalysis 取该手牌最近一次成功的分析。
// 没有分析就不能追问 —— 追问是"基于结论的提问"，没结论可问
func (s *ReviewChatService) latestDoneAnalysis(userID, handID uint) (*models.ReviewAnalysis, error) {
	var analysis models.ReviewAnalysis
	err := config.DB.Where("user_id = ? AND hand_id = ? AND status = ?",
		userID, handID, models.AnalysisStatusDone).
		Order("id DESC").First(&analysis).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("这手牌还没有分析结论，先分析一次再来追问")
	}
	if err != nil {
		return nil, err
	}
	return &analysis, nil
}
