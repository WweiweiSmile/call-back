package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"errors"

	"gorm.io/gorm"
)

type TransactionService struct{}

// Deposit 存分
func (s *TransactionService) Deposit(operatorID uint, req *dto.DepositRequest) (*models.Transaction, error) {
	// 确定目标用户
	targetUserID := operatorID
	if req.TargetUserID != nil && *req.TargetUserID > 0 {
		targetUserID = *req.TargetUserID
	}

	// 检查游戏是否存在
	var game models.Game
	if err := config.DB.First(&game, req.GameID).Error; err != nil {
		return nil, errors.New("游戏不存在")
	}

	// 检查游戏状态 - 创建即进行中，只有已结束的游戏不能操作
	if game.Status == "ended" {
		return nil, errors.New("游戏已结束")
	}

	// 检查权限
	operatorType := models.OperatorTypeSelf
	if targetUserID != operatorID {
		// 代理操作，检查是否是游戏创建者
		if game.CreatorID != operatorID {
			return nil, errors.New("无权进行代理操作")
		}
		operatorType = models.OperatorTypeProxy
	}

	// 检查目标用户是否已加入游戏
	if err := ensureActiveParticipant(config.DB, req.GameID, targetUserID); err != nil {
		return nil, err
	}

	remark := req.Remark
	if operatorType == models.OperatorTypeProxy {
		remark = proxyRemark(config.DB, targetUserID)
	}

	var transaction *models.Transaction
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		transaction, err = applyBalanceChange(tx, balanceOpParams{
			UserID:       targetUserID,
			GameID:       req.GameID,
			TransType:    models.TransTypeDeposit,
			Amount:       req.Amount,
			OperatorID:   operatorID,
			OperatorType: operatorType,
			Remark:       remark,
			AllowCreate:  true,
		})
		return err
	})

	if err != nil {
		return nil, err
	}

	return transaction, nil
}

// Withdraw 取分
func (s *TransactionService) Withdraw(operatorID uint, req *dto.WithdrawRequest) (*models.Transaction, error) {
	// 确定目标用户
	targetUserID := operatorID
	if req.TargetUserID != nil && *req.TargetUserID > 0 {
		targetUserID = *req.TargetUserID
	}

	// 检查游戏是否存在
	var game models.Game
	if err := config.DB.First(&game, req.GameID).Error; err != nil {
		return nil, errors.New("游戏不存在")
	}

	// 检查游戏状态 - 创建即进行中，只有已结束的游戏不能操作
	if game.Status == "ended" {
		return nil, errors.New("游戏已结束")
	}

	// 检查权限
	operatorType := models.OperatorTypeSelf
	if targetUserID != operatorID {
		// 代理操作，检查是否是游戏创建者
		if game.CreatorID != operatorID {
			return nil, errors.New("无权进行代理操作")
		}
		operatorType = models.OperatorTypeProxy
	}

	// 检查目标用户是否已加入游戏
	if err := ensureActiveParticipant(config.DB, req.GameID, targetUserID); err != nil {
		return nil, err
	}

	remark := req.Remark
	if operatorType == models.OperatorTypeProxy {
		remark = proxyRemark(config.DB, targetUserID)
	}

	var transaction *models.Transaction
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		// 取分沿用既有行为：余额记录不存在直接报错，且不做余额充足性校验
		transaction, err = applyBalanceChange(tx, balanceOpParams{
			UserID:       targetUserID,
			GameID:       req.GameID,
			TransType:    models.TransTypeWithdraw,
			Amount:       req.Amount,
			OperatorID:   operatorID,
			OperatorType: operatorType,
			Remark:       remark,
			AllowCreate:  false,
		})
		return err
	})

	if err != nil {
		return nil, err
	}

	return transaction, nil
}

