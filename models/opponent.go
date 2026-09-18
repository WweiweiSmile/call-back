package models

import (
	"time"

	"gorm.io/gorm"
)

// OpponentNameMaxRunes 对手名长度上限。
// 名字会进提示词，限长既是成本控制，也是提示词注入的第一道防线（与 hero_thought 同待遇）
const OpponentNameMaxRunes = 20

// Opponent 对手（"对手表"）。
//
// 作用是把同一手牌里那个"坐在 CO 的老王"变成跨手牌能指认的人：
// 没有这张表，每手牌的 villains 都是孤立的，画像与提示词都无从说"同一个老王"。
//
// 按 user_id 隔离，与 review_hands 同一套口径：同一个"老王"在两个人那里
// 是两条互不相干的记录，不共享、不互见。
//
// 名字是唯一事实来源：手牌提交时按 (user_id, LOWER(name)) 查或建，
// VillainInfo.OpponentID 是后端解析出来的结果，不是用户输入。
type Opponent struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	UserID uint   `json:"userId" gorm:"not null;index:idx_op_user_name,priority:1;comment:归属用户ID"`
	Name   string `json:"name" gorm:"size:32;not null;index:idx_op_user_name,priority:2;comment:对手称呼"`

	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Opponent) TableName() string {
	return "opponents"
}
