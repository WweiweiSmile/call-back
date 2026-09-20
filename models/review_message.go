package models

import "time"

// 对话角色
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
)

// 消息的处理状态。
//
// 只有 assistant 的占位行会处于 pending/running —— user 行落库即终态。
// 刻意不复用 AnalysisStatus*：那是分析表的状态常量，两者语义不同，
// 借用的结果是看到常量名会以为是分析在跑
const (
	MessageStatusPending = "pending" // 占位行已落库，后台还没开始
	MessageStatusRunning = "running" // 后台正在调模型
	MessageStatusDone    = "done"    // Content 可用
	MessageStatusFailed  = "failed"  // ErrorMsg 有原因
)

// ReviewMessage 复盘追问对话的一条消息。
//
// 追问的上下文 = 系统提示词 + 记忆块 + 手牌块 + 该次分析的结果 + 这段对话历史。
//
// **对话按 AnalysisID 绑定**：一手牌被改过并重新分析后会产生新的一条 analysis，
// 那是一段全新的对话，旧消息不会跟过来 —— 否则用户会看到一段针对已经不存在的那次
// 分析的问答。所以读取一律 `WHERE analysis_id = ?`，不是 hand_id
type ReviewMessage struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	UserID uint `json:"userId" gorm:"not null;index;comment:归属用户，数据隔离依据"`
	// HandID 属于哪手牌。**不参与对话的过滤**，留着是为了溯源，
	// 以及将来按手牌清理时有个依据（改绑到 analysis_id 之前它是查询键）
	HandID uint `json:"handId" gorm:"not null;index:idx_rm_hand_id_id,priority:1"`
	// AnalysisID 基于哪次分析追问，也是对话的绑定维度
	AnalysisID uint `json:"analysisId" gorm:"not null;index"`

	Role string `json:"role" gorm:"size:20;not null;comment:user/assistant"`
	// Content 消息正文。用户输入进提示词，写入前按 HeroThought 同样的口径限长。
	// assistant 占位行在后台答完之前是空串
	Content string `json:"content" gorm:"type:text;not null"`

	// Status 这条消息的处理状态。user 行恒为 done；assistant 行先落 pending，
	// 后台答完写 done（失败写 failed + ErrorMsg），前端据此决定渲染"教练正在想"
	// 还是渲染正文
	//
	// default:'done' 只服务于**存量行迁移**：AutoMigrate 加列时 MySQL 会给老行填
	// done，与"老对话都已完成"的语义一致。代码里创建时**必须显式赋值** ——
	// MySQL 不支持 RETURNING，GORM 不会把数据库填的默认值回读到结构体，
	// 靠 default 只会拿到空串，而空串不是终态，前端会把输入框永久禁用
	Status string `json:"status" gorm:"size:20;not null;default:'done';comment:pending/running/done/failed"`
	// ErrorMsg 失败原因，只有 assistant 行有
	ErrorMsg string `json:"errorMsg,omitempty" gorm:"size:500"`

	// TokensIn/TokensOut 只有 assistant 消息有值，用于成本记账
	TokensIn  int `json:"tokensIn" gorm:"comment:仅 assistant 消息"`
	TokensOut int `json:"tokensOut" gorm:"comment:仅 assistant 消息"`

	// CreatedAt 同时是对话的排序依据，按 id 升序即为时间顺序
	CreatedAt time.Time `json:"createdAt" gorm:"index:idx_rm_hand_id_id,priority:2"`
}

// TableName 指定表名
func (ReviewMessage) TableName() string {
	return "review_messages"
}
