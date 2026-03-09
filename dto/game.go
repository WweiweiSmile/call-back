package dto

import "time"

// CreateGameRequest 创建游戏请求
type CreateGameRequest struct {
	Name        string     `json:"name" binding:"required"`
	Description string     `json:"description"`
	StartTime   *time.Time `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
}

// JoinGameRequest 加入游戏请求
type JoinGameRequest struct {
	GameID uint `json:"game_id" binding:"required"`
}

// GameResponse 游戏响应
type GameResponse struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatorID   uint       `json:"creator_id"`
	CreatorName string     `json:"creator_name,omitempty"`
	Status      string     `json:"status"`
	StartTime   *time.Time `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	PlayerCount int        `json:"player_count"`
	CreatedAt   time.Time  `json:"created_at"`
	IsCreator   bool       `json:"is_creator,omitempty"`
	IsJoined    bool       `json:"is_joined,omitempty"`
}

// GameListResponse 游戏列表响应
type GameListResponse struct {
	Total int64           `json:"total"`
	List  []GameResponse  `json:"list"`
}
