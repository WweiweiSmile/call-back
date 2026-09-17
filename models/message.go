package models

import "time"

// 消息类型
const (
	MsgTypeRequestCreated  = "score_request_created"  // 发给场次创建者：有新的存取分申请
	MsgTypeRequestApproved = "score_request_approved" // 发给申请人：申请已通过
	MsgTypeRequestRejected = "score_request_rejected" // 发给申请人：申请已驳回

	// MsgTypeTagSuggestionPending 发给所有系统管理：AI 提议了字典外的新标签，等审批入库。
	// 标签字典是所有人共用的，所以审批权限只给 admin
	MsgTypeTagSuggestionPending = "tag_suggestion_pending"
)

// 消息分类：决定消息详情页是"出按钮"还是"只读"
const (
	MsgCategoryApproval = "approval" // 需要收件人处理
	MsgCategoryNotice   = "notice"   // 结果告知
)

// MessageCategoryOf 由消息类型派生分类。
//
// 刻意不落库：type 已经是判别式，再加一列就是同一事实存两份，插入时漏写就漂移。
// 新增审批类消息时在这里加一条 case（M2 的标签入库审批加 MsgTypeTagSuggestionPending）。
func MessageCategoryOf(msgType string) string {
	switch msgType {
	case MsgTypeRequestCreated, // 发给场次创建者的待审申请
		MsgTypeTagSuggestionPending: // 发给系统管理的新标签待审
		return MsgCategoryApproval
	default:
		// 包含 approved/rejected：它们已经是结论，不该再给按钮
		return MsgCategoryNotice
	}
}

// Message 站内消息表
type Message struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	UserID    uint   `json:"userId" gorm:"not null;index:idx_msg_user_read,priority:1;comment:接收人ID"`
	Type      string `json:"type" gorm:"size:40;not null;comment:消息类型"`
	Title     string `json:"title" gorm:"size:255;comment:标题"`
	Content   string `json:"content" gorm:"type:text;comment:正文"`
	GameID    *uint  `json:"gameId" gorm:"comment:关联场次ID"`
	RequestID *uint  `json:"requestId" gorm:"comment:关联存取分申请ID"`
	// SuggestionID 关联的标签建议ID。与 RequestID 分开是因为两者语义与状态源都不同，
	// 共用一列会让"可操作性"的判定无从下手
	SuggestionID *uint      `json:"suggestionId" gorm:"comment:关联标签建议ID"`
	IsRead       bool       `json:"isRead" gorm:"not null;default:false;index:idx_msg_user_read,priority:2;comment:是否已读"`
	ReadAt       *time.Time `json:"readAt" gorm:"comment:已读时间"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "messages"
}
