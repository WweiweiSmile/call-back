package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	// AIDailyLimit 单用户每日分析次数上限，防止额度被刷爆。
	// 不区分用的是谁的 Key —— 卡的是"这个账号一天能发起多少次分析"
	AIDailyLimit int
	// Deprecated: 恒等于 DeepSeekAPIKey != ""，不再是"AI 是否可用"的判据。
	// 用户级的可用性判断在 services.AISettingService.HasUsableConfig
	AIEnabled bool
}

var AppConfig *Config

func LoadConfig() error {
	envFile, tried, err := locateEnvFile()
	if err != nil {
		return err
	}
	if envFile == "" {
		// 没有 .env 是合法的部署方式（全靠环境变量），不是错误。
		// 但要把找过哪些地方写进启动日志 —— 生产上「.env 明明放了却没生效」
		// 这一类问题全靠这一行定位
		log.Printf("Warning: 没找到 .env（找过 %v），改用环境变量", tried)
	} else if err := godotenv.Load(envFile); err != nil {
		return fmt.Errorf("加载 %s 失败: %w", envFile, err)
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
		AIDailyLimit:    GetEnvInt("AI_DAILY_LIMIT", defaultAIDailyLimit),
		AIEnabled:       apiKey != "",
	}

	preset := DefaultAIPreset()
	log.Printf("AI 分析使用用户自备 Key；默认预设 %s / %s", preset.BaseURL, preset.Model)
	if apiKey != "" {
		log.Println("Warning: DEEPSEEK_API_KEY 已废弃，自 BYOK 起不再对任何用户生效（Key 现在按用户存在 user_preferences 里）")
	}
	// 只提示，不读值：AI 调用不再设超时（K3 这类「始终推理」模型一次分析要几分钟，
	// 掐断等于把已经烧掉的推理 token 白白扔掉）。留着这行会让运维误以为它还能调
	if GetEnv("AI_TIMEOUT_SECONDS", "") != "" {
		log.Println("Warning: AI_TIMEOUT_SECONDS 已废弃，AI 调用不再设超时，这行可以从 .env 里删掉")
	}

	log.Println("Config loaded successfully")
	return nil
}

// envFileOverride 显式指定 .env 路径的环境变量名。
//
// 为什么需要它：`godotenv.Load()` 不带参数时只认进程的 CWD，而部署环境
// （宝塔面板 / systemd）拉起的进程 CWD 往往不是项目目录 —— 于是整个 .env
// **静默失效**，服务用一堆默认值起来、连到错误的数据库，而且不报任何错。
// 这个变量让部署方可以明确指定路径，不依赖启动方式。
const envFileOverride = "ENV_FILE"

// candidateEnvFiles 返回 .env 的候选位置，按优先级排列。
//
// 可执行文件目录排在 CWD 之前：二进制所在目录更接近「应用的家」，
// 而 CWD 只是启动方式的副产物（`go run .` 与从别处调起同一个二进制，
// CWD 完全不同）。CWD 的 .env 作为兜底排在最后。
func candidateEnvFiles() []string {
	exe, err := os.Executable()
	if err != nil {
		// 理论上不会发生。真发生了就只剩 CWD 一个候选 —— 有总比没有好
		return []string{".env"}
	}
	return []string{filepath.Join(filepath.Dir(exe), ".env"), ".env"}
}

// locateEnvFile 定位 .env 文件。
//
// 返回（命中的路径，尝试过的候选，错误）：
//
//   - ENV_FILE 显式指定时**只认它**，读不到就报错。这里刻意不退回候选列表：
//     部署方明明配了路径、服务却按默认值起来，是比启动失败难查得多的问题。
//     值会先裁掉首尾空白 —— 从面板输入框里粘出来的路径常带空白。
//   - 没指定时按候选顺序找。都没有是**合法**的部署方式（全靠环境变量），
//     返回空路径 + 全部候选，由调用方写进启动日志。
func locateEnvFile() (string, []string, error) {
	if override := strings.TrimSpace(os.Getenv(envFileOverride)); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", nil, fmt.Errorf("%s 指向的 %s 读不到: %w", envFileOverride, override, err)
		}
		return override, nil, nil
	}

	candidates := candidateEnvFiles()
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil, nil
		}
	}
	return "", candidates, nil
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
