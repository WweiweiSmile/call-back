package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	ServerPort string
	JWTSecret  string

	// PrefEncryptionKey 用户 API Key 的 AES-256-GCM 主密钥。
	// 生成：openssl rand -base64 32。为空或长度不对时「模型设置」只能读不能写，
	// 服务本身仍能正常启动（见 utils.SetPreferenceEncryptionKey）
	PrefEncryptionKey string

	// ---------- AI 分析 ----------
	// Deprecated: 自 BYOK（用户自带 Key）起不再对任何用户生效，服务端不再回退这把 Key。
	// 保留字段与 .env 行只为兼容旧部署，代码里不得再读
	DeepSeekAPIKey string
	// DeepSeekBaseURL / DeepSeekModel 不再是"服务端调用用的模型"，
	// 改为默认预设 DefaultAIPreset 的取值来源：没配过模型的用户打开设置页
	// 时看到的预填值就是它们
	DeepSeekBaseURL string
	DeepSeekModel   string
	// AITimeoutSec 单次分析超时。模型返回结构化 JSON 通常 20~60 秒
	AITimeoutSec int
	// AIDailyLimit 单用户每日分析次数上限，防止额度被刷爆。
	// 不区分用的是谁的 Key —— 卡的是"这个账号一天能发起多少次分析"
	AIDailyLimit int
	// Deprecated: 恒等于 DeepSeekAPIKey != ""，不再是"AI 是否可用"的判据。
	// 用户级的可用性判断在 services.AISettingService.HasUsableConfig
	AIEnabled bool
}

var AppConfig *Config

func LoadConfig() error {
	// 尝试加载 .env 文件（如果存在）
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	apiKey := GetEnv("DEEPSEEK_API_KEY", "")

	AppConfig = &Config{
		DBHost:     GetEnv("DB_HOST", "localhost"),
		DBPort:     GetEnv("DB_PORT", "3306"),
		DBUser:     GetEnv("DB_USER", "root"),
		DBPassword: GetEnv("DB_PASSWORD", ""),
		DBName:     GetEnv("DB_NAME", "call_game"),
		ServerPort: GetEnv("SERVER_PORT", "8080"),
		JWTSecret:  GetEnv("JWT_SECRET", "call-game-secret-key-2026"),

		PrefEncryptionKey: GetEnv("PREF_ENCRYPTION_KEY", ""),

		DeepSeekAPIKey:  apiKey,
		DeepSeekBaseURL: GetEnv("DEEPSEEK_BASE_URL", DefaultAIBaseURL),
		DeepSeekModel:   GetEnv("DEEPSEEK_MODEL", DefaultAIModel),
		AITimeoutSec:    GetEnvInt("AI_TIMEOUT_SECONDS", defaultAITimeoutSec),
		AIDailyLimit:    GetEnvInt("AI_DAILY_LIMIT", defaultAIDailyLimit),
		AIEnabled:       apiKey != "",
	}

	preset := DefaultAIPreset()
	log.Printf("AI 分析使用用户自备 Key；默认预设 %s / %s", preset.BaseURL, preset.Model)
	if apiKey != "" {
		log.Println("Warning: DEEPSEEK_API_KEY 已废弃，自 BYOK 起不再对任何用户生效（Key 现在按用户存在 user_preferences 里）")
	}

	log.Println("Config loaded successfully")
	return nil
}

func GetEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetEnvInt 读整数配置，非法值与空值一律退回默认值，避免一个手滑的环境变量让服务起不来
func GetEnvInt(key string, defaultValue int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("Warning: %s=%q 不是合法整数，使用默认值 %d", key, raw, defaultValue)
		return defaultValue
	}
	return value
}
