package models

import (
	"time"

	"gorm.io/gorm"
)

// 申请类型
const (
	ScoreReqTypeDeposit  = "deposit"  // 存分
	ScoreReqTypeWithdraw = "withdraw" // 取分
)

// 申请状态
const (
	ScoreReqStatusPending   = "pending"   // 待审核
	ScoreReqStatusApproved  = "approved"  // 已通过
	ScoreReqStatusRejected  = "rejected"  // 已驳回
	ScoreReqStatusCancelled = "cancelled" // 已撤销
)

// ScoreRequest 存取分申请表
// 普通参与者发起，场次创建者审核，通过后才真正变更余额
type ScoreRequest struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	GameID        uint           `json:"gameId" gorm:"not null;index:idx_sr_game_status,priority:1;comment:场次ID"`
	UserID        uint           `json:"userId" gorm:"not null;index:idx_sr_user_status,priority:1;comment:申请人ID(分数归属人)"`
	Type          string         `json:"type" gorm:"size:20;not null;comment:申请类型: deposit-存分, withdraw-取分"`
	Amount        int64          `json:"amount" gorm:"not null;comment:申请数量"`
	Remark        string         `json:"remark" gorm:"type:text;comment:申请人备注"`
	Status        string         `json:"status" gorm:"size:20;not null;index:idx_sr_game_status,priority:2;index:idx_sr_user_status,priority:2;comment:状态: pending-待审核, approved-已通过, rejected-已驳回, cancelled-已撤销"`
	ReviewerID    *uint          `json:"reviewerId" gorm:"comment:审核人ID(场次创建者)"`
	ReviewRemark  string         `json:"reviewRemark" gorm:"type:text;comment:审核备注/驳回理由"`
	ReviewedAt    *time.Time     `json:"reviewedAt" gorm:"comment:审核时间"`
	TransactionID *uint          `json:"transactionId" gorm:"comment:通过后生成的交易记录ID"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (ScoreRequest) TableName() string {
	return "score_requests"
}

// IsPending 是否处于待审核状态
func (r *ScoreRequest) IsPending() bool {
	return r.Status == ScoreReqStatusPending
}
