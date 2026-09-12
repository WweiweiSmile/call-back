package models

import (
	"time"
)

// Game 游戏场次表
type Game struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	Name        string     `json:"name" gorm:"size:255;not null;comment:游戏名称"`
	Description string     `json:"description" gorm:"type:text;comment:游戏描述"`
	CreatorID   uint       `json:"creatorId" gorm:"not null;index;comment:创建者ID"`
	Status      string     `json:"status" gorm:"size:20;default:'';comment:状态: ''-进行中, ended-已结束"`
	EndTime     *time.Time `json:"endTime" gorm:"comment:结束时间"`
	PlayerCount int        `json:"playerCount" gorm:"default:0;comment:当前人数"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// TableName 指定表名
func (Game) TableName() string {
	return "games"
}

const (
	GameStatusOngoing = "ongoing" // 进行中
	GameStatusEnded   = "ended"   // 已结束
)

// GetEffectiveStatus 获取游戏的有效状态
// 创建即进行中，只有创建者结束游戏后才变为 ended。
func (g *Game) GetEffectiveStatus() string {
	if g.Status == GameStatusEnded {
		return GameStatusEnded
	}
	return GameStatusOngoing
}

// IsEnded 检查游戏是否已结束
func (g *Game) IsEnded() bool {
	return g.Status == GameStatusEnded
}

// IsOngoing 检查游戏是否进行中
func (g *Game) IsOngoing() bool {
	return !g.IsEnded()
}
