package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	gameStatusEnded = "ended"
)

type GameService struct{}

// CreateGame 创建游戏
func (s *GameService) CreateGame(creatorID uint, req *dto.CreateGameRequest) (*models.Game, error) {
	game := &models.Game{
		Name:        req.Name,
		Description: req.Description,
		CreatorID:   creatorID,
		Status:      "", // 空字符串表示未结束
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		PlayerCount: 0,
	}

	if err := config.DB.Create(game).Error; err != nil {
		return nil, err
	}

	return game, nil
}

// GetGameList 获取游戏列表
func (s *GameService) GetGameList(userID uint, status string, page, pageSize int) (*dto.GameListResponse, error) {
	var games []models.Game
	var total int64

	query := config.DB.Model(&models.Game{})
	// 不显示已结束的游戏
	query = query.Where("status != ? OR status IS NULL", gameStatusEnded)

	// 如果指定了状态筛选，需要在内存中筛选（因为状态是动态计算的）
	// 先查询所有未结束的游戏
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&games).Error; err != nil {
		return nil, err
	}

	// 转换为响应格式，并在内存中进行状态筛选
	gameResponses := make([]dto.GameResponse, 0, len(games))
	for _, game := range games {
		effectiveStatus := game.GetEffectiveStatus()

		// 如果指定了状态筛选，跳过不符合的
		if status != "" && effectiveStatus != status {
			continue
		}

		// 检查用户是否已加入
		var userGame models.UserGame
		isJoined := false
		config.DB.Where("user_id = ? AND game_id = ? AND status = 'active'", userID, game.ID).First(&userGame)
		if userGame.ID > 0 {
			isJoined = true
		}

		gameResponses = append(gameResponses, dto.ToGameResponse(&game, userID, isJoined))
	}

	return &dto.GameListResponse{
		Total: total,
		List:  gameResponses,
	}, nil
}

// GetGame 获取游戏详情
func (s *GameService) GetGame(gameID uint) (*models.Game, error) {
	var game models.Game
	if err := config.DB.First(&game, gameID).Error; err != nil {
		return nil, err
	}
	return &game, nil
}

// JoinGame 加入游戏
func (s *GameService) JoinGame(userID, gameID uint) error {
	// 检查游戏是否存在
	var game models.Game
	if err := config.DB.First(&game, gameID).Error; err != nil {
		return errors.New("游戏不存在")
	}

	// 检查游戏是否已结束
	if game.IsEnded() {
		return errors.New("游戏已结束，无法加入")
	}

	// 检查是否已加入
	var existingUserGame models.UserGame
	err := config.DB.Where("user_id = ? AND game_id = ?", userID, gameID).First(&existingUserGame).Error
	if err == nil {
		if existingUserGame.Status == "active" {
			return errors.New("您已加入该游戏")
		}
		// 重新激活
		existingUserGame.Status = "active"
		existingUserGame.LeftAt = nil
		config.DB.Save(&existingUserGame)
		return nil
	}

	// 开始事务
	return config.DB.Transaction(func(tx *gorm.DB) error {
		// 创建用户-游戏关联
		userGame := &models.UserGame{
			UserID:   userID,
			GameID:   gameID,
			JoinedAt: time.Now(),
			Status:   "active",
		}
		if err := tx.Create(userGame).Error; err != nil {
			return err
		}

		// 初始化用户余额
		userBalance := &models.UserBalance{
			UserID:         userID,
			GameID:         gameID,
			TotalDeposit:   0,
			TotalWithdraw:  0,
			CurrentBalance: 0,
			BalanceStatus:  "balanced",
		}
		if err := tx.Create(userBalance).Error; err != nil {
			return err
		}

		// 更新游戏人数
		if err := tx.Model(&game).UpdateColumn("player_count", gorm.Expr("player_count + 1")).Error; err != nil {
			return err
		}

		return nil
	})
}

// LeaveGame 退出游戏
func (s *GameService) LeaveGame(userID, gameID uint) error {
	var userGame models.UserGame
	if err := config.DB.Where("user_id = ? AND game_id = ? AND status = 'active'", userID, gameID).First(&userGame).Error; err != nil {
		return errors.New("您未加入该游戏")
	}

	now := time.Now()
	userGame.Status = "left"
	userGame.LeftAt = &now

	return config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&userGame).Error; err != nil {
			return err
		}

		// 更新游戏人数
		var game models.Game
		tx.First(&game, gameID)
		if err := tx.Model(&game).UpdateColumn("player_count", gorm.Expr("player_count - 1")).Error; err != nil {
			return err
		}

		return nil
	})
}

// GetMyGames 获取我的游戏
func (s *GameService) GetMyGames(userID uint, page, pageSize int) (*dto.GameListResponse, error) {
	var userGames []models.UserGame
	var total int64

	query := config.DB.Model(&models.UserGame{}).Where("user_id = ? AND status = 'active'", userID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Preload("Game").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&userGames).Error; err != nil {
		return nil, err
	}

	gameResponses := make([]dto.GameResponse, 0, len(userGames))
	for _, ug := range userGames {
		// 不显示已结束的游戏
		if ug.Game.IsEnded() {
			continue
		}
		gameResponses = append(gameResponses, dto.ToGameResponse(&ug.Game, userID, true))
	}

	return &dto.GameListResponse{
		Total: total,
		List:  gameResponses,
	}, nil
}

// GetCreatedGames 获取我创建的游戏
func (s *GameService) GetCreatedGames(userID uint, page, pageSize int) (*dto.GameListResponse, error) {
	var games []models.Game
	var total int64

	query := config.DB.Model(&models.Game{}).Where("creator_id = ?", userID)
	// 不显示已结束的游戏
	query = query.Where("status != ? OR status IS NULL", gameStatusEnded)
	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&games).Error; err != nil {
		return nil, err
	}

	gameResponses := make([]dto.GameResponse, 0, len(games))
	for _, game := range games {
		gameResponses = append(gameResponses, dto.ToGameResponse(&game, userID, false))
	}

	return &dto.GameListResponse{
		Total: total,
		List:  gameResponses,
	}, nil
}

// EndGame 结束游戏
func (s *GameService) EndGame(creatorID, gameID uint) error {
	var game models.Game
	if err := config.DB.First(&game, gameID).Error; err != nil {
		return errors.New("游戏不存在")
	}

	if game.CreatorID != creatorID {
		return errors.New("只有创建者可以结束游戏")
	}

	if game.IsEnded() {
		return errors.New("游戏已经结束")
	}

	// 更新游戏状态为已结束
	game.Status = models.GameStatusEnded
	if err := config.DB.Save(&game).Error; err != nil {
		return err
	}

	return nil
}
