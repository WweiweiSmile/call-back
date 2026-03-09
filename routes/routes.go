package routes

import (
	"call-go/controllers"
	"call-go/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine) {
	// 初始化控制器
	authController := controllers.NewAuthController()
	gameController := controllers.NewGameController()
	transactionController := controllers.NewTransactionController()

	// API 路由组
	api := r.Group("/api/v1")
	{
		// 认证相关（不需要登录）
		auth := api.Group("/auth")
		{
			auth.POST("/register", authController.Register) // 注册
			auth.POST("/login", authController.Login)       // 登录
		}

		// 需要认证的路由
		authorized := api.Group("")
		authorized.Use(middleware.AuthMiddleware())
		{
			// 用户信息
			authorized.GET("/auth/user", authController.GetUserInfo)

			// 游戏相关路由
			games := authorized.Group("/games")
			{
				games.POST("", gameController.CreateGame)           // 创建游戏
				games.GET("", gameController.GetGameList)             // 获取游戏列表
				games.GET("/my", gameController.GetMyGames)           // 获取我的游戏
				games.GET("/created", gameController.GetCreatedGames) // 获取我创建的游戏
				games.GET("/:id", gameController.GetGame)             // 获取游戏详情
				games.POST("/join", gameController.JoinGame)          // 加入游戏
				games.POST("/:id/leave", gameController.LeaveGame)    // 退出游戏
				games.POST("/:id/end", gameController.EndGame)        // 结束游戏
			}

			// 交易相关路由
			transactions := authorized.Group("/transactions")
			{
				transactions.POST("/deposit", transactionController.Deposit)                              // 存分
				transactions.POST("/withdraw", transactionController.Withdraw)                            // 取分
				transactions.GET("/game/:game_id", transactionController.GetTransactionList)              // 获取游戏交易记录
				transactions.GET("/balance/:game_id", transactionController.GetUserBalance)               // 获取用户余额
				transactions.GET("/participants/:game_id", transactionController.GetGameParticipants)     // 获取游戏参与者（含余额）
			}
		}
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})
}
