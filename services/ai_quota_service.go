package services

import (
	"call-go/config"
	"call-go/models"
	"errors"
	"log"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInsufficientPoints 点数不足。调用方据此给出可行动的提示
var ErrInsufficientPoints = errors.New("AI 点数不足")

// AIQuotaService 用户 AI 点数余额的读写。
//
// ⚠️ 必须是叶子服务：只依赖 config / models。理由同 AISettingService ——
// ReviewAnalysisService / ReviewChatService / OpponentProfileService 都会用它，
// 一旦它反过来依赖它们中的任何一个就会成环
type AIQuotaService struct{}

// GetPoints 读剩余点数。**没有行时惰性补一行**（默认点数）再返回。
//
// 这里破例在读路径上写库，理由与 PreferenceService.Get 那条"读不写"不同：
// 额度是运营要**直接查看并手动修改**的余额。表里空着的话，"给某人加点"就得先
// 判断有没有行（运维会踩空），而且库里的状态与接口返回的余额对不上
func (s *AIQuotaService) GetPoints(userID uint) (int, error) {
	var row models.UserAIQuota
	err := config.DB.Where("user_id = ?", userID).First(&row).Error
	if err == nil {
		return row.Points, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	// 没有行：补一行默认的。建行失败（并发下别人刚建也会走到这里）不该让读失败 ——
	// 回落默认值，下一次读就能拿到真实余额
	if err := s.ensureRow(userID); err != nil {
		log.Printf("[AI 额度] 惰性建行失败 user=%d: %v", userID, err)
	}
	return config.AIConfig().PointLimit, nil
}

// ensureRow 幂等建行（并发安全：已存在则不动）。
//
// 用户在认证中心侧创建，这里不做跨仓注册钩子 —— 行由第一次读或扣点补上
func (s *AIQuotaService) ensureRow(userID uint) error {
	return config.DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.UserAIQuota{
			UserID: userID,
			Points: config.AIConfig().PointLimit,
		}).Error
}

// DeductPoints 扣点。余额不足时返回 ErrInsufficientPoints。
//
// 用一条**原子条件更新**（WHERE points >= cost）杜绝超支：两个并发请求不可能
// 都成功。RowsAffected == 0 就说明余额不够 —— 这样点数永远不会变成负数
func (s *AIQuotaService) DeductPoints(userID uint, cost int) error {
	if cost <= 0 {
		return nil
	}

	if err := s.ensureRow(userID); err != nil {
		return err
	}

	res := config.DB.Model(&models.UserAIQuota{}).
		Where("user_id = ? AND points >= ?", userID, cost).
		UpdateColumn("points", gorm.Expr("points - ?", cost))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInsufficientPoints
	}
	return nil
}

// RefundPoints 退点（best-effort）。
//
// 只在**已经成功扣过点**之后的失败分支里调用：模型调用失败或落库失败时，
// 用户没拿到结果就不该扣。失败只记日志、不再向上抛 —— 退点失败不该
// 把主流程真正的错误盖掉
func (s *AIQuotaService) RefundPoints(userID uint, amount int) {
	if amount <= 0 {
		return
	}
	res := config.DB.Model(&models.UserAIQuota{}).
		Where("user_id = ?", userID).
		UpdateColumn("points", gorm.Expr("points + ?", amount))
	if res.Error != nil {
		log.Printf("[AI 额度] 退点失败 user=%d amount=%d: %v", userID, amount, res.Error)
	}
}
