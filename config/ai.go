package config

const (
	// defaultAIDailyLimit 仅剩兼容旧 .env 用，不再是任何额度判据（见 Config.AIDailyLimit）
	defaultAIDailyLimit = 30

	// DefaultAIPointLimit 新用户的初始 AI 点数。
	// 9 点 = 3 次复盘，也可以拆成 9 轮追问或 3 次画像。
	// 与 models.UserAIQuota 的 gorm default 必须保持一致
	DefaultAIPointLimit = 9

	// 各功能消耗的点数。**单一事实来源**：扣点处（services）与 /ai-status 响应（dto）共用。
	// 口径：1 次复盘 = 3 轮追问 = 1 次画像更新
	AICostAnalysis        = 3
	AICostOpponentProfile = 3
	AICostProfileSummary  = 3
	AICostChat            = 1

	// DefaultAIBaseURL / DefaultAIModel 默认预设的兜底值。
	// 部署方可以用 DEEPSEEK_BASE_URL / DEEPSEEK_MODEL 覆盖
	DefaultAIBaseURL = "https://api.deepseek.com"
	DefaultAIModel   = "deepseek-chat"
)

// AISettings AI 分析的**服务端级**运行时参数。
//
// 凭据与模型名不在这个结构里 —— 自 2026-10 起凭据有两级来源：
// 用户自己的（BYOK，存 user_preferences）优先，其次服务端 Key（AI_SERVER_API_KEY）。
// 解析统一走 services.AISettingService.ResolveCallSettings
//
// 刻意没有"单次调用超时"这一项：K3 这类「始终推理」模型的思考时间不受我们控制，
// 一次分析可能要几分钟，掐断只会把已经烧掉的推理 token 白白扔掉。
// 详见 services/ai_client.go 里关于不设超时的说明
type AISettings struct {
	// PointLimit 新用户的初始点数
	PointLimit int
}

// AIConfig 读取 AI 运行时参数。
//
// 不直接暴露 AppConfig 字段，是为了让 AppConfig 尚未初始化时也能安全取到
// 一套默认值 —— 否则任何在 LoadConfig 之前构造的组件（比如只注册路由的
// 单测）都会 nil 解引用崩溃。
func AIConfig() AISettings {
	limit := DefaultAIPointLimit

	if AppConfig != nil {
		if AppConfig.AIPointLimit > 0 {
			limit = AppConfig.AIPointLimit
		}
	}

	return AISettings{PointLimit: limit}
}

// AIServerSettings 服务端兜底的模型凭据（AI_SERVER_*）
type AIServerSettings struct {
	// APIKey 为空表示没有服务端兜底 —— 那时只有自备 Key 的用户能用 AI 分析
	APIKey  string
	BaseURL string
	Model   string
}

// AIServerConfig 读服务端凭据。与 AIConfig 同理是 nil-safe 的，
// 且刻意不复用 DeepSeek* —— 那三个字段现在服务的是 BYOK 设置页的预填值
func AIServerConfig() AIServerSettings {
	key := ""
	baseURL := DefaultAIBaseURL
	model := DefaultAIModel

	if AppConfig != nil {
		key = AppConfig.ServerAIAPIKey
		if AppConfig.ServerAIBaseURL != "" {
			baseURL = AppConfig.ServerAIBaseURL
		}
		if AppConfig.ServerAIModel != "" {
			model = AppConfig.ServerAIModel
		}
	}

	return AIServerSettings{APIKey: key, BaseURL: baseURL, Model: model}
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
			Model:   "kimi-k3",
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
