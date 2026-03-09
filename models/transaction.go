package models

import (
	"time"

	"gorm.io/gorm"
)

// Transaction 存取分记录表
type Transaction struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	UserID        uint           `json:"user_id" gorm:"not null;index;comment:用户ID"`
	GameID        uint           `json:"game_id" gorm:"not null;index;comment:场次ID"`
	OperatorID    uint           `json:"operator_id" gorm:"not null;comment:操作人ID"`
	OperatorType  string         `json:"operator_type" gorm:"size:20;not null;comment:操作类型: self-自主操作, proxy-代理操作"`
	TransType     string         `json:"trans_type" gorm:"size:20;not null;comment:交易类型: deposit-存分, withdraw-取分"`
	Amount        int64          `json:"amount" gorm:"not null;comment:数量"`
	BalanceAfter  int64          `json:"balance_after" gorm:"not null;comment:操作后余额"`
	Remark        string         `json:"remark" gorm:"type:text;comment:备注"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Transaction) TableName() string {
	return "transactions"
}
