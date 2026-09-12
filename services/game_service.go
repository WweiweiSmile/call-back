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
	// 创建即进行中：空字符串表示未结束
	game := &models.Game{
		Name:        req.Name,
		Description: req.Description,
		CreatorID:   creatorID,
		Status:      "",
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

	// 状态过滤下推到 SQL，保证 total 与实际返回条数一致
	if status == models.GameStatusEnded {
		// 明确查询已结束的游戏
		query = query.Where("status = ?", gameStatusEnded)
	} else {
		// 进行中 / 未指定状态：都不展示已结束的游戏
		query = query.Where("status != ? OR status IS NULL", gameStatusEnded)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&games).Error; err != nil {
		return nil, err
	}

	// 转换为响应格式
	gameResponses := make([]dto.GameResponse, 0, len(games))
	for _, game := range games {
		// 检查用户是否已加入
		var userGame models.UserGame
		isJoined := false
		config.DB.Where("user_id = ? AND game_id = ? AND status = 'active'", userID, game.ID).First(&userGame)
		if userGame.ID > 0 {
			isJoined = true
		}

		resp := dto.ToGameResponse(&game, userID, isJoined)
		// 填充创建人用户名
		var creator models.User
		if err := config.DB.First(&creator, game.CreatorID).Error; err == nil {
			if creator.Nickname != "" {
				resp.CreatorName = creator.Nickname
			} else {
				resp.CreatorName = creator.Username
			}
		}
		gameResponses = append(gameResponses, resp)
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
func (s *GameService) GetMyGames(userID uint, status string, page, pageSize int) (*dto.GameListResponse, error) {
	var userGames []models.UserGame
	var total int64

	// 关联 games 表，把状态过滤下推到 SQL，保证 total 与实际返回条数一致
	query := config.DB.Model(&models.UserGame{}).
		Joins("JOIN games ON games.id = user_games.game_id").
		Where("user_games.user_id = ? AND user_games.status = 'active'", userID)

	if status == models.GameStatusEnded {
		query = query.Where("games.status = ?", gameStatusEnded)
	} else {
		query = query.Where("games.status != ? OR games.status IS NULL", gameStatusEnded)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Select("user_games.*").
		Order("user_games.created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&userGames).Error; err != nil {
		return nil, err
	}

	// 手动加载游戏数据
	gameIDs := make([]uint, 0, len(userGames))
	for _, ug := range userGames {
		gameIDs = append(gameIDs, ug.GameID)
	}

	var games []models.Game
	if len(gameIDs) > 0 {
		config.DB.Find(&games, gameIDs)
	}

	// 查询用户在这些游戏中的余额
	var balances []models.UserBalance
	if len(gameIDs) > 0 {
		config.DB.Where("user_id = ? AND game_id IN ?", userID, gameIDs).Find(&balances)
	}

	// 创建 map 方便查找
	gameMap := make(map[uint]models.Game)
	for _, g := range games {
		gameMap[g.ID] = g
	}

	balanceMap := make(map[uint]models.UserBalance)
	for _, b := range balances {
		balanceMap[b.GameID] = b
	}

	gameResponses := make([]dto.GameResponse, 0, len(userGames))
	for _, ug := range userGames {
		// 从 map 中获取游戏
		game, exists := gameMap[ug.GameID]
		if !exists || game.ID == 0 {
			continue // 游戏不存在，跳过
		}

		// 获取用户余额
		var balance *models.UserBalance
		if b, ok := balanceMap[game.ID]; ok {
			balance = &b
		}

		resp := dto.ToGameResponseWithBalance(&game, userID, true, balance)
		// 填充创建人用户名
		var creator models.User
		if err := config.DB.First(&creator, game.CreatorID).Error; err == nil {
			if creator.Nickname != "" {
				resp.CreatorName = creator.Nickname
			} else {
				resp.CreatorName = creator.Username
			}
		}
		gameResponses = append(gameResponses, resp)
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
		resp := dto.ToGameResponse(&game, userID, false)
		// 填充创建人用户名
		var creator models.User
		if err := config.DB.First(&creator, game.CreatorID).Error; err == nil {
			if creator.Nickname != "" {
				resp.CreatorName = creator.Nickname
			} else {
				resp.CreatorName = creator.Username
			}
		}
		gameResponses = append(gameResponses, resp)
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

	now := time.Now()
	// 更新游戏状态为已结束，同时设置结束时间
	if err := config.DB.Model(&game).Updates(map[string]interface{}{
		"status":   models.GameStatusEnded,
		"end_time": now,
	}).Error; err != nil {
		return err
	}

	return nil
}
