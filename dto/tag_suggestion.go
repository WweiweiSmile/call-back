package dto

import "time"

// ApproveTagSuggestionRequest 审批通过标签建议。
//
// 模型只给了 Name 与 Reason，而提示词里让它选标签靠的是 code + name + description，
// 所以 code 与 category 必须由审批人补齐——直接拿模型编的字符串入库，等于把
// "字典外的标签"原样塞进字典，还是聚不了合
type ApproveTagSuggestionRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Category    string `json:"category" binding:"required,oneof=preflop postflop mental bankroll"`
	Description string `json:"description"`
}

// RejectTagSuggestionRequest 驳回标签建议
type RejectTagSuggestionRequest struct {
	ReviewRemark string `json:"reviewRemark"`
}

// TagSuggestionResponse AI 提议的新标签
type TagSuggestionResponse struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	HitCount     int        `json:"hitCount"`
	ReviewerID   *uint      `json:"reviewerId,omitempty"`
	ReviewRemark string     `json:"reviewRemark"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	TagID        *uint      `json:"tagId,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}
