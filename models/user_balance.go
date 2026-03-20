package models

import (
	"time"

	"gorm.io/gorm"
)

// UserBalance 场次余额表
type UserBalance struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	UserID         uint           `json:"userId" gorm:"not null;index:idx_user_game_balance;comment:用户ID"`
	GameID         uint           `json:"gameId" gorm:"not null;index:idx_user_game_balance;comment:场次ID"`
	TotalDeposit   int64          `json:"totalDeposit" gorm:"default:0;comment:场次存分总量"`
	TotalWithdraw  int64          `json:"totalWithdraw" gorm:"default:0;comment:场次取分总量"`
	CurrentBalance int64          `json:"currentBalance" gorm:"default:0;comment:场次当前余额"`
	LastTransTime  *time.Time     `json:"lastTransTime" gorm:"comment:最后交易时间"`
	BalanceStatus  string         `json:"balanceStatus" gorm:"size:20;default:'balanced';comment:平衡状态: balanced-平衡, unbalanced-不平衡"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
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
