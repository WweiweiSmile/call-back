package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"call-go/utils"
)

// context 里的键名。与 call-auth 保持一致
const (
	CtxUserID   = "user_id"
	CtxUsername = "username"
	CtxRole     = "role"
)

// AuthMiddleware JWT 认证中间件。
//
// 验签是拿本地公钥做的**纯计算**，认证中心不在请求路径上 —— 每个请求都不会
// 去连它（《认证中心设计文档》§3.4）
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 放行 OPTIONS 请求
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortUnauthorized(c, "未登录")
			return
		}

		// 检查 Bearer 格式。用 EqualFold：RFC 7235 说 scheme 大小写不敏感
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && strings.EqualFold(parts[0], "Bearer")) {
			abortUnauthorized(c, "认证格式错误")
			return
		}

		claims, err := utils.ParseToken(parts[1])
		if err != nil {
			abortUnauthorized(c, "认证已过期，请重新登录")
			return
		}

		// 用户 id 在 sub 里（JWT 规范要求 sub 是字符串），要转一下。
		// 转不出来说明 token 结构不对，当无效处理 —— 不能默认成 0 号用户
		userID, err := claims.UserID()
		if err != nil {
			abortUnauthorized(c, "认证已过期，请重新登录")
			return
		}

		c.Set(CtxUserID, userID)
		c.Set(CtxUsername, claims.Username)
		// 角色直接来自 token 的 claims，不查库。代价是改角色后要等 token
		// 过期（最长 15 分钟）才生效
		c.Set(CtxRole, claims.Role)

		c.Next()
	}
}

func abortUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"code":    401,
		"message": message,
	})
	c.Abort()
}

// GetUserID 从 context 中获取用户ID
func GetUserID(c *gin.Context) uint {
	userID, _ := c.Get(CtxUserID)
	if uid, ok := userID.(uint); ok {
		return uid
	}
	return 0
}

// GetUsername 从 context 中获取用户名
func GetUsername(c *gin.Context) string {
	username, _ := c.Get(CtxUsername)
	if name, ok := username.(string); ok {
		return name
	}
	return ""
}

// IsAdmin 当前请求是不是管理员发起的。
//
// 原先各处是查库看 user.Role，现在读 token 里的 claims —— 少一次查询，
// 代价是改角色后要等 token 过期才生效
func IsAdmin(c *gin.Context) bool {
	role, _ := c.Get(CtxRole)
	if r, ok := role.(string); ok {
		return r == "admin"
	}
	return false
}