// GetTransactionList 获取交易记录列表
func (s *TransactionService) GetTransactionList(userID, gameID uint, page, pageSize int) (*dto.TransactionListResponse, error) {
	var transactions []models.Transaction
	var total int64

	query := config.DB.Model(&models.Transaction{}).Where("game_id = ?", gameID)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&transactions).Error; err != nil {
		return nil, err
	}

	// 收集所有需要查询的用户ID
	userIDs := make([]uint, 0, len(transactions)*2)
	for _, t := range transactions {
		userIDs = append(userIDs, t.UserID, t.OperatorID)
	}

	// 去重
	uniqueUserIDs := make([]uint, 0, len(userIDs))
	userIDMap := make(map[uint]bool)
	for _, id := range userIDs {
		if !userIDMap[id] {
			userIDMap[id] = true
			uniqueUserIDs = append(uniqueUserIDs, id)
		}
	}

	// 批量查询用户信息
	var users []models.User
	if len(uniqueUserIDs) > 0 {
		config.DB.Where("id IN ?", uniqueUserIDs).Find(&users)
	}

	// 构建用户信息映射
	userMap := make(map[uint]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	// 转换为响应格式
	transactionResponses := make([]dto.TransactionResponse, 0, len(transactions))
	for _, t := range transactions {
		userName := ""
		if user, exists := userMap[t.UserID]; exists {
			userName = user.Nickname
			if userName == "" {
				userName = user.Username
			}
		}

		operatorName := ""
		if user, exists := userMap[t.OperatorID]; exists {
			operatorName = user.Nickname
			if operatorName == "" {
				operatorName = user.Username
			}
		}

		transactionResponses = append(transactionResponses, dto.TransactionResponse{
			ID:           t.ID,
			UserID:       t.UserID,
			UserName:     userName,
			GameID:       t.GameID,
			OperatorID:   t.OperatorID,
			OperatorName: operatorName,
			OperatorType: t.OperatorType,
			TransType:    t.TransType,
			Amount:       t.Amount,
			BalanceAfter: t.BalanceAfter,
			Remark:       t.Remark,
			CreatedAt:    t.CreatedAt,
		})
	}

	return &dto.TransactionListResponse{
		Total: total,
		List:  transactionResponses,
	}, nil
}

// GetUserBalance 获取用户余额
func (s *TransactionService) GetUserBalance(userID, gameID uint) (*dto.UserBalanceResponse, error) {
	var userBalance models.UserBalance
	if err := config.DB.Where("user_id = ? AND game_id = ?", userID, gameID).First(&userBalance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &dto.UserBalanceResponse{
				UserID:         userID,
				GameID:         gameID,
				TotalDeposit:   0,
				TotalWithdraw:  0,
				CurrentBalance: 0,
				BalanceStatus:  "balanced",
			}, nil
		}
		return nil, err
	}

	return &dto.UserBalanceResponse{
		UserID:         userBalance.UserID,
		GameID:         userBalance.GameID,
		TotalDeposit:   userBalance.TotalDeposit,
		TotalWithdraw:  userBalance.TotalWithdraw,
		CurrentBalance: userBalance.CurrentBalance,
		BalanceStatus:  userBalance.BalanceStatus,
	}, nil
}

// GetGameParticipants 获取游戏参与者列表（含余额）
func (s *TransactionService) GetGameParticipants(gameID uint) ([]dto.UserBalanceResponse, error) {
	var userGames []models.UserGame
	if err := config.DB.Where("game_id = ? AND status = 'active'", gameID).Find(&userGames).Error; err != nil {
		return nil, err
	}

	userIDs := make([]uint, 0, len(userGames))
	for _, ug := range userGames {
		userIDs = append(userIDs, ug.UserID)
	}

	// 批量查询用户信息
	var users []models.User
	if len(userIDs) > 0 {
		config.DB.Where("id IN ?", userIDs).Find(&users)
	}

	// 构建用户信息映射
	userMap := make(map[uint]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	var userBalances []models.UserBalance
	if err := config.DB.Where("game_id = ? AND user_id IN ?", gameID, userIDs).Find(&userBalances).Error; err != nil {
		return nil, err
	}

	balanceMap := make(map[uint]models.UserBalance)
	for _, ub := range userBalances {
		balanceMap[ub.UserID] = ub
	}

	result := make([]dto.UserBalanceResponse, 0, len(userGames))
	for _, ug := range userGames {
		// 获取用户姓名
		userName := ""
		if user, exists := userMap[ug.UserID]; exists {
			userName = user.Nickname
			if userName == "" {
				userName = user.Username
			}
		}

		ub, exists := balanceMap[ug.UserID]
		if !exists {
			result = append(result, dto.UserBalanceResponse{
				UserID:         ug.UserID,
				UserName:       userName,
				GameID:         gameID,
				TotalDeposit:   0,
				TotalWithdraw:  0,
				CurrentBalance: 0,
				BalanceStatus:  "balanced",
			})
		} else {
			result = append(result, dto.UserBalanceResponse{
				UserID:         ub.UserID,
				UserName:       userName,
				GameID:         ub.GameID,
				TotalDeposit:   ub.TotalDeposit,
				TotalWithdraw:  ub.TotalWithdraw,
				CurrentBalance: ub.CurrentBalance,
				BalanceStatus:  ub.BalanceStatus,
			})
		}
	}

	return result, nil
}
