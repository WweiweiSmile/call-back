package services

import (
	"call-go/config"
	"call-go/models"
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	// ChatMessageMaxRunes 单条追问的字数上限，与 HeroThought 同一口径。
	// 用户输入会进提示词，限长既是成本控制也是注入面的收敛
	ChatMessageMaxRunes = 1000
	// ChatHistoryLimit 拼进提示词的历史条数上限，防止上下文随对话无限增长
	ChatHistoryLimit = 20
	// ChatMaxTokens 助手回复长度上限。要求 300 字以内，600 token 留有余量
	ChatMaxTokens = 600
)

// ReviewChatService 追问对话：基于已有的分析结论回答学员的问题
type ReviewChatService struct {
	reviewService *ReviewService
	memoryService *ReviewMemoryService
	aiClient      *AIClient
}

func NewReviewChatService() *ReviewChatService {
	return &ReviewChatService{
		reviewService: &ReviewService{},
		memoryService: NewReviewMemoryService(),
		aiClient:      NewAIClient(),
	}
}

// Ask 学员追问，返回落库后的一问一答。
//
// 顺序上先调模型、成功了再把两条消息一起落库：
// 调用失败时若已经存下用户那条，历史里就会留下一句没有回复的话，
// 下一轮把它当上下文喂给模型，它会以为自己在跟一个没答完的问题对话。
// 失败时前端保留输入框内容让用户重试，比存半截对话更干净。
func (s *ReviewChatService) Ask(
	ctx context.Context,
	userID, handID uint,
	question string,
) (*models.ReviewMessage, *models.ReviewMessage, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, nil, fmt.Errorf("追问内容不能为空")
	}
	if len([]rune(question)) > ChatMessageMaxRunes {
		return nil, nil, fmt.Errorf("追问不能超过 %d 字", ChatMessageMaxRunes)
	}

	// GetHand 自带归属校验，拿着别人的 hand_id 只会得到"不存在"
	hand, err := s.reviewService.GetHand(userID, handID)
	if err != nil {
		return nil, nil, err
	}

	analysis, err := s.latestDoneAnalysis(userID, handID)
	if err != nil {
		return nil, nil, err
	}

	history, err := s.recentHistory(userID, handID)
	if err != nil {
		return nil, nil, err
	}
	// 本轮问题作为最后一条拼进提示词，此时它还没落库
	history = append(history, models.ReviewMessage{
		Role:    models.MessageRoleUser,
		Content: question,
	})

	memory := s.memoryService.BuildMemoryContext(userID)
	system, userPrompt := BuildChatPrompt(hand, analysis, memory, history)

	completion, err := s.aiClient.Complete(ctx, system, userPrompt, ChatMaxTokens)
	if err != nil {
		return nil, nil, err
	}

	userMsg := &models.ReviewMessage{
		UserID:     userID,
		HandID:     handID,
		AnalysisID: analysis.ID,
		Role:       models.MessageRoleUser,
		Content:    question,
	}
	assistantMsg := &models.ReviewMessage{
		UserID:     userID,
		HandID:     handID,
		AnalysisID: analysis.ID,
		Role:       models.MessageRoleAssistant,
		Content:    completion.Content,
		TokensIn:   completion.TokensIn,
		TokensOut:  completion.TokensOut,
	}

	// 两条一起写：只有一问没有一答的记录没有任何意义
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(userMsg).Error; err != nil {
			return err
		}
		return tx.Create(assistantMsg).Error
	}); err != nil {
		return nil, nil, fmt.Errorf("保存对话失败: %w", err)
	}

	return userMsg, assistantMsg, nil
}

// ListMessages 读某手牌的完整对话，按时间升序。
// 同样先校验手牌归属，避免用别人的 hand_id 探到对话内容
func (s *ReviewChatService) ListMessages(userID, handID uint) ([]models.ReviewMessage, error) {
	if _, err := s.reviewService.GetHand(userID, handID); err != nil {
		return nil, err
	}

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

// recentHistory 取最近若干条对话，按时间升序返回
func (s *ReviewChatService) recentHistory(userID, handID uint) ([]models.ReviewMessage, error) {
	var messages []models.ReviewMessage
	if err := config.DB.Where("user_id = ? AND hand_id = ?", userID, handID).
		Order("id DESC").Limit(ChatHistoryLimit).Find(&messages).Error; err != nil {
		return nil, err
	}

	// 倒序取"最近 N 条"之后再翻回时间顺序，否则模型看到的对话是倒着的
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
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
