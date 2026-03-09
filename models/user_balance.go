package models

import (
	"time"

	"gorm.io/gorm"
)

// UserBalance 场次余额表
type UserBalance struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	UserID         uint           `json:"user_id" gorm:"not null;index:idx_user_game_balance;comment:用户ID"`
	GameID         uint           `json:"game_id" gorm:"not null;index:idx_user_game_balance;comment:场次ID"`
	TotalDeposit   int64          `json:"total_deposit" gorm:"default:0;comment:场次存分总量"`
	TotalWithdraw  int64          `json:"total_withdraw" gorm:"default:0;comment:场次取分总量"`
	CurrentBalance int64          `json:"current_balance" gorm:"default:0;comment:场次当前余额"`
	LastTransTime  *time.Time     `json:"last_trans_time" gorm:"comment:最后交易时间"`
	BalanceStatus  string         `json:"balance_status" gorm:"size:20;default:'balanced';comment:平衡状态: balanced-平衡, unbalanced-不平衡"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (UserBalance) TableName() string {
	return "user_balances"
}

// UpdateBalanceStatus 更新平衡状态
func (ub *UserBalance) UpdateBalanceStatus() {
	if ub.TotalDeposit-ub.TotalWithdraw == 0 {
		ub.BalanceStatus = "balanced"
	} else {
		ub.BalanceStatus = "unbalanced"
	}
}
