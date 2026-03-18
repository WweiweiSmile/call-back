package config

import (
	"log"
	"os"

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
}

var AppConfig *Config

func LoadConfig() error {
	// 尝试加载 .env 文件（如果存在）
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	AppConfig = &Config{
		DBHost:     GetEnv("DB_HOST", "localhost"),
		DBPort:     GetEnv("DB_PORT", "3306"),
		DBUser:     GetEnv("DB_USER", "root"),
		DBPassword: GetEnv("DB_PASSWORD", ""),
		DBName:     GetEnv("DB_NAME", "call_game"),
		ServerPort: GetEnv("SERVER_PORT", "8080"),
		JWTSecret:  GetEnv("JWT_SECRET", "call-game-secret-key-2026"),
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
