package services

import (
	"call-go/models"
	"errors"
	"time"

	"gorm.io/gorm"
)

// 余额操作相关错误
var (
	ErrBalanceNotFound = errors.New("用户余额记录不存在")
	ErrNotParticipant  = errors.New("目标用户未加入该游戏")
)

// balanceOpParams 一次余额变更的全部输入。
// 调用方负责所有前置校验与 remark 拼装，本结构只描述"怎么改"。
type balanceOpParams struct {
	UserID       uint // 分数归属人 → transactions.user_id
	GameID       uint
	TransType    string // deposit | withdraw
	Amount       int64
	OperatorID   uint   // 实际操作人 → transactions.operator_id
	OperatorType string // self | proxy
	Remark       string // 调用方拼好的最终备注，本函数不再改写
	// AllowCreate 余额记录不存在时是否自动创建（存分为 true，取分为 false）
	AllowCreate bool
}

// ensureActiveParticipant 校验用户是该场次的在册参与者
func ensureActiveParticipant(db *gorm.DB, gameID, userID uint) error {
	var userGame models.UserGame
	if err := db.Where("user_id = ? AND game_id = ? AND status = 'active'", userID, gameID).
		First(&userGame).Error; err != nil {
		return ErrNotParticipant
	}
	return nil
}

// proxyRemark 代理操作的备注文案。
// 注意：这是刻意沿用既有行为——调用方传入的 remark 会被无条件覆盖，
// 且 Nickname 为空时不会回退到 Username。保持原样以避免线上备注口径变化。
func proxyRemark(db *gorm.DB, targetUserID uint) string {
	var targetUser models.User
	db.First(&targetUser, targetUserID)
	return "代理用户：" + targetUser.Nickname
}

// applyBalanceChange 在给定事务内原子增减余额并写入一条 transactions。
//
// 必须在调用方开启的事务里执行；本函数只做数学与落库，不做权限与业务校验。
// 余额更新走原子 SQL（而非先读后写），因余额不足或记录缺失导致的"未命中行"
// 由 RowsAffected 判定，顺带解决并发下的超取与丢更新。
func applyBalanceChange(tx *gorm.DB, p balanceOpParams) (*models.Transaction, error) {
	delta := p.Amount
	if p.TransType == models.TransTypeWithdraw {
		delta = -p.Amount
	}

	// 确保余额记录存在。并发安全由下面的原子 UPDATE 保证，这里的读取只用于分支。
	var exists models.UserBalance
	err := tx.Where("user_id = ? AND game_id = ?", p.UserID, p.GameID).First(&exists).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !p.AllowCreate {
			return nil, ErrBalanceNotFound
		}
		created := models.UserBalance{
			UserID:        p.UserID,
			GameID:        p.GameID,
			BalanceStatus: "balanced",
		}
		if err := tx.Create(&created).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// 写 last_trans_time 保证任何一次变更都会让行内容发生变化，
	// 这样 MySQL 的 RowsAffected 才能可靠地表示"是否命中行"。
	updates := map[string]any{
		"current_balance": gorm.Expr("current_balance + ?", delta),
		"last_trans_time": time.Now(),
	}
	if delta >= 0 {
		updates["total_deposit"] = gorm.Expr("total_deposit + ?", delta)
	} else {
		updates["total_withdraw"] = gorm.Expr("total_withdraw + ?", -delta)
	}

	// 注意：取分不校验余额是否充足。本业务的余额允许为负
	// （总存分 != 总取分即"不平衡"，是合法状态），所以这里不加 current_balance 守卫。
	res := tx.Model(&models.UserBalance{}).
		Where("user_id = ? AND game_id = ?", p.UserID, p.GameID).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrBalanceNotFound
	}

	// 同一事务内读回用于 balance_after；此时该行已被上面的 UPDATE 持锁，读到的值一致
	var ub models.UserBalance
	if err := tx.Where("user_id = ? AND game_id = ?", p.UserID, p.GameID).First(&ub).Error; err != nil {
		return nil, err
	}

	// 平衡状态单独更新，避免在同一条 UPDATE 的 SET 里依赖列求值顺序
	status := ub.BalanceStatus
	ub.UpdateBalanceStatus()
	if ub.BalanceStatus != status {
		if err := tx.Model(&models.UserBalance{}).
			Where("user_id = ? AND game_id = ?", p.UserID, p.GameID).
			Update("balance_status", ub.BalanceStatus).Error; err != nil {
			return nil, err
		}
	}

	transaction := &models.Transaction{
		UserID:       p.UserID,
		GameID:       p.GameID,
		OperatorID:   p.OperatorID,
		OperatorType: p.OperatorType,
		TransType:    p.TransType,
		Amount:       p.Amount,
		BalanceAfter: ub.CurrentBalance,
		Remark:       p.Remark,
	}
	if err := tx.Create(transaction).Error; err != nil {
		return nil, err
	}

	return transaction, nil
}
