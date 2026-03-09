package config

import (
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	// 先连接到 MySQL 服务器（不指定数据库），创建数据库
	serverDSN := "root:Qw13101192533@tcp(localhost:3306)/?charset=utf8mb4&parseTime=True&loc=Local"
	
	tempDB, err := gorm.Open(mysql.Open(serverDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL server: %w", err)
	}

	// 创建数据库（如果不存在）
	log.Println("Creating database if not exists...")
	err = tempDB.Exec("CREATE DATABASE IF NOT EXISTS call_game DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	// 现在连接到具体的数据库
	dsn := "root:Qw13101192533@tcp(localhost:3306)/call_game?charset=utf8mb4&parseTime=True&loc=Local"
	
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Database connected successfully")
	return nil
}
