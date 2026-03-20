package models

import (
	"time"

	"gorm.io/gorm"
)

// Game 游戏场次表
type Game struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"size:255;not null;comment:游戏名称"`
	Description string         `json:"description" gorm:"type:text;comment:游戏描述"`
	CreatorID   uint           `json:"creatorId" gorm:"not null;index;comment:创建者ID"`
	Status      string         `json:"status" gorm:"size:20;default:'';comment:状态: ''-未结束, ended-已结束"`
	StartTime   *time.Time     `json:"startTime" gorm:"comment:开始时间"`
	EndTime     *time.Time     `json:"endTime" gorm:"comment:结束时间"`
	PlayerCount int            `json:"playerCount" gorm:"default:0;comment:当前人数"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Game) TableName() string {
	return "games"
}

const (
	GameStatusNotStarted = "pending" // 未开始
	GameStatusOngoing    = "ongoing" // 进行中
	GameStatusEnded      = "ended"   // 已结束
)

// GetEffectiveStatus 获取游戏的有效状态（动态计算）
// - pending: 服务器时间 < 开始时间 且 status != ended
// - ongoing: 服务器时间 >= 开始时间 且 status != ended
// - ended: status == ended
func (g *Game) GetEffectiveStatus() string {
	if g.Status == GameStatusEnded {
		return GameStatusEnded
	}

	now := time.Now()

	// 如果没有设置开始时间，默认为进行中
	if g.StartTime == nil {
		return GameStatusOngoing
	}

	if now.Before(*g.StartTime) {
		return GameStatusNotStarted
	}

	return GameStatusOngoing
}

// IsEnded 检查游戏是否已结束
func (g *Game) IsEnded() bool {
	return g.Status == GameStatusEnded
}

// IsNotStarted 检查游戏是否未开始
func (g *Game) IsNotStarted() bool {
	if g.IsEnded() {
		return false
	}
	if g.StartTime == nil {
		return false
	}
	return time.Now().Before(*g.StartTime)
}

// IsOngoing 检查游戏是否进行中
func (g *Game) IsOngoing() bool {
	return !g.IsEnded() && !g.IsNotStarted()
}
