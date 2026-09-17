package models

import "time"

// 标签建议状态
const (
	TagSuggestionStatusPending  = "pending"  // 待审批
	TagSuggestionStatusApproved = "approved" // 已入库
	TagSuggestionStatusRejected = "rejected" // 已拒绝
)

// ReviewTagSuggestion AI 提议的新漏洞标签待审队列。
//
// 为什么必须有这张表：review_analyses.result 里的 suggestedTags 只是那次分析的
// 副产物，无人认领、无法去重、也无法承载"批到哪一步了"。而标签字典是所有人共用的，
// 模型编的标签一旦直接进去就会污染所有人的画像聚合。
//
// 为什么按 Name 唯一：同一个漏洞模型会用同一个词反复提议（"翻前跟注过宽"在一百手牌里
// 出现一百次）。不去重的话待审列表会被同名条目淹没，管理员要处理一百遍同一件事。
// 去重后 HitCount 反而成了有用的信号——被提议得越频繁，越值得入库。
type ReviewTagSuggestion struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"size:64;uniqueIndex;not null;comment:模型给的标签名，去重键"`
	// Reason 模型自己写的"为什么现有标签覆盖不了"，审批时预填进判定说明
	Reason string `json:"reason" gorm:"size:500;comment:模型给出的理由"`
	Status string `json:"status" gorm:"size:20;not null;default:'pending';index;comment:pending/approved/rejected"`
	// HitCount 被提议的次数。拒绝过的名字再次出现时只累加，不重新回到待审
	HitCount int `json:"hitCount" gorm:"not null;default:1;comment:被提议次数"`

	// 来源：第一次与最近一次提议它的分析。只留 ID 不留外键，分析被删不影响这里
	FirstAnalysisID uint `json:"firstAnalysisId" gorm:"comment:首次提议的分析ID"`
	LastAnalysisID  uint `json:"lastAnalysisId" gorm:"comment:最近提议的分析ID"`

	// 审批信息
	ReviewerID   *uint      `json:"reviewerId" gorm:"comment:审批人ID(系统管理)"`
	ReviewRemark string     `json:"reviewRemark" gorm:"size:255;comment:驳回理由"`
	ReviewedAt   *time.Time `json:"reviewedAt" gorm:"comment:审批时间"`

	// TagID 通过后落进 review_leak_tags 的那条标签
	TagID *uint `json:"tagId" gorm:"comment:通过后生成的标签ID"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (ReviewTagSuggestion) TableName() string {
	return "review_tag_suggestions"
}

// IsPending 是否还在待审
func (s *ReviewTagSuggestion) IsPending() bool {
	return s.Status == TagSuggestionStatusPending
}
