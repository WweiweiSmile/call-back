package models

import "time"

// 消息类型
const (
	MsgTypeRequestCreated  = "score_request_created"  // 发给场次创建者：有新的存取分申请
	MsgTypeRequestApproved = "score_request_approved" // 发给申请人：申请已通过
	MsgTypeRequestRejected = "score_request_rejected" // 发给申请人：申请已驳回
)

// Message 站内消息表
type Message struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	UserID    uint       `json:"userId" gorm:"not null;index:idx_msg_user_read,priority:1;comment:接收人ID"`
	Type      string     `json:"type" gorm:"size:40;not null;comment:消息类型"`
	Title     string     `json:"title" gorm:"size:255;comment:标题"`
	Content   string     `json:"content" gorm:"type:text;comment:正文"`
	GameID    *uint      `json:"gameId" gorm:"comment:关联场次ID"`
	RequestID *uint      `json:"requestId" gorm:"comment:关联申请单ID"`
	IsRead    bool       `json:"isRead" gorm:"not null;default:false;index:idx_msg_user_read,priority:2;comment:是否已读"`
	ReadAt    *time.Time `json:"readAt" gorm:"comment:已读时间"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "messages"
}
