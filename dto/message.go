package dto

import "time"

// MessageResponse 站内消息响应
type MessageResponse struct {
	ID        uint       `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	GameID    *uint      `json:"gameId,omitempty"`
	RequestID *uint      `json:"requestId,omitempty"`
	IsRead    bool       `json:"isRead"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// MessageListResponse 消息列表响应
type MessageListResponse struct {
	Total int64             `json:"total"`
	List  []MessageResponse `json:"list"`
}

// UnreadCountResponse 未读消息数响应
type UnreadCountResponse struct {
	Count int64 `json:"count"`
}
