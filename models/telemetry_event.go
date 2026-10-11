package models

import "time"

// TelemetryEvent 埋点事件（轻量行为日志）。
//
// ⚠️ 这张表的模型在 **call-auth 与 call-back 各有一份** —— 两个仓库是独立的 Go 模块，
// 不共享代码。改这里的表名或字段，必须同步改另一份，否则两个服务的 AutoMigrate
// 会对同一张表产生不同理解（先启动的那个建表，后启动的尝试 ALTER）
//
// 只记录、不拦截。它的用途是**看信号**：谁在换账号、消耗来自哪个 IP。
// 要做风控请另做限流 —— 这张表不参与任何请求的放行判断
type TelemetryEvent struct {
	ID uint `json:"id" gorm:"primaryKey"`

	// Event 事件名。取值见 telemetry 包的常量，**别在调用处硬编码字符串** ——
	// 拼错不会报错，只会让统计静默少一块
	Event string `json:"event" gorm:"size:64;not null;index:idx_tel_event_created,priority:1"`

	// UserID 事件归属。0 表示事件发生时还没有用户（如注册失败、未登录请求）
	UserID uint `json:"userId" gorm:"not null;default:0;index"`

	// IP 客户端 IP，取 gin 的 ClientIP()。
	// ⚠️ 它在反向代理后取决于 gin 的 TrustedProxies 配置（TRUSTED_PROXIES）：
	// 配对了才是真实客户端，配错可能就是代理的地址。所以再看下面那列 RemoteIP
	IP string `json:"ip" gorm:"size:64;not null;default:'';index"`

	// RemoteIP TCP 对端的地址 —— **内核给的，不可伪造**。
	//
	// 它是 IP 那列的保险丝：X-Forwarded-For 配错时，至少能靠它看出
	// "这批请求到底是不是来自同一个地方"。代价是反代之后它是代理的地址
	RemoteIP string `json:"remoteIp" gorm:"size:64;not null;default:'';index"`

	// UserAgent 原始 UA，按 rune 截到 512（列宽上限）
	UserAgent string `json:"userAgent" gorm:"size:512;not null;default:''"`

	// Extra 事件专属字段，如 {"kind":"analysis","cost":3}。没有就不写（NULL）
	Extra map[string]any `json:"extra,omitempty" gorm:"serializer:json;type:json;comment:事件专属字段"`

	CreatedAt time.Time `json:"createdAt" gorm:"index:idx_tel_event_created,priority:2"`
}

// TableName 表名
func (TelemetryEvent) TableName() string {
	return "telemetry_events"
}
