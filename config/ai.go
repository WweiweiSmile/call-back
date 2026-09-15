package config

// AISettings AI 分析的运行时配置快照
type AISettings struct {
	Enabled    bool
	APIKey     string
	BaseURL    string
	Model      string
	TimeoutSec int
	DailyLimit int
}

// AIConfig 读取 AI 配置。
//
// 不直接暴露 AppConfig 字段，是为了让 AppConfig 尚未初始化时也能安全取到
// 一套默认值 —— 否则任何在 LoadConfig 之前构造的组件（比如只注册路由的
// 单测）都会 nil 解引用崩溃。
func AIConfig() AISettings {
	if AppConfig == nil {
		return AISettings{
			Enabled:    false,
			BaseURL:    "https://api.deepseek.com",
			Model:      "deepseek-chat",
			TimeoutSec: 120,
			DailyLimit: 30,
		}
	}

	timeout := AppConfig.AITimeoutSec
	if timeout <= 0 {
		timeout = 120
	}
	limit := AppConfig.AIDailyLimit
	if limit <= 0 {
		limit = 30
	}

	baseURL := AppConfig.DeepSeekBaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := AppConfig.DeepSeekModel
	if model == "" {
		model = "deepseek-chat"
	}

	return AISettings{
		Enabled:    AppConfig.AIEnabled,
		APIKey:     AppConfig.DeepSeekAPIKey,
		BaseURL:    baseURL,
		Model:      model,
		TimeoutSec: timeout,
		DailyLimit: limit,
	}
}
