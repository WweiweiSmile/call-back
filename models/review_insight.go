package models

import "time"

// 洞察类型
const (
	InsightKindLeak     = "leak"     // 漏洞
	InsightKindStrength = "strength" // 优点
)

// ReviewInsight 复盘洞察 —— 长期记忆的原子。
//
// 每次分析成功后，把 AI 输出的每条 leaks / strengths 落成一行。
// 画像页的「点击某个漏洞 → 看历史上哪几手牌犯的」就是按 tag_code 查这张表。
//
// Evidence 必填且来自 AI 对本手牌的引用：它是防止「AI 随口贴标签」的唯一抓手 ——
// 没有它，画像就退化成一句无据可查的结论。
type ReviewInsight struct {
	ID         uint `json:"id" gorm:"primaryKey"`
	UserID     uint `json:"userId" gorm:"not null;index:idx_ri_user_id_id,priority:1;comment:归属用户，数据隔离依据"`
	HandID     uint `json:"handId" gorm:"not null;index;comment:证据来自哪手牌"`
	AnalysisID uint `json:"analysisId" gorm:"not null;index;comment:来自哪次分析"`

	// Kind leak / strength。strength 没有 tag_code ——
	// 标签字典是「漏洞」字典，拿它描述做对的地方会自相矛盾（见 StrengthItem 注释）
	Kind string `json:"kind" gorm:"size:20;not null;index;comment:leak/strength"`

	TagCode  string `json:"tagCode" gorm:"size:64;index;comment:关联 review_leak_tags.code，strength 为空"`
	Severity int    `json:"severity" gorm:"comment:1~3，strength 恒为 0"`
	Evidence string `json:"evidence" gorm:"type:text;not null;comment:引用本手牌的一句话依据"`

	// CreatedAt 同时也是「最近一次出现时间」的来源
	CreatedAt time.Time `json:"createdAt" gorm:"index:idx_ri_user_id_id,priority:2"`
}

// TableName 指定表名
func (ReviewInsight) TableName() string {
	return "review_insights"
}

// ProfileLeakStat 画像里的一个漏洞条目
type ProfileLeakStat struct {
	TagCode string `json:"tagCode"`
	Name    string `json:"name"`
	// Count 统计窗口内的出现次数
	Count int `json:"count"`
	// HistoricCount 统计窗口之外的累计次数。
	//
	// 用途是区分"已经改掉的毛病"：Count 为 0 而这个数很大，说明以前常犯、
	// 最近这些手没再出现 —— 那是进步，不是数据缺失。没有它的话，窗口一收紧，
	// 老毛病就无声消失了，用户分不清是改掉了还是统计漏了
	HistoricCount int `json:"historicCount"`
	// LastSeenAt 最近一次出现的时间（YYYY-MM-DD），含窗口之外的历史
	LastSeenAt string `json:"lastSeenAt"`
	// AvgSeverity 平均严重度，保留一位小数由前端处理
	AvgSeverity float64 `json:"avgSeverity"`
	// TopEvidence 最近几条证据，供画像页直接展示与钻取
	TopEvidence []string `json:"topEvidence"`
}

// ProfileStrengthItem 画像里的一条优点。
//
// 刻意不做聚合计数：优点没有标签，按文本聚合会得到一堆近似重复的字符串，
// 反而看不清。保留最近若干条即可，作用是提醒用户「你做对过这些」。
type ProfileStrengthItem struct {
	Text string `json:"text"`
	// HandID 便于从优点跳回那手牌
	HandID uint   `json:"handId"`
	Date   string `json:"date"`
}

// ReviewProfile 用户复盘画像 —— 长期记忆的载体。
//
// 每个用户一条，滚动更新。它是每次分析时注入提示词的内容来源，
// 也是画像页的数据源。
type ReviewProfile struct {
	UserID uint `json:"userId" gorm:"primaryKey;comment:一个用户一条"`

	// HandsReviewed 已完成分析的手牌数
	HandsReviewed int `json:"handsReviewed" gorm:"comment:已分析的手牌数"`

	Leaks     []ProfileLeakStat     `json:"leaks" gorm:"serializer:json;type:json;comment:漏洞排行"`
	Strengths []ProfileStrengthItem `json:"strengths" gorm:"serializer:json;type:json;comment:最近的优点"`

	// Summary 一段自然语言总结，≤500 字，由 AI 增量重写
	Summary string `json:"summary" gorm:"type:text;comment:AI 增量重写的阶段总结"`

	// SummaryVersion 每次重写 +1，便于回溯「这条总结是哪一版」
	SummaryVersion int `json:"summaryVersion" gorm:"comment:总结版本号"`

	// LastSummaryAt 上次重写总结的时间。为空表示从未重写过
	LastSummaryAt *time.Time `json:"lastSummaryAt,omitempty" gorm:"comment:上次重写总结的时间"`

	// LastSummaryInsightID 上次重写总结时已纳入的最大洞察 ID。
	// 用 ID 水位线而不是「上次的洞察总数」来判断新增量：
	// 前者不受删除、重跑等影响，语义更稳
	LastSummaryInsightID uint `json:"-" gorm:"comment:上次重写总结时纳入的最大洞察ID"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (ReviewProfile) TableName() string {
	return "review_profiles"
}
