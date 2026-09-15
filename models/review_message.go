package models

import "time"

// 对话角色
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
)

// ReviewMessage 复盘追问对话的一条消息。
//
// 追问的上下文 = 系统提示词 + 记忆块 + 手牌块 + 该次分析的结果 + 这段对话历史，
// 所以每条消息要记住它是基于哪次分析追问的 —— 手牌被改过并重新分析后，
// 旧对话仍然对应旧结论，不会张冠李戴。
type ReviewMessage struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	UserID uint `json:"userId" gorm:"not null;index;comment:归属用户，数据隔离依据"`
	HandID uint `json:"handId" gorm:"not null;index:idx_rm_hand_id_id,priority:1"`
	// AnalysisID 基于哪次分析追问
	AnalysisID uint `json:"analysisId" gorm:"not null;index"`

	Role string `json:"role" gorm:"size:20;not null;comment:user/assistant"`
	// Content 消息正文。用户输入进提示词，写入前按 HeroThought 同样的口径限长
	Content string `json:"content" gorm:"type:text;not null"`

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
