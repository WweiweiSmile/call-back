package dto

import (
	"call-go/models"
	"time"
)

// ReviewMessageRequest 追问请求
type ReviewMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

// ReviewMessageResponse 一条对话消息
type ReviewMessageResponse struct {
	ID   uint   `json:"id"`
	Role string `json:"role"`
	// Content 消息正文
	Content string `json:"content"`
	// TokensIn/TokensOut 只有 assistant 消息有值，前端不需要展示，留作对账
	TokensIn  int       `json:"tokensIn,omitempty"`
	TokensOut int       `json:"tokensOut,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// AskReviewMessageResponse 一次追问的结果。
// 用户那条也一并返回：落库时机由后端掌握（模型答成功才写），
// 前端据权威的 id 与时间戳渲染，不必自己造
type AskReviewMessageResponse struct {
	Question ReviewMessageResponse `json:"question"`
	Answer   ReviewMessageResponse `json:"answer"`
}

// ReviewMessageListResponse 对话历史
type ReviewMessageListResponse struct {
	List []ReviewMessageResponse `json:"list"`
}

// ToReviewMessageResponse 转换单条消息
func ToReviewMessageResponse(msg *models.ReviewMessage) ReviewMessageResponse {
	return ReviewMessageResponse{
		ID:        msg.ID,
		Role:      msg.Role,
		Content:   msg.Content,
		TokensIn:  msg.TokensIn,
		TokensOut: msg.TokensOut,
		CreatedAt: msg.CreatedAt,
	}
}
