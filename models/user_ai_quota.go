package models

import "time"

// UserAIQuota 用户 AI 点数余额。
//
// 是**一次性累计总额**，不按天重置：新用户 9 点（= 3 次复盘），用完由运营
// 直接改这里的 points 充值（没有发放 UI）。之所以不做成"每日重置"，是因为
// 在服务端统一出 Key 之后，"谁主动来要更多额度"本身就是最强的需求信号。
//
// 列的所有权硬约定（同 user_preferences）：这张表只归 services.AIQuotaService 写。
// 其它 service 一律不得构造字面量模型去 Save 它 —— 额度要在热路径上高频
// 原子自减（UPDATE ... WHERE points >= ?），与整行 Save 混用会互相覆盖
type UserAIQuota struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	UserID uint `json:"userId" gorm:"not null;uniqueIndex;comment:每个用户一条"`

	// Points 剩余点数。运营充值改这一列。
	// default 与 config.DefaultAIPointLimit 必须一致
	Points int `json:"points" gorm:"not null;default:9;comment:剩余点数，运营充值改这里"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 表名
func (UserAIQuota) TableName() string {
	return "user_ai_quotas"
}
