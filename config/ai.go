package config

const (
	defaultAITimeoutSec = 120
	defaultAIDailyLimit = 30

	// DefaultAIBaseURL / DefaultAIModel 默认预设的兜底值。
	// 部署方可以用 DEEPSEEK_BASE_URL / DEEPSEEK_MODEL 覆盖
	DefaultAIBaseURL = "https://api.deepseek.com"
	DefaultAIModel   = "deepseek-chat"
)

// AISettings AI 分析的**服务端级**运行时参数。
//
// 凭据与模型名自 BYOK 起按用户解析，不在这个结构里 ——
// 它们来自 user_preferences，由 services.AISettingService.ResolveCallSettings 给出
type AISettings struct {
	// TimeoutSec 单次模型调用超时
	TimeoutSec int
	// DailyLimit 单用户每日分析次数上限
	DailyLimit int
}

// AIConfig 读取 AI 运行时参数。
//
// 不直接暴露 AppConfig 字段，是为了让 AppConfig 尚未初始化时也能安全取到
// 一套默认值 —— 否则任何在 LoadConfig 之前构造的组件（比如只注册路由的
// 单测）都会 nil 解引用崩溃。
func AIConfig() AISettings {
	timeout := defaultAITimeoutSec
	limit := defaultAIDailyLimit

	if AppConfig != nil {
		if AppConfig.AITimeoutSec > 0 {
			timeout = AppConfig.AITimeoutSec
		}
		if AppConfig.AIDailyLimit > 0 {
			limit = AppConfig.AIDailyLimit
		}
	}

	return AISettings{TimeoutSec: timeout, DailyLimit: limit}
}

// AIPreset 一个预设供应商，供设置页一键填充
type AIPreset struct {
	// Key 稳定标识，前端用它做 chip 的选中态
	Key string
	// Name 展示名
	Name string
	// BaseURL 填到 /v1 这一层，客户端自己拼 /chat/completions
	BaseURL string
	// Model 预填的模型名。只是预填，用户可以改
	Model string
}

// 预设的稳定标识
const (
	AIPresetDeepSeek = "deepseek"
	AIPresetOpenAI   = "openai"
	AIPresetMoonshot = "moonshot"
	AIPresetZhipu    = "zhipu"
	AIPresetCustom   = "custom"
)

// DefaultAIPreset 默认预设。
//
// baseUrl / 模型名取自 DEEPSEEK_BASE_URL / DEEPSEEK_MODEL，所以部署方改这两个
// 环境变量就能改「没配过的用户打开模型设置时看到的预填值」，不必改代码。
//
// 这也是升级后老用户唯一的过渡路径：他们只需要粘一把自己的 Key，
// 地址与模型名已经预填好了
func DefaultAIPreset() AIPreset {
	baseURL := DefaultAIBaseURL
	model := DefaultAIModel

	if AppConfig != nil {
		if AppConfig.DeepSeekBaseURL != "" {
			baseURL = AppConfig.DeepSeekBaseURL
		}
		if AppConfig.DeepSeekModel != "" {
			model = AppConfig.DeepSeekModel
		}
	}

	return AIPreset{
		Key:     AIPresetDeepSeek,
		Name:    "DeepSeek",
		BaseURL: baseURL,
		Model:   model,
	}
}

// AIPresets 预设供应商列表。
//
// 是静态字典而不是库表：没有运营侧收益，加一家只是改这里一行。
// 模型名会随供应商迭代而过时，但填错只是预填值不准，用户可以手改 ——
// 所以这里的取值刻意挑各家最长期稳定的那几个。
//
// custom 的 BaseURL / Model 刻意留空，前端选中它时不得把空值灌进输入框
func AIPresets() []AIPreset {
	return []AIPreset{
		DefaultAIPreset(),
		{
			Key:     AIPresetOpenAI,
			Name:    "OpenAI",
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o-mini",
		},
		{
			Key:     AIPresetMoonshot,
			Name:    "Kimi",
			BaseURL: "https://api.moonshot.cn/v1",
			Model:   "moonshot-v1-8k",
		},
		{
			Key:     AIPresetZhipu,
			Name:    "智谱 GLM",
			BaseURL: "https://open.bigmodel.cn/api/paas/v4",
			Model:   "glm-4-flash",
		},
		{
			Key:  AIPresetCustom,
			Name: "自定义",
		},
	}
}
