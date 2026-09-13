package dto

import "time"

// CreateScoreRequestRequest 提交存取分申请
// 注意：没有 UserID 字段——申请人一律取自 JWT，不接受请求体指定
type CreateScoreRequestRequest struct {
	GameID uint   `json:"gameId" binding:"required"`
	Type   string `json:"type" binding:"required,oneof=deposit withdraw"`
	Amount int64  `json:"amount" binding:"required,min=1"`
	Remark string `json:"remark"`
}

// ReviewScoreRequestRequest 审核申请（通过/驳回）
type ReviewScoreRequestRequest struct {
	ReviewRemark string `json:"reviewRemark"`
}

// ScoreRequestResponse 申请单响应
type ScoreRequestResponse struct {
	ID            uint       `json:"id"`
	GameID        uint       `json:"gameId"`
	GameName      string     `json:"gameName,omitempty"`
	UserID        uint       `json:"userId"`
	UserName      string     `json:"userName,omitempty"`
	Type          string     `json:"type"`
	Amount        int64      `json:"amount"`
	Remark        string     `json:"remark"`
	Status        string     `json:"status"`
	ReviewerID    *uint      `json:"reviewerId,omitempty"`
	ReviewerName  string     `json:"reviewerName,omitempty"`
	ReviewRemark  string     `json:"reviewRemark"`
	ReviewedAt    *time.Time `json:"reviewedAt,omitempty"`
	TransactionID *uint      `json:"transactionId,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// ScoreRequestListResponse 申请单列表响应
type ScoreRequestListResponse struct {
	Total int64                  `json:"total"`
	List  []ScoreRequestResponse `json:"list"`
}
