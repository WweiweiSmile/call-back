package dto

import (
	"call-go/models"
	"time"
)

// ReviewProfileResponse 用户复盘画像
type ReviewProfileResponse struct {
	UserID        uint `json:"userId"`
	HandsReviewed int  `json:"handsReviewed"`
	// Leaks 漏洞排行，已按出现次数从多到少排好
	Leaks []models.ProfileLeakStat `json:"leaks"`
	// Strengths 最近做对的地方。不做聚合计数，优点没有标签可聚合
	Strengths      []models.ProfileStrengthItem `json:"strengths"`
	Summary        string                       `json:"summary"`
	SummaryVersion int                          `json:"summaryVersion"`
	// LastSummaryAt 为空表示还没生成过总结
	LastSummaryAt *time.Time `json:"lastSummaryAt,omitempty"`
	// SummaryStatus 总结重写任务的状态。pending/running 时前端的 Summary 是旧版本，
	// 该显示"生成中"并轮询，而不是把旧总结当成刚生成的结果
	SummaryStatus string `json:"summaryStatus"`
	// SummaryError 只有 failed 时有值
	SummaryError string `json:"summaryError,omitempty"`
}

// ToReviewProfileResponse 转换画像。
// 数组兜成空数组而不是 nil，前端少写一层判空
func ToReviewProfileResponse(p *models.ReviewProfile) ReviewProfileResponse {
	leaks := p.Leaks
	if leaks == nil {
		leaks = []models.ProfileLeakStat{}
	}
	strengths := p.Strengths
	if strengths == nil {
		strengths = []models.ProfileStrengthItem{}
	}

	return ReviewProfileResponse{
		UserID:         p.UserID,
		HandsReviewed:  p.HandsReviewed,
		Leaks:          leaks,
		Strengths:      strengths,
		Summary:        p.Summary,
		SummaryVersion: p.SummaryVersion,
		LastSummaryAt:  p.LastSummaryAt,
		SummaryStatus:  p.SummaryStatus,
		SummaryError:   p.SummaryError,
	}
}

// ReviewInsightResponse 一条历史洞察（画像页钻取用）
//
// 带上手牌的展示信息，让画像页能直接渲染"这条漏洞出现在哪手牌"，
// 并支持点进去看详情，不必再查一次手牌接口
type ReviewInsightResponse struct {
	InsightID uint   `json:"insightId"`
	HandID    uint   `json:"handId"`
	HandTitle string `json:"handTitle"`
	Position  string `json:"position"`
	TableSize int    `json:"tableSize"`
	HeroCards string `json:"heroCards"`
	Severity  int    `json:"severity"`
	Evidence  string `json:"evidence"`
	// CreatedAt 这条洞察的产生时间
	CreatedAt time.Time `json:"createdAt"`
}

// ReviewInsightListResponse 某个漏洞的全部历史证据
type ReviewInsightListResponse struct {
	List []ReviewInsightResponse `json:"list"`
}
