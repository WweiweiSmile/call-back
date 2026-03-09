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
	CreatorID   uint           `json:"creator_id" gorm:"not null;index;comment:创建者ID"`
	Status      string         `json:"status" gorm:"size:20;default:'pending';comment:状态: pending-即将开始, ongoing-进行中, ended-已结束"`
	StartTime   *time.Time     `json:"start_time" gorm:"comment:开始时间"`
	EndTime     *time.Time     `json:"end_time" gorm:"comment:结束时间"`
	PlayerCount int            `json:"player_count" gorm:"default:0;comment:当前人数"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Game) TableName() string {
	return "games"
}
