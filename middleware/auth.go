package middleware

import (
	"call-go/utils"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware JWT 认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Authorization header 中获取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未登录",
			})
			c.Abort()
			return
		}

		// 检查 Bearer 格式
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证格式错误",
			})
			c.Abort()
			return
		}

		// 解析 token
		claims, err := utils.ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证已过期，请重新登录",
			})
			c.Abort()
			return
		}

		// 将用户信息存储到 context 中
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)

		c.Next()
	}
}

// GetUserID 从 context 中获取用户ID
func GetUserID(c *gin.Context) uint {
	userID, _ := c.Get("user_id")
	if uid, ok := userID.(uint); ok {
		return uid
	}
	return 0
}

// GetUsername 从 context 中获取用户名
func GetUsername(c *gin.Context) string {
	username, _ := c.Get("username")
	if name, ok := username.(string); ok {
		return name
	}
	return ""
}
