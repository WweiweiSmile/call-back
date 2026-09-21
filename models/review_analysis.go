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
//
// v1.1：手牌块增加"桌型"一行，系统提示词补充"位置含义随人数变化"。
// v1.0 的分析结论是在不知道人数的情况下给出的，与 v1.1 不可直接比较。
//
// v1.2（M7.1）：对手块列出全部对手（带名字与位置），行动序列里的对手写成
// "老王 (CO)" 而不是笼统的 "Villain"。老手牌的输出与 v1.1 逐字一致，所以
// 跨版本的结论仍然可比 —— 变的只是新录入手牌的对手信息颗粒度
//
// v2.0：系统提示词注入《小绿皮书》方法论（services/coach_methodology.go），
// 追问对话共用同一套口径；输出 Schema 增加 opponentRead 与 actionAdvice 两个
// 字段。这是**不向后兼容**的一次变更：v2.0 之前的分析结果里这两个字段为空，
// 前端必须容忍缺失（按可选字段处理），跨版本的结论也不再直接可比。
const CurrentPromptVersion = "v3.0-skills"

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

	// OpponentRead 对手形象与手牌范围推断。
	//
	// 这是 M7 之后新增的分析维度：让教练先"读人"再点评，而不是直接讲这手牌
	// 该怎么打。指针 + omitempty：v2.0 之前的存量分析没有这个字段，用零值
	// 空结构体会让前端无法区分"没推断"和"推断为空"
	OpponentRead *OpponentRead `json:"opponentRead,omitempty"`

	// ActionAdvice 逐街的行动建议（下注/加注/过牌/弃牌 + 尺度）。
	//
	// 与 Alternatives 的区别：Alternatives 是"事后看还有哪些线路"，
	// ActionAdvice 是"当时就该这么打"的正面答案，带具体尺度数字
	ActionAdvice []ActionAdviceItem `json:"actionAdvice,omitempty"`
}

// 对手形象的五格分类。与提示词里的枚举一一对应
const (
	ProfileLoosePassive    = "loosePassive"    // 松弱，跟注站
	ProfileTightPassive    = "tightPassive"    // 紧弱，岩石
	ProfileLooseAggressive = "looseAggressive" // 松凶，LAG
	ProfileTightAggressive = "tightAggressive" // 紧凶，TAG
	// ProfileUnknown 样本不足时的兜底。刻意用它而不是留空字符串：
	// "不知道"本身是一个结论，前端要显式展示出来提醒用户补信息
	ProfileUnknown = "unknown"
)

// OpponentRead 对手形象与范围推断
type OpponentRead struct {
	// Profile 五格形象之一，取值见 Profile* 常量
	Profile string `json:"profile"`

	// ProfileReason 归类的依据，必须引用本手牌里对手的实际行动
	ProfileReason string `json:"profileReason"`

	// Streets 逐街的范围变化
	Streets []RangeStreetItem `json:"streets"`

	// Conclusion 范围倾向的量级结论。
	// 刻意用文字而不是结构化概率：模型给不出可靠的概率分布，
	// 强制它填数字只会产出一堆精确的假数据
	Conclusion string `json:"conclusion"`
}

// RangeStreetItem 单条街的范围变化
type RangeStreetItem struct {
	Street string `json:"street"`
	// Action 对手在这一街做了什么，用于把范围变化锚定到具体动作
	Action string `json:"action"`
	// RangeKept 这个行动保留了他范围里的哪些牌
	RangeKept string `json:"rangeKept"`
	// RangeDropped 去掉了哪些牌
	RangeDropped string `json:"rangeDropped"`
}

// ActionAdviceItem 单条街的行动建议
type ActionAdviceItem struct {
	Street string `json:"street"`
	// Action 建议的动作：bet / raise / check / fold
	Action string `json:"action"`
	// Sizing 具体尺度，如 "1/2池(12BB)"。提示词明确禁止没有数字的表述
	Sizing string `json:"sizing"`
	// Reason 依据哪条原则
	Reason string `json:"reason"`
	// TargetProfile 针对哪个形象、利用哪个倾向
	TargetProfile string `json:"targetProfile"`
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
