package models

import (
	"time"

	"gorm.io/gorm"
)

// UserGame 用户-场次关联表
type UserGame struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	UserID    uint           `json:"user_id" gorm:"not null;index:idx_user_game;comment:用户ID"`
	GameID    uint           `json:"game_id" gorm:"not null;index:idx_user_game;comment:场次ID"`
	JoinedAt  time.Time      `json:"joined_at" gorm:"comment:加入时间"`
	LeftAt    *time.Time     `json:"left_at" gorm:"comment:退出时间"`
	Status    string         `json:"status" gorm:"size:20;default:'active';comment:状态: active-活跃, left-已退出"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	Game Game `json:"game,omitempty" gorm:"foreignKey:GameID"`
}

// TableName 指定表名
func (UserGame) TableName() string {
	return "user_games"
}
