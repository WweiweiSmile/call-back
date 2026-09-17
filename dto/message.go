package dto

import "time"

// MessageResponse 站内消息响应
type MessageResponse struct {
	ID       uint   `json:"id"`
	Type     string `json:"type"`
	Category string `json:"category"` // approval-待收件人处理 / notice-结果告知
	Title    string `json:"title"`
	Content  string `json:"content"`
	GameID   *uint  `json:"gameId,omitempty"`
	// Actionable 是否还能处理。由关联单据的状态派生，不是存储字段
	Actionable   bool       `json:"actionable"`
	RequestID    *uint      `json:"requestId,omitempty"`
	SuggestionID *uint      `json:"suggestionId,omitempty"`
	IsRead       bool       `json:"isRead"`
	ReadAt       *time.Time `json:"readAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// MessageDetailResponse 消息详情。
//
// 审批类消息附带关联单据：正文里只有"谁、多少分、存还是取"，没有申请人填的备注，
// 让审批人闭眼批一笔资金变动是不合适的。
type MessageDetailResponse struct {
	MessageResponse
	ScoreRequest  *ScoreRequestResponse  `json:"scoreRequest,omitempty"`
	TagSuggestion *TagSuggestionResponse `json:"tagSuggestion,omitempty"`
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
