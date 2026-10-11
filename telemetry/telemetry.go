// Package telemetry 轻量埋点：把"谁在什么时候做了什么"记一行，用于事后看信号。
//
// 设计取向（三条，都很要紧）：
//
//  1. **只记，不拦。** 它不参与任何请求的放行判断，不做风控。写失败也只是少一条
//     记录，绝不改变请求的结果 —— 埋点把接口搞挂是最不能接受的事故
//  2. **调用处用常量，别硬编码字符串。** 事件名拼错不会编译报错，只会让统计
//     静默少一块，事后极难发现
//  3. **同步写、失败只记日志。** 不用 goroutine 队列：单次 insert 的代价远小于
//     一个可能无界增长的队列，而这个量级（每用户每次操作一条）根本不需要异步
//
// ⚠️ **IP 的准确性依赖部署配置**，用之前先读这一段：
//   - `IP` 取 ClientIP()，它是否等于真实客户端**完全取决于 `TRUSTED_PROXIES`**
//     （见 config.Config.TrustedProxies）。配错的话它可能是代理地址，或者被
//     客户端伪造的 X-Forwarded-For
//   - `RemoteIP` 取 TCP 对端，**不可伪造**，但反代后是代理的地址
//   - 所以分析"同 IP 多账号"时，**两列一起看**：IP 是猜测，RemoteIP 是事实
//   - 开发环境的 Vite 代理默认不转发 XFF（`xfwd` 默认关），那时 IP 会是 127.0.0.1
//
// 与 models.TelemetryEvent 的关系：本包负责"怎么取、怎么写"，模型负责"存成什么样"。
// 同一个组件在 call-auth 有一份等价实现（两个仓库不共享代码），**事件名必须一致**。
package telemetry

import (
	"call-go/config"
	"call-go/models"
	"log"

	"github.com/gin-gonic/gin"
)

// 事件名。新增事件时在这里加常量，并同步 call-auth 的同名文件
const (
	// EventUserRegister 注册成功。用于识别"同一 IP 注册了多个账号"
	EventUserRegister = "user.register"
	// EventAICall 一次**真正消耗点数**的 AI 调用。
	// 复用已有结论、命中在途闸、被点数拦下 —— 这些都没消耗，不记
	EventAICall = "ai.call"
)

// ai.call 的消耗类型，写进 Extra["kind"]。与 config 里的点数单价一一对应
const (
	KindAnalysis        = "analysis"
	KindOpponentProfile = "opponent_profile"
	KindProfileSummary  = "profile_summary"
	KindChat            = "chat"
)

// UserAgentMaxLen 与 models.TelemetryEvent.UserAgent 的列宽一致（按 rune 算）
const UserAgentMaxLen = 512

// Record 记一条埋点。best-effort：任何失败都只写日志，不影响调用方
//
// c 允许为 nil（后台任务这类拿不到 HTTP 上下文的场景会传 nil），
// 那时 IP 与 UA 记成空串，userID 与 extra 照常
func Record(c *gin.Context, event string, userID uint, extra map[string]any) {
	if config.DB == nil {
		// 单测或极早期启动阶段：没库就不记，不是错误
		return
	}

	ev := models.TelemetryEvent{
		Event:  event,
		UserID: userID,
		Extra:  extra,
	}
	if c != nil {
		// 两个都记，理由见 models.TelemetryEvent 上那两列的注释：
		// IP 依赖 TrustedProxies 配置（配错就可能是代理地址或被伪造），
		// RemoteIP 是内核给的、不可伪造。互为印证
		ev.IP = c.ClientIP()
		ev.RemoteIP = c.RemoteIP()
		ev.UserAgent = truncateRunes(c.Request.UserAgent(), UserAgentMaxLen)
	}

	if err := config.DB.Create(&ev).Error; err != nil {
		log.Printf("[埋点] 写入失败 event=%s user=%d: %v", event, userID, err)
	}
}

// RecordAICall 记一次消耗了点数的 AI 调用。kind 用上面的 Kind* 常量。
//
// 单价由本函数自己按 kind 查 config，不由调用处传 —— 一是让调用处只写一个
// 字符串常量，二是避免有人手写一个和 config 不一致的数字
func RecordAICall(c *gin.Context, userID uint, kind string) {
	Record(c, EventAICall, userID, map[string]any{
		"kind": kind,
		"cost": costOf(kind),
	})
}

// costOf kind → 点数单价。未知 kind 返回 0（只影响统计里的 cost 列，
// 不会让埋点失败 —— 记下来比记准更重要）
func costOf(kind string) int {
	switch kind {
	case KindAnalysis:
		return config.AICostAnalysis
	case KindOpponentProfile:
		return config.AICostOpponentProfile
	case KindProfileSummary:
		return config.AICostProfileSummary
	case KindChat:
		return config.AICostChat
	}
	return 0
}

// truncateRunes 按 rune 截断。列宽是 512 个字符（VARCHAR(512)），
// 按字节截会把多字节字符切碎存进 utf8mb4 列
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
