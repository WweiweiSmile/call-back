package dto

import (
	"call-go/models"
	"time"
)

// ReviewAnalysisResponse 一次分析的响应。
//
// 刻意不返回 InputSnapshot / RawOutput：前者含完整提示词（内部实现细节，
// 也容易让用户误以为提示词里的内容是自己说过的话），后者是模型原始输出，
// 排查时去数据库看即可。
type ReviewAnalysisResponse struct {
	ID     uint   `json:"id"`
	HandID uint   `json:"handId"`
	Status string `json:"status"`

	Model         string `json:"model"`
	PromptVersion string `json:"promptVersion"`

	// Result 仅在 status=done 时有值
	Result *models.AnalysisResult `json:"result,omitempty"`

	TokensIn   int   `json:"tokensIn,omitempty"`
	TokensOut  int   `json:"tokensOut,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`

	ErrorMsg string `json:"errorMsg,omitempty"`

	// Stale 该结论对应的手牌内容已被修改。
	//
	// 改完内容后旧结论不能直接丢（用户可能想对比），但也绝不能当成
	// 当前内容的结论展示。前端据此加一条"内容已修改，建议重新分析"的提示
	Stale bool `json:"stale,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ReviewAnalysisListResponse 分析列表响应
type ReviewAnalysisListResponse struct {
	List []ReviewAnalysisResponse `json:"list"`
}

// RequestAnalysisResponse 触发分析的响应。
//
// Reused 告诉前端这次是不是直接复用了已有结论（没调用模型、没扣额度），
// 前端据此决定文案是"已重新分析"还是"内容没变，直接看上次结论"
type RequestAnalysisResponse struct {
	Analysis ReviewAnalysisResponse `json:"analysis"`
	Reused   bool                   `json:"reused"`
}

// AIStatusResponse AI 可用状态
type AIStatusResponse struct {
	Enabled    bool `json:"enabled"`
	DailyLimit int  `json:"dailyLimit"`
	UsedToday  int  `json:"usedToday"`
	Remaining  int  `json:"remaining"`
}

// ToReviewAnalysisResponse 模型转响应
func ToReviewAnalysisResponse(a *models.ReviewAnalysis) ReviewAnalysisResponse {
	resp := ReviewAnalysisResponse{
		ID:            a.ID,
		HandID:        a.HandID,
		Status:        a.Status,
		Model:         a.Model,
		PromptVersion: a.PromptVersion,
		TokensIn:      a.TokensIn,
		TokensOut:     a.TokensOut,
		DurationMs:    a.DurationMs,
		ErrorMsg:      a.ErrorMsg,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
	// 只有成功的结果才下发，避免前端拿到半截数据
	if a.Status == models.AnalysisStatusDone {
		resp.Result = a.Result
	}
	return resp
}
