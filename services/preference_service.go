package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"call-go/utils"
	"errors"

	"gorm.io/gorm"
)

type PreferenceService struct{}

// Get 取用户的默认盲注设置。
//
// 没设置过的用户不写库，直接返回一套默认值 —— 否则每个注册用户都要多一条
// 他从未主动设置过的记录，而库里根本分不出"他选的就是默认值"和"他没设置过"。
// 只有真的在设置页保存了才建行。
func (s *PreferenceService) Get(userID uint) (*dto.UserPreferenceResponse, error) {
	var pref models.UserPreference
	err := config.DB.Where("user_id = ?", userID).First(&pref).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &dto.UserPreferenceResponse{
			SmallBlindBB: models.DefaultSmallBlindBB,
			BigBlindBB:   models.DefaultBigBlindBB,
			AnteBB:       models.DefaultAnteBB,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	resp := dto.ToUserPreferenceResponse(&pref)
	return &resp, nil
}

// newPreferenceDefaults 一条"用户从未设置过任何东西"的偏好记录（尚未落库）。
//
// 单独抽出来是因为它有两个调用方：本文件的 Update 与 AISettingService。
// 两边在**建行**时都必须带上盲注默认值 —— PreferenceService.Get 是直接从行里
// 读的（不像"没有行"那样有兜底分支），一旦插入的行是 0/0/0，只配了模型、
// 没碰过盲注的用户拿到的默认值就从 0.5/1/0 变成 0/0/0，录入页预填会静默失效。
//
// 别指望 gorm 的 default: tag 兜住：零值字段是否从 INSERT 里省略取决于
// 字段与 tag 的组合，是个不该赌的行为。默认值只有这一处构造点
func newPreferenceDefaults(userID uint) *models.UserPreference {
	return &models.UserPreference{
		UserID:       userID,
		SmallBlindBB: models.DefaultSmallBlindBB,
		BigBlindBB:   models.DefaultBigBlindBB,
		AnteBB:       models.DefaultAnteBB,
	}
}

// Update 保存默认盲注设置（upsert）。
//
// 校验与手牌录入共用 utils.ValidateBlinds：两处口径必须一致，
// 否则会出现"设置页存得进去、录入手牌却被拒"这种自相矛盾的状态。
//
// ⚠️ 本方法只拥有盲注三列。模型配置（AI 四列）归 AISettingService 管，
// 这里只是把从库里读出来的原值原样写回，绝不要在这条路径上构造字面量模型
func (s *PreferenceService) Update(userID uint, req *dto.UpdateUserPreferenceRequest) (*dto.UserPreferenceResponse, error) {
	if err := utils.ValidateBlinds(req.SmallBlindBB, req.BigBlindBB, req.AnteBB); err != nil {
		return nil, err
	}

	var pref models.UserPreference
	err := config.DB.Where("user_id = ?", userID).First(&pref).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		pref = *newPreferenceDefaults(userID)
		pref.SmallBlindBB = req.SmallBlindBB
		pref.BigBlindBB = req.BigBlindBB
		pref.AnteBB = req.AnteBB
		if err := config.DB.Create(&pref).Error; err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		pref.SmallBlindBB = req.SmallBlindBB
		pref.BigBlindBB = req.BigBlindBB
		pref.AnteBB = req.AnteBB
		if err := config.DB.Save(&pref).Error; err != nil {
			return nil, err
		}
	}

	resp := dto.ToUserPreferenceResponse(&pref)
	return &resp, nil
}
