package models

import "time"

// 分析任务状态
const (
	AnalysisStatusPending = "pending" // 已入队，等待执行
	AnalysisStatusRunning = "running" // 正在调用模型
	AnalysisStatusDone    = "done"    // 分析完成，Result 可用
	AnalysisStatusFailed  = "failed"  // 失败，ErrorMsg 有原因
)

// 提示词版本。改动提示词契约时同步 +1，便于日后回溯"这条结论是哪版提示词产出的"
const CurrentPromptVersion = "v1.0"

// 结构化分析结果。
//
// 刻意要求模型返回 JSON 而不是一段散文：散文没法入库、没法按漏洞聚合、
// 也没法做"点击某个漏洞看历史上哪几手牌犯的"。这些字段名与提示词里给出的
// Schema 一一对应，改名必须两边一起改。
type AnalysisResult struct {
	// HandSummary 一句话概括这手牌的核心矛盾
	HandSummary string `json:"handSummary"`

	// StreetAnalysis 逐街评价
	StreetAnalysis []StreetAnalysisItem `json:"streetAnalysis"`

	// KeyMistake 最关键的错误。刻意限制最多一个 —— 逼模型分清主次，
	// 避免"每条街都有问题"的和稀泥式点评
	KeyMistake *KeyMistakeItem `json:"keyMistake,omitempty"`

	// Alternatives 替代线路
	Alternatives []AlternativeItem `json:"alternatives"`

	// Leaks 漏洞标签。TagCode 必须取自受控词表
	Leaks []LeakItem `json:"leaks"`

	// Strengths 做得好的地方
	Strengths []StrengthItem `json:"strengths"`

	// SuggestedTags 模型认为需要的新标签，进待审队列，不直接入库
	SuggestedTags []SuggestedTagItem `json:"suggestedTags,omitempty"`

	// Drills 下次的练习建议
	Drills []string `json:"drills"`
}

// StreetAnalysisItem 单条街的评价
type StreetAnalysisItem struct {
	Street  string `json:"street"`
	Verdict string `json:"verdict"` // ok / marginal / mistake
	Comment string `json:"comment"`
}

// KeyMistakeItem 关键错误
type KeyMistakeItem struct {
	Street     string `json:"street"`
	What       string `json:"what"`
	Why        string `json:"why"`
	BetterLine string `json:"betterLine"`
}

// AlternativeItem 替代线路
type AlternativeItem struct {
	Line string `json:"line"`
	Note string `json:"note"`
}

// LeakItem 一个漏洞。
// Evidence 必填：有了它才能做"点击漏洞钻取到具体手牌"，
// 也是防止模型随口贴标签的唯一抓手
type LeakItem struct {
	TagCode  string `json:"tagCode"`
	Severity int    `json:"severity"` // 1~3
	Evidence string `json:"evidence"`
}

// StrengthItem 做得好的地方。
//
// 刻意不给它标签：标签字典是"漏洞"字典，让模型从里面挑一个去描述做对的地方
// 会语义拧巴 —— 真实测试里它把「大盲防守过松」当作优点用了。
//
// 优点本来也不需要聚合（长期记忆聚合的是漏洞），一段引用本手牌动作的文字就够
type StrengthItem struct {
	Text string `json:"text"`
}

// SuggestedTagItem 模型提议的新标签
type SuggestedTagItem struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ReviewAnalysis 一次 AI 分析
//
// 同一手牌可以有多条分析（内容变更后重跑、或用户主动重跑），保留历史便于对比。
type ReviewAnalysis struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	HandID uint `json:"handId" gorm:"not null;index:idx_ra_hand,priority:1;comment:所属手牌"`
	UserID uint `json:"userId" gorm:"not null;index:idx_ra_user_created,priority:1;comment:归属用户，数据隔离依据"`

	Status string `json:"status" gorm:"size:20;not null;default:'pending';index;comment:pending/running/done/failed"`

	Model         string `json:"model" gorm:"size:64;comment:模型标识"`
	PromptVersion string `json:"promptVersion" gorm:"size:20;comment:提示词版本"`

	// ContentHash 触发本次分析时手牌的内容指纹，用于"内容没变就复用上次结果"
	ContentHash string `json:"-" gorm:"size:64;index;comment:触发时的手牌内容指纹"`

	// InputSnapshot 本次实际发给模型的完整输入。
	//
	// 提示词会演进、记忆会累积，半年后看到一条结论想不通"它当时为什么这么说"，
	// 只有这一列能查清。每行几 KB，排障价值远大于存储成本
	InputSnapshot string `json:"-" gorm:"type:mediumtext;comment:实际发送给模型的完整输入"`

	// Result 结构化分析结果，JSON 列
	Result *AnalysisResult `json:"result,omitempty" gorm:"serializer:json;type:json;comment:结构化分析结果"`

	// RawOutput 模型原始返回，排查解析失败时用
	RawOutput string `json:"-" gorm:"type:mediumtext;comment:模型原始返回"`

	TokensIn  int `json:"tokensIn" gorm:"comment:输入 token，用于成本核算"`
	TokensOut int `json:"tokensOut" gorm:"comment:输出 token"`

	ErrorMsg   string `json:"errorMsg,omitempty" gorm:"size:500;comment:失败原因"`
	DurationMs int64  `json:"durationMs" gorm:"comment:耗时(毫秒)"`

	CreatedAt time.Time `json:"createdAt" gorm:"index:idx_ra_user_created,priority:2"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (ReviewAnalysis) TableName() string {
	return "review_analyses"
}
