package dto

import (
	"call-go/models"
	"time"
)

// CreateGameRequest 创建游戏请求
type CreateGameRequest struct {
	Name        string       `json:"name" binding:"required"`
	Description string       `json:"description"`
	StartTime   NullableTime `json:"startTime"`
}

// JoinGameRequest 加入游戏请求
type JoinGameRequest struct {
	GameID uint `json:"gameId" binding:"required"`
}

// GameResponse 游戏响应
type GameResponse struct {
	ID                uint       `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	CreatorID         uint       `json:"creatorId"`
	CreatorName       string     `json:"creatorName,omitempty"`
	Status            string     `json:"status"`
	StartTime         *time.Time `json:"startTime"`
	EndTime           *time.Time `json:"endTime"`
	PlayerCount       int        `json:"playerCount"`
	CreatedAt         time.Time  `json:"createdAt"`
	IsCreator         bool       `json:"isCreator,omitempty"`
	IsJoined          bool       `json:"isJoined,omitempty"`
	UserTotalDeposit  int64      `json:"userTotalDeposit,omitempty"`
	UserTotalWithdraw int64      `json:"userTotalWithdraw,omitempty"`
	UserNetScore      int64      `json:"userNetScore,omitempty"`
}

// GameListResponse 游戏列表响应
type GameListResponse struct {
	Total int64          `json:"total"`
	List  []GameResponse `json:"list"`
}

// ToGameResponse 将 Game 模型转换为 GameResponse（使用动态计算的状态）
func ToGameResponse(game *models.Game, currentUserID uint, isJoined bool) GameResponse {
	return GameResponse{
		ID:          game.ID,
		Name:        game.Name,
		Description: game.Description,
		CreatorID:   game.CreatorID,
		Status:      game.GetEffectiveStatus(),
		StartTime:   game.StartTime,
		EndTime:     game.EndTime,
		PlayerCount: game.PlayerCount,
		CreatedAt:   game.CreatedAt,
		IsCreator:   game.CreatorID == currentUserID,
		IsJoined:    isJoined,
	}
}

// ToGameResponseWithBalance 将 Game 模型转换为 GameResponse，并包含用户余额信息
func ToGameResponseWithBalance(game *models.Game, currentUserID uint, isJoined bool, balance *models.UserBalance) GameResponse {
	resp := ToGameResponse(game, currentUserID, isJoined)
	if balance != nil {
		resp.UserTotalDeposit = balance.TotalDeposit
		resp.UserTotalWithdraw = balance.TotalWithdraw
		resp.UserNetScore = balance.TotalDeposit - balance.TotalWithdraw
	}
	return resp
}
