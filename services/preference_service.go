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

// Update 保存默认盲注设置（upsert）。
//
// 校验与手牌录入共用 utils.ValidateBlinds：两处口径必须一致，
// 否则会出现"设置页存得进去、录入手牌却被拒"这种自相矛盾的状态。
func (s *PreferenceService) Update(userID uint, req *dto.UpdateUserPreferenceRequest) (*dto.UserPreferenceResponse, error) {
	if err := utils.ValidateBlinds(req.SmallBlindBB, req.BigBlindBB, req.AnteBB); err != nil {
		return nil, err
	}

	var pref models.UserPreference
	err := config.DB.Where("user_id = ?", userID).First(&pref).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		pref = models.UserPreference{
			UserID:       userID,
			SmallBlindBB: req.SmallBlindBB,
			BigBlindBB:   req.BigBlindBB,
			AnteBB:       req.AnteBB,
		}
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
