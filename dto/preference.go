package dto

import "call-go/models"

// UserPreferenceResponse 用户默认设置响应
type UserPreferenceResponse struct {
	SmallBlindBB float64 `json:"smallBlindBb"`
	BigBlindBB   float64 `json:"bigBlindBb"`
	AnteBB       float64 `json:"anteBb"`
}

// UpdateUserPreferenceRequest 更新默认设置。
//
// 整体替换语义：前端永远提交完整的三项。用值而不是指针，
// 是因为 0 对前注是合法值（不打前注），用指针区分"没传"与"传了 0"反而绕。
type UpdateUserPreferenceRequest struct {
	SmallBlindBB float64 `json:"smallBlindBb"`
	BigBlindBB   float64 `json:"bigBlindBb"`
	AnteBB       float64 `json:"anteBb"`
}

// ToUserPreferenceResponse 把模型转成响应
func ToUserPreferenceResponse(pref *models.UserPreference) UserPreferenceResponse {
	return UserPreferenceResponse{
		SmallBlindBB: pref.SmallBlindBB,
		BigBlindBB:   pref.BigBlindBB,
		AnteBB:       pref.AnteBB,
	}
}
