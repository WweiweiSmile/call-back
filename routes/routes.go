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
	scoreRequestController := controllers.NewScoreRequestController()
	messageController := controllers.NewMessageController()
	reviewController := controllers.NewReviewController()
	tagSuggestionController := controllers.NewTagSuggestionController()
	preferenceController := controllers.NewPreferenceController()

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

			// 用户默认设置。目前只有复盘录入用的盲注默认值，
			// 放在顶层而不是 /reviews 下面：它是账号级偏好，不属于某手牌或某次分析
			preferences := authorized.Group("/preferences")
			{
				preferences.GET("", preferenceController.Get)    // 我的默认设置
				preferences.PUT("", preferenceController.Update) // 保存默认设置
			}

			// 游戏相关路由
			games := authorized.Group("/games")
			{
				games.POST("", gameController.CreateGame)             // 创建游戏
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
				transactions.POST("/deposit", transactionController.Deposit)                          // 存分
				transactions.POST("/withdraw", transactionController.Withdraw)                        // 取分
				transactions.GET("/game/:game_id", transactionController.GetTransactionList)          // 获取游戏交易记录
				transactions.GET("/balance/:game_id", transactionController.GetUserBalance)           // 获取用户余额
				transactions.GET("/participants/:game_id", transactionController.GetGameParticipants) // 获取游戏参与者（含余额）
			}

			// 存取分申请（普通参与者发起，场次创建者审核）
			scoreRequests := authorized.Group("/score-requests")
			{
				scoreRequests.POST("", scoreRequestController.Create)              // 提交申请
				scoreRequests.GET("", scoreRequestController.GetList)              // 申请列表（scope=mine|review）
				scoreRequests.POST("/:id/approve", scoreRequestController.Approve) // 审核通过
				scoreRequests.POST("/:id/reject", scoreRequestController.Reject)   // 审核驳回
				scoreRequests.POST("/:id/cancel", scoreRequestController.Cancel)   // 撤销申请
			}

			// 站内消息（静态路径放在通配路径之前，避免路由冲突）
			messages := authorized.Group("/messages")
			{
				messages.GET("", messageController.GetList)                     // 消息列表
				messages.GET("/unread-count", messageController.GetUnreadCount) // 未读数
				messages.POST("/read-all", messageController.MarkAllRead)       // 全部已读
				messages.POST("/:id/read", messageController.MarkRead)          // 单条已读
				// 静态段与通配段同层共存是允许的（gin 只在新增段以 : 开头时才走冲突分支），
				// /unread-count 会稳定命中静态路由，匹配不到才落到 :id
				messages.GET("/:id", messageController.GetDetail) // 消息详情
			}

			// 复盘（手牌记录 + AI 分析 + 长期记忆）
			// 手牌对每个用户私有，所有接口都按当前用户过滤
			reviews := authorized.Group("/reviews")
			{
				reviews.POST("/hands", reviewController.CreateHand)       // 创建手牌
				reviews.GET("/hands", reviewController.GetHandList)       // 手牌列表
				reviews.GET("/hands/:id", reviewController.GetHand)       // 手牌详情
				reviews.PUT("/hands/:id", reviewController.UpdateHand)    // 更新手牌（整体替换）
				reviews.DELETE("/hands/:id", reviewController.DeleteHand) // 删除手牌

				reviews.POST("/hands/:id/analyze", reviewController.AnalyzeHand)     // 触发 AI 分析（异步）
				reviews.GET("/hands/:id/analyses", reviewController.GetHandAnalyses) // 该手牌的历史分析
				reviews.GET("/analyses/:id", reviewController.GetAnalysis)           // 轮询分析状态与结果

				// 静态路径放在 /hands/:id 之类的通配路径之后不影响匹配，
				// 因为它们的第一段就不同（leak-tags / ai-status vs hands）
				reviews.GET("/leak-tags", reviewController.GetLeakTags) // 漏洞标签字典
				reviews.GET("/ai-status", reviewController.GetAIStatus) // AI 可用状态与剩余额度

				// AI 新标签的入库审批（只有系统管理能操作，在 service 层校验）
				reviews.POST("/tag-suggestions/:id/approve", tagSuggestionController.Approve)
				reviews.POST("/tag-suggestions/:id/reject", tagSuggestionController.Reject)

				// 长期记忆（M4）：画像与漏洞钻取
				reviews.GET("/profile", reviewController.GetProfile)                             // 我的复盘画像
				reviews.POST("/profile/summary/refresh", reviewController.RefreshProfileSummary) // 手动重写总结
				reviews.GET("/insights", reviewController.GetInsights)                           // 某漏洞的历史证据

				// 追问对话（M5）
				reviews.POST("/hands/:id/messages", reviewController.AskQuestion) // 追问，同步返回回复
				reviews.GET("/hands/:id/messages", reviewController.GetMessages)  // 对话历史
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
