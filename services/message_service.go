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

// pushMessage 在给定事务内写入一条站内消息。
// 供申请/审批事务复用，保证"申请状态变更"与"通知"同生共死。
func pushMessage(tx *gorm.DB, userID uint, msgType, title, content string, gameID, requestID *uint) error {
	return tx.Create(&models.Message{
		UserID:    userID,
		Type:      msgType,
		Title:     title,
		Content:   content,
		GameID:    gameID,
		RequestID: requestID,
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

	list := make([]dto.MessageResponse, 0, len(messages))
	for _, m := range messages {
		list = append(list, dto.MessageResponse{
			ID:        m.ID,
			Type:      m.Type,
			Title:     m.Title,
			Content:   m.Content,
			GameID:    m.GameID,
			RequestID: m.RequestID,
			IsRead:    m.IsRead,
			ReadAt:    m.ReadAt,
			CreatedAt: m.CreatedAt,
		})
	}

	return &dto.MessageListResponse{Total: total, List: list}, nil
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

// MarkAllRead 标记全部已读，可按场次过滤
func (s *MessageService) MarkAllRead(userID uint, gameID *uint) error {
	query := config.DB.Model(&models.Message{}).
		Where("user_id = ? AND is_read = ?", userID, false)
	if gameID != nil {
		query = query.Where("game_id = ?", *gameID)
	}
	return query.Updates(map[string]any{"is_read": true, "read_at": time.Now()}).Error
}
