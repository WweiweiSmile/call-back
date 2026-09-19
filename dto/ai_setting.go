package dto

import "call-go/config"

// AIModelPresetResponse 预设供应商，供设置页一键填充
type AIModelPresetResponse struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

// AIModelSettingsResponse 我的模型配置。
//
// 只回显掩码，任何情况下都不下发明文 Key —— 想改就只能重新填一把
type AIModelSettingsResponse struct {
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"hasApiKey"`
	// APIKeyHint 已经是展示用的掩码（如 "••••a1b2"），不是原始尾号。
	// 没有 Key 时是空串。前端不要再加工
	APIKeyHint string `json:"apiKeyHint"`
	// Presets 只有 GET 会填。PUT 的响应里省略 —— 前端保存后不需要重新渲染预设
	Presets []AIModelPresetResponse `json:"presets,omitempty"`
}

// UpdateAIModelSettingsRequest 保存模型配置。
type UpdateAIModelSettingsRequest struct {
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
	// APIKey 三态：nil = 不改动已存的 Key；"" = 清除；非空 = 替换。
	//
	// 用指针，与 UpdateUserPreferenceRequest 刻意"用值不用指针"的理由正好相反：
	// 那里 0 是合法值，这里"没传"和"传空串"是两个不同的动作。
	// 前端永远拿不到明文，所以绝不能用整体替换语义 —— 那会让"只改模型名"
	// 的请求把 Key 顺手清掉
	APIKey *string `json:"apiKey"`
}

// ToAIModelSettingsResponse 组装响应。
//
// ⚠️ 只接收密文与尾号，**绝不接收明文** —— 唯一的输入口子收窄到参数类型上，
// 比靠"记得不要传明文"可靠
func ToAIModelSettingsResponse(
	baseURL, model, apiKeyHint string,
	hasAPIKey bool,
	presets []AIModelPresetResponse,
) *AIModelSettingsResponse {
	masked := ""
	if hasAPIKey && apiKeyHint != "" {
		// 掩码在后端拼：前端拿到的就是最终展示串，两端不必各写一份格式化逻辑
		masked = "••••" + apiKeyHint
	}

	return &AIModelSettingsResponse{
		BaseURL:    baseURL,
		Model:      model,
		HasAPIKey:  hasAPIKey,
		APIKeyHint: masked,
		Presets:    presets,
	}
}

// ToAIModelPresetResponses 预设列表转响应
func ToAIModelPresetResponses(presets []config.AIPreset) []AIModelPresetResponse {
	result := make([]AIModelPresetResponse, 0, len(presets))
	for _, p := range presets {
		result = append(result, AIModelPresetResponse{
			Key:     p.Key,
			Name:    p.Name,
			BaseURL: p.BaseURL,
			Model:   p.Model,
		})
	}
	return result
}
