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
	// Content 消息正文。assistant 消息在后台答完之前是空串，**不能拿它当完成判据**，
	// 要看 Status —— 模型也可能返回空内容，那种情况下两者都空
	Content string `json:"content"`
	// Status pending/running/done/failed。user 消息恒为 done
	Status string `json:"status"`
	// ErrorMsg 只有 failed 的 assistant 消息有值
	ErrorMsg string `json:"errorMsg,omitempty"`
	// TokensIn/TokensOut 只有 assistant 消息有值，前端不需要展示，留作对账
	TokensIn  int       `json:"tokensIn,omitempty"`
	TokensOut int       `json:"tokensOut,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// AskReviewMessageResponse 一次追问的结果。
//
// 异步：Answer 通常是 status=pending 的占位记录（Content 为空），前端据它轮询。
// 用户那条也一并返回：它的落库时机由后端掌握，前端据权威的 id 与时间戳渲染，
// 不必自己造
type AskReviewMessageResponse struct {
	Question ReviewMessageResponse `json:"question"`
	Answer   ReviewMessageResponse `json:"answer"`
	// Inflight 本次没有新建任务，返回的是这手牌正在跑的那对 ——
	// 前端据此提示"上一条还在思考中"，并且不该清空输入框
	Inflight bool `json:"inflight"`
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
		Status:    msg.Status,
		ErrorMsg:  msg.ErrorMsg,
		TokensIn:  msg.TokensIn,
		TokensOut: msg.TokensOut,
		CreatedAt: msg.CreatedAt,
	}
}
