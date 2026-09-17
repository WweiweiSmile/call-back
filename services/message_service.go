package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"errors"
	"time"

	"gorm.io/gorm"
)

type MessageService struct{}

// messageRef 消息关联的单据。
//
// 用结构体而不是继续加位置参数：三种关联至多命中一种，写成三个 *uint 位置参数
// 时调用方全是 nil, nil, &id 这种看不出哪个是哪个的写法
type messageRef struct {
	GameID       *uint
	RequestID    *uint
	SuggestionID *uint
}

// pushMessage 在给定事务内写入一条站内消息。
// 供申请/审批事务复用，保证"单据状态变更"与"通知"同生共死。
func pushMessage(tx *gorm.DB, userID uint, msgType, title, content string, ref messageRef) error {
	return tx.Create(&models.Message{
		UserID:       userID,
		Type:         msgType,
		Title:        title,
		Content:      content,
		GameID:       ref.GameID,
		RequestID:    ref.RequestID,
		SuggestionID: ref.SuggestionID,
	}).Error
}

// GetList 获取当前用户的消息列表
func (s *MessageService) GetList(userID uint, isRead *bool, page, pageSize int) (*dto.MessageListResponse, error) {
	query := config.DB.Model(&models.Message{}).Where("user_id = ?", userID)
	if isRead != nil {
		query = query.Where("is_read = ?", *isRead)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var messages []models.Message
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&messages).Error; err != nil {
		return nil, err
	}

	return &dto.MessageListResponse{Total: total, List: toMessageResponses(messages)}, nil
}

// GetByID 取单条消息（只能取自己的）
func (s *MessageService) GetByID(userID, messageID uint) (*dto.MessageDetailResponse, error) {
	var message models.Message
	if err := config.DB.Where("id = ? AND user_id = ?", messageID, userID).First(&message).Error; err != nil {
		// 刻意不区分"不存在"与"不是你的"：不泄露别人的消息是否存在
		return nil, errors.New("消息不存在")
	}

	detail := &dto.MessageDetailResponse{
		MessageResponse: toMessageResponses([]models.Message{message})[0],
	}

	// 审批类消息附带关联单据
	switch {
	case message.Type == models.MsgTypeRequestCreated && message.RequestID != nil:
		var request models.ScoreRequest
		if err := config.DB.First(&request, *message.RequestID).Error; err == nil {
			// 复用申请单自己的响应组装，避免两处字段口径漂移
			responses := (&ScoreRequestService{}).fillResponses([]models.ScoreRequest{request})
			detail.ScoreRequest = &responses[0]
		}
		// 单据查不到时不算错误：消息本身还能看，只是没有附带内容
	case message.Type == models.MsgTypeTagSuggestionPending && message.SuggestionID != nil:
		var suggestion models.ReviewTagSuggestion
		if err := config.DB.First(&suggestion, *message.SuggestionID).Error; err == nil {
			detail.TagSuggestion = toTagSuggestionResponse(&suggestion)
		}
	}

	return detail, nil
}

// toMessageResponses 组装消息响应：分类由 type 派生，可操作性由关联单据状态派生
func toMessageResponses(messages []models.Message) []dto.MessageResponse {
	actionable := resolveActionable(messages)

	list := make([]dto.MessageResponse, 0, len(messages))
	for _, m := range messages {
		list = append(list, dto.MessageResponse{
			ID:           m.ID,
			Type:         m.Type,
			Category:     models.MessageCategoryOf(m.Type),
			Title:        m.Title,
			Content:      m.Content,
			GameID:       m.GameID,
			Actionable:   actionable[m.ID],
			RequestID:    m.RequestID,
			SuggestionID: m.SuggestionID,
			IsRead:       m.IsRead,
			ReadAt:       m.ReadAt,
			CreatedAt:    m.CreatedAt,
		})
	}
	return list
}

// resolveActionable 判断哪些审批类消息还能处理。
//
// 唯一真相是关联单据自己的状态：审批通过/驳回、申请人撤销都会改它，所以不需要在
// 消息上再存一个 handled_at —— 那要在三个地方都记得回填，漏一处就变成"按钮还在、
// 点了报错"，存量数据还得额外写迁移脚本回填。
//
// M2 的标签入库审批在这里按 type 分派到 review_tag_suggestions。
func resolveActionable(messages []models.Message) map[uint]bool {
	result := make(map[uint]bool)

	requestIDs := make([]uint, 0, len(messages))
	suggestionIDs := make([]uint, 0, len(messages))
	for _, m := range messages {
		if models.MessageCategoryOf(m.Type) != models.MsgCategoryApproval {
			continue
		}
		switch m.Type {
		case models.MsgTypeRequestCreated:
			if m.RequestID != nil {
				requestIDs = append(requestIDs, *m.RequestID)
			}
		case models.MsgTypeTagSuggestionPending:
			if m.SuggestionID != nil {
				suggestionIDs = append(suggestionIDs, *m.SuggestionID)
			}
		}
	}

	pendingRequests := map[uint]bool{}
	if len(requestIDs) > 0 {
		var requests []models.ScoreRequest
		config.DB.Model(&models.ScoreRequest{}).
			Select("id", "status").
			Where("id IN ?", requestIDs).
			Find(&requests)
		for _, r := range requests {
			pendingRequests[r.ID] = r.IsPending()
		}
	}

	pendingSuggestions := map[uint]bool{}
	if len(suggestionIDs) > 0 {
		var suggestions []models.ReviewTagSuggestion
		config.DB.Model(&models.ReviewTagSuggestion{}).
			Select("id", "status").
			Where("id IN ?", suggestionIDs).
			Find(&suggestions)
		for _, s := range suggestions {
			pendingSuggestions[s.ID] = s.IsPending()
		}
	}

	for _, m := range messages {
		switch m.Type {
		case models.MsgTypeRequestCreated:
			result[m.ID] = m.RequestID != nil && pendingRequests[*m.RequestID]
		case models.MsgTypeTagSuggestionPending:
			result[m.ID] = m.SuggestionID != nil && pendingSuggestions[*m.SuggestionID]
		}
	}
	return result
}

// GetUnreadCount 获取未读消息数
func (s *MessageService) GetUnreadCount(userID uint) (int64, error) {
	var count int64
	err := config.DB.Model(&models.Message{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count).Error
	return count, err
}

// MarkRead 标记单条已读（只能标记自己的消息）
func (s *MessageService) MarkRead(userID, messageID uint) error {
	now := time.Now()
	res := config.DB.Model(&models.Message{}).
		Where("id = ? AND user_id = ? AND is_read = ?", messageID, userID, false).
		Updates(map[string]any{"is_read": true, "read_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("消息不存在或已读")
	}
	return nil
}

// MarkAllRead 标记全部已读，可按场次过滤。
//
// 待处理的审批消息不标已读：未读红点是提醒收件人"还有事没做"的信号，
// 一进列表就清掉的话，红点没了、待办还在，用户不会再想起来点进去。
func (s *MessageService) MarkAllRead(userID uint, gameID *uint) error {
	query := config.DB.Model(&models.Message{}).
		Where("user_id = ? AND is_read = ?", userID, false)
	if gameID != nil {
		query = query.Where("game_id = ?", *gameID)
	}

	var messages []models.Message
	if err := query.Find(&messages).Error; err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	actionable := resolveActionable(messages)
	ids := make([]uint, 0, len(messages))
	for _, m := range messages {
		if !actionable[m.ID] {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	return config.DB.Model(&models.Message{}).
		Where("id IN ?", ids).
		Updates(map[string]any{"is_read": true, "read_at": time.Now()}).Error
}
