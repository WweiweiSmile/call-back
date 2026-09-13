package routes

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSetupRoutes 路由注册冒烟测试。
// gin 在遇到冲突路由时会在注册阶段 panic，这个测试能在不连数据库的前提下
// 提前发现"服务起不来"这类问题，并校验新接口确实挂上了。
func TestSetupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	SetupRoutes(r) // 路由冲突会在这里 panic

	want := []string{
		"POST /api/v1/score-requests",
		"GET /api/v1/score-requests",
		"POST /api/v1/score-requests/:id/approve",
		"POST /api/v1/score-requests/:id/reject",
		"POST /api/v1/score-requests/:id/cancel",
		"GET /api/v1/messages",
		"GET /api/v1/messages/unread-count",
		"POST /api/v1/messages/:id/read",
		"POST /api/v1/messages/read-all",
	}

	got := make(map[string]bool)
	for _, ri := range r.Routes() {
		got[ri.Method+" "+ri.Path] = true
	}

	for _, w := range want {
		if !got[w] {
			t.Errorf("路由未注册: %s", w)
		}
	}
}
