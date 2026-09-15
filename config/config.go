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

	// ---------- AI 分析 ----------
	// API Key 只存在后端，绝不下发前端
	DeepSeekAPIKey  string
	DeepSeekBaseURL string
	DeepSeekModel   string
	// AITimeoutSec 单次分析超时。模型返回结构化 JSON 通常 20~60 秒
	AITimeoutSec int
	// AIDailyLimit 单用户每日分析次数上限，防止额度被刷爆
	AIDailyLimit int
	// AIEnabled 未配置 API Key 时自动关闭，前端据此提示"AI 分析未启用"，
	// 而不是等用户点了分析再看一个失败
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

		DeepSeekAPIKey:  apiKey,
		DeepSeekBaseURL: GetEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		DeepSeekModel:   GetEnv("DEEPSEEK_MODEL", "deepseek-chat"),
		AITimeoutSec:    GetEnvInt("AI_TIMEOUT_SECONDS", 120),
		AIDailyLimit:    GetEnvInt("AI_DAILY_LIMIT", 30),
		AIEnabled:       apiKey != "",
	}

	if AppConfig.AIEnabled {
		log.Printf("AI analysis enabled, model: %s", AppConfig.DeepSeekModel)
	} else {
		log.Println("Warning: DEEPSEEK_API_KEY not set, AI analysis is disabled")
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
