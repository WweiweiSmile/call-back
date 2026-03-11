package main

import (
	"call-go/config"
	"call-go/models"
	"call-go/routes"
	"call-go/utils"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	if err := config.LoadConfig(); err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// 设置JWT密钥
	utils.SetJWTSecret(config.AppConfig.JWTSecret)

	// 初始化数据库
	if err := config.InitDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// 自动迁移数据库表
	if err := config.DB.AutoMigrate(
		&models.User{},
		&models.Game{},
		&models.UserGame{},
		&models.Transaction{},
		&models.UserBalance{},
	); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// 插入测试数据
	seedTestData()

	// 设置 Gin
	r := gin.Default()

	// 设置路由
	routes.SetupRoutes(r)

	// 启动服务器
	port := config.AppConfig.ServerPort
	log.Printf("Server starting on :%s", port)
	log.Printf("API docs: http://localhost:%s/api/v1", port)
	log.Printf("Health check: http://localhost:%s/health", port)
	log.Println("Test accounts: testuser1/123456, testuser2/123456, testuser3/123456")
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

func seedTestData() {
	// 检查是否已经有测试数据
	var userCount int64
	config.DB.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		log.Println("Test data already exists, skipping...")
		return
	}

	log.Println("Seeding test data...")

	// 创建测试用户
	users := []models.User{
		{Username: "testuser1", Nickname: "当前用户", Status: "active"},
		{Username: "testuser2", Nickname: "张三", Status: "active"},
		{Username: "testuser3", Nickname: "李四", Status: "active"},
	}
	for i := range users {
		users[i].SetPassword("123456")
		config.DB.Create(&users[i])
	}

	// 创建测试游戏
	now := time.Now()
	games := []models.Game{
		{
			Name:        "周末扑克局",
			Description: "每周六晚的固定局",
			CreatorID:   users[1].ID,
			Status:      "ongoing",
			PlayerCount: 12,
			CreatedAt:   now,
		},
		{
			Name:        "麻将友谊赛",
			Description: "我创建的游戏",
			CreatorID:   users[0].ID,
			Status:      "ongoing",
			PlayerCount: 5,
			CreatedAt:   now,
		},
		{
			Name:        "新手练习场",
			Description: "",
			CreatorID:   users[2].ID,
			Status:      "pending",
			PlayerCount: 3,
			CreatedAt:   now,
		},
	}
	for i := range games {
		config.DB.Create(&games[i])
	}

	// 创建用户-游戏关联
	userGames := []models.UserGame{
		{UserID: users[0].ID, GameID: games[0].ID, JoinedAt: now, Status: "active"},
		{UserID: users[1].ID, GameID: games[0].ID, JoinedAt: now, Status: "active"},
		{UserID: users[0].ID, GameID: games[1].ID, JoinedAt: now, Status: "active"},
	}
	for i := range userGames {
		config.DB.Create(&userGames[i])
	}

	// 创建用户余额
	userBalances := []models.UserBalance{
		{
			UserID:         users[0].ID,
			GameID:         games[0].ID,
			TotalDeposit:   15000,
			TotalWithdraw:  12000,
			CurrentBalance: 3000,
			BalanceStatus:  "unbalanced",
			LastTransTime:  &now,
		},
		{
			UserID:         users[1].ID,
			GameID:         games[0].ID,
			TotalDeposit:   20000,
			TotalWithdraw:  18000,
			CurrentBalance: 2000,
			BalanceStatus:  "unbalanced",
			LastTransTime:  &now,
		},
		{
			UserID:         users[0].ID,
			GameID:         games[1].ID,
			TotalDeposit:   5000,
			TotalWithdraw:  5000,
			CurrentBalance: 0,
			BalanceStatus:  "balanced",
			LastTransTime:  &now,
		},
	}
	for i := range userBalances {
		config.DB.Create(&userBalances[i])
	}

	// 创建交易记录
	transactions := []models.Transaction{
		{
			UserID:       users[0].ID,
			GameID:       games[0].ID,
			OperatorID:   users[0].ID,
			OperatorType: "self",
			TransType:    "deposit",
			Amount:       5000,
			BalanceAfter: 3000,
			CreatedAt:    now,
		},
		{
			UserID:       users[0].ID,
			GameID:       games[0].ID,
			OperatorID:   users[1].ID,
			OperatorType: "proxy",
			TransType:    "withdraw",
			Amount:       2000,
			BalanceAfter: -2000,
			Remark:       "创建者操作",
			CreatedAt:    now.Add(-5 * time.Minute),
		},
		{
			UserID:       users[1].ID,
			GameID:       games[0].ID,
			OperatorID:   users[1].ID,
			OperatorType: "self",
			TransType:    "deposit",
			Amount:       3000,
			BalanceAfter: 2000,
			CreatedAt:    now.Add(-10 * time.Minute),
		},
	}
	for i := range transactions {
		config.DB.Create(&transactions[i])
	}

	log.Println("Test data seeded successfully!")
	log.Println("Test accounts:")
	log.Println("  - testuser1 / 123456")
	log.Println("  - testuser2 / 123456")
	log.Println("  - testuser3 / 123456")
}
