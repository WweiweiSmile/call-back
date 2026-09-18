package models

import "time"

// UserPreference 用户级默认设置。
//
// 目前只有复盘用的盲注默认值。单独成表而不是往 users 上加列：
// users 是登录鉴权的核心表，往上面堆业务偏好会让每次读用户信息都多带一堆字段，
// 以后新增设置项也不必再动鉴权链路。
type UserPreference struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	UserID uint `json:"userId" gorm:"not null;uniqueIndex;comment:每个用户一条"`

	SmallBlindBB float64 `json:"smallBlindBb" gorm:"default:0.5;comment:默认小盲(BB)"`
	BigBlindBB   float64 `json:"bigBlindBb" gorm:"default:1;comment:默认大盲(BB)"`
	AnteBB       float64 `json:"anteBb" gorm:"default:0;comment:默认前注(BB)，每人一份"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (UserPreference) TableName() string {
	return "user_preferences"
}
