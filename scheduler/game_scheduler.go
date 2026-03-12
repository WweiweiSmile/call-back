package scheduler

import (
	"call-go/config"
	"call-go/models"
	"log"
	"time"
)

const (
	gameStatusPending = "pending"
	gameStatusOngoing = "ongoing"
)

// GameScheduler 游戏状态调度器
type GameScheduler struct {
	ticker *time.Ticker
	stop   chan struct{}
}

// NewGameScheduler 创建游戏状态调度器
func NewGameScheduler() *GameScheduler {
	return &GameScheduler{
		stop: make(chan struct{}),
	}
}

// Start 启动调度器
func (s *GameScheduler) Start(interval time.Duration) {
	s.ticker = time.NewTicker(interval)
	log.Printf("Game scheduler started with interval: %v", interval)

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.updateGameStatuses()
			case <-s.stop:
				s.ticker.Stop()
				log.Println("Game scheduler stopped")
				return
			}
		}
	}()

	// 立即执行一次
	s.updateGameStatuses()
}

// Stop 停止调度器
func (s *GameScheduler) Stop() {
	close(s.stop)
}

// updateGameStatuses 更新游戏状态
func (s *GameScheduler) updateGameStatuses() {
	now := time.Now()

	// 更新应该开始但还未开始的游戏
	result := config.DB.Model(&models.Game{}).
		Where("status = ? AND start_time IS NOT NULL AND start_time <= ?", gameStatusPending, now).
		Update("status", gameStatusOngoing)

	if result.Error != nil {
		log.Printf("Failed to update game statuses: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("Updated %d games from pending to ongoing", result.RowsAffected)
	}
}
