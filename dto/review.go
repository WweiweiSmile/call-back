package dto

import (
	"call-go/models"
	"time"
)

// ReviewHandRequest 手牌请求。
//
// 创建与更新共用：更新是整体替换语义 —— 前端提交的永远是一份完整表单，
// 做部分更新反而要处理"字段没传"和"字段传了空值"的区分，得不偿失。
type ReviewHandRequest struct {
	GameID       *uint                 `json:"gameId"`
	Title        string                `json:"title"`
	TableSize    int                   `json:"tableSize"`
	HeroPosition string                `json:"heroPosition" binding:"required"`
	HeroCards    string                `json:"heroCards" binding:"required"`
	HeroStackBB  float64               `json:"heroStackBb"`
	Stakes       string                `json:"stakes"`
	Board        string                `json:"board"`
	VillainCount int                   `json:"villainCount"`
	Villains     []models.VillainInfo  `json:"villains"`
	PotType      string                `json:"potType"`
	Streets      []models.StreetRecord `json:"streets"`
	HeroThought  string                `json:"heroThought"`
	Result       string                `json:"result"`
	ResultAmount *float64              `json:"resultAmount"`
	HeroTags     []string              `json:"heroTags"`
}

// ReviewHandResponse 手牌响应
type ReviewHandResponse struct {
	ID            uint                  `json:"id"`
	GameID        *uint                 `json:"gameId,omitempty"`
	GameName      string                `json:"gameName,omitempty"`
	Title         string                `json:"title"`
	TableSize     int                   `json:"tableSize"`
	HeroPosition  string                `json:"heroPosition"`
	HeroCards     string                `json:"heroCards"`
	HeroStackBB   float64               `json:"heroStackBb"`
	Stakes        string                `json:"stakes"`
	Board         string                `json:"board"`
	VillainCount  int                   `json:"villainCount"`
	Villains      []models.VillainInfo  `json:"villains"`
	PotType       string                `json:"potType"`
	Streets       []models.StreetRecord `json:"streets"`
	HeroThought   string                `json:"heroThought"`
	Result        string                `json:"result"`
	ResultAmount  *float64              `json:"resultAmount,omitempty"`
	HeroTags      []string              `json:"heroTags"`
	AnalyzeStatus string                `json:"analyzeStatus"`
	CreatedAt     time.Time             `json:"createdAt"`
	UpdatedAt     time.Time             `json:"updatedAt"`
}

// ReviewHandListResponse 手牌列表响应
type ReviewHandListResponse struct {
	Total int64                `json:"total"`
	List  []ReviewHandResponse `json:"list"`
}

// ReviewLeakTagResponse 漏洞标签响应
type ReviewLeakTagResponse struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	SortOrder   int    `json:"sortOrder"`
}

// ReviewLeakTagListResponse 标签字典响应
type ReviewLeakTagListResponse struct {
	List []ReviewLeakTagResponse `json:"list"`
}

// ToReviewHandResponse 将 ReviewHand 模型转换为响应。
// gameName 由 service 批量查好后传入，避免这里逐条查库。
func ToReviewHandResponse(hand *models.ReviewHand, gameName string) ReviewHandResponse {
	// 列表/详情接口直接渲染这些数组，返回 nil 会让前端多写一层判空
	streets := hand.Streets
	if streets == nil {
		streets = []models.StreetRecord{}
	}
	villains := hand.Villains
	if villains == nil {
		villains = []models.VillainInfo{}
	}
	tags := hand.HeroTags
	if tags == nil {
		tags = []string{}
	}

	return ReviewHandResponse{
		ID:            hand.ID,
		GameID:        hand.GameID,
		GameName:      gameName,
		Title:         hand.Title,
		TableSize:     hand.TableSize,
		HeroPosition:  hand.HeroPosition,
		HeroCards:     hand.HeroCards,
		HeroStackBB:   hand.HeroStackBB,
		Stakes:        hand.Stakes,
		Board:         hand.Board,
		VillainCount:  hand.VillainCount,
		Villains:      villains,
		PotType:       hand.PotType,
		Streets:       streets,
		HeroThought:   hand.HeroThought,
		Result:        hand.Result,
		ResultAmount:  hand.ResultAmount,
		HeroTags:      tags,
		AnalyzeStatus: hand.AnalyzeStatus,
		CreatedAt:     hand.CreatedAt,
		UpdatedAt:     hand.UpdatedAt,
	}
}

// ToReviewLeakTagResponse 将标签模型转换为响应，只暴露前端需要的字段
func ToReviewLeakTagResponse(tag *models.ReviewLeakTag) ReviewLeakTagResponse {
	return ReviewLeakTagResponse{
		Code:        tag.Code,
		Name:        tag.Name,
		Category:    tag.Category,
		Description: tag.Description,
		SortOrder:   tag.SortOrder,
	}
}
