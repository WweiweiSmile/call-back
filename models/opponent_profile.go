package models

import "time"

// 画像置信度。与五格形象一样，都是提示词里写死的枚举
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// OpponentTendency 画像里的一条倾向观察。
type OpponentTendency struct {
	// Aspect 观察的维度：翻前开池 / 面对加注 / 持续下注 / 转牌二次开火 /
	// 面对我的下注 / 下注尺度 / 诈唬倾向
	Aspect string `json:"aspect"`
	// Observation 一句话说清他偏在哪一边
	Observation string `json:"observation"`
	// SampleSize 样本量，形如 "8/12"。**必填且由系统校验**：
	// 没有它，"他 60% 会加注"这句话无法判断是 3 手里的 2 手还是 30 手里的 18 手，
	// 而这两种情况该给出的建议完全相反
	SampleSize string `json:"sampleSize"`
	// Evidence 引用具体手牌里真实发生的动作
	Evidence string `json:"evidence"`
}

// OpponentExploit 一条剥削方案。四个字段缺一不可：
// 少了 Risk，用户就不知道这招什么时候会反噬；少了 Sizing，建议就落不了地
type OpponentExploit struct {
	// Against 针对上面哪一条倾向
	Against string `json:"against"`
	// Adjustment 我具体该怎么做
	Adjustment string `json:"adjustment"`
	// Sizing 带数字的尺度，如"他过牌后我用 2/3 池下注"
	Sizing string `json:"sizing"`
	// Risk 他一旦反制，我承担的代价是什么
	Risk string `json:"risk"`
}

// OpponentProfile 对手画像。
//
// 一个对手一条。与 ReviewProfile（自己的长期画像）是两回事：
//   - ReviewProfile 每次分析手牌后自动滚动更新，回答"我有什么毛病"
//   - 这张表**由用户主动触发**，回答"他是谁、我该怎么打他"
//
// 它的输入是 services.ComputeOpponentStats 算好的事实 + 与他的对抗手牌，
// **不含他也不含我的底牌之外的任何推测**：对手底牌只在摊牌时才有，
// 那部分单独作为摊牌偏差样本喂给模型（见 models.OpponentStats.Showdown）
type OpponentProfile struct {
	// OpponentID 主键。一个对手只有一条画像，重新生成是覆盖而不是追加 ——
	// 留着历史版本会引出"该信哪一版"的问题，而用户看的就是最新那一版
	OpponentID uint `json:"opponentId" gorm:"primaryKey;comment:对手表id"`

	// UserID 冗余一份。对手表本身已经按 user_id 隔离，但画像要独立查、
	// 独立回收中断任务，与其每次 join 一次对手表，不如直接带上
	UserID uint `json:"userId" gorm:"not null;index;comment:归属用户，数据隔离依据"`

	// Profile 五格形象之一，取值见 Profile* 常量。样本不足时是 ProfileUnknown
	Profile string `json:"profile" gorm:"size:32;comment:loosePassive/tightPassive/looseAggressive/tightAggressive/unknown"`
	// Confidence low/medium/high
	Confidence    string `json:"confidence" gorm:"size:16"`
	ProfileReason string `json:"profileReason" gorm:"type:text;comment:归类的依据"`

	Tendencies []OpponentTendency `json:"tendencies" gorm:"serializer:json;type:json;comment:逐条倾向，每条带样本量"`
	Exploits   []OpponentExploit  `json:"exploits" gorm:"serializer:json;type:json;comment:剥削方案"`
	// Unknowns 样本不足、现在还看不出来的维度。**不是空的就行** ——
	// 它同时也是给用户的一份"还缺什么"清单
	Unknowns []string `json:"unknowns" gorm:"serializer:json;type:json"`
	// WatchNext 下次交手要重点记录什么
	WatchNext []string `json:"watchNext" gorm:"serializer:json;type:json"`

	// Summary 一段自然语言总结，像教练当面讲这个人。
	// mediumtext 的理由同 ReviewProfile.Summary：text 的 65535 是**字节**，
	// utf8mb4 下只装得下约 2.1 万字，是个看不见的上限
	Summary string `json:"summary" gorm:"type:mediumtext;comment:AI 写的自然语言总结"`

	// HandsAtGeneration 生成时覆盖的交手手数。
	// 之后又录了新手牌时页面据此提示"画像比你现在的记录旧了"——
	// 没有它，用户只能凭生成时间猜，而时间说明不了样本量变了多少
	HandsAtGeneration int `json:"handsAtGeneration" gorm:"comment:生成时覆盖的手数"`

	// Status 任务状态，取值复用 SummaryStatus* 常量（pending/running/done/failed）。
	// default:'done' 只服务于**存量行迁移**，代码里写状态时必须显式赋值
	Status string `json:"status" gorm:"size:20;not null;default:'done'"`
	// ErrorMsg 失败原因，会被截到列宽
	ErrorMsg string `json:"error,omitempty" gorm:"size:500"`
	// StartedAt 本次任务的开始时间。**判"这条还在跑吗"必须用它，不能用 UpdatedAt** ——
	// 与 ReviewProfile.SummaryStartedAt 同一个理由：任何一次写库都会把 UpdatedAt
	// 顶到当下，窗口就永远不过期了
	StartedAt *time.Time `json:"startedAt,omitempty" gorm:"comment:本次任务开始时间，任务时间窗判据"`
	// LastGeneratedAt 上次成功生成的时间。为空表示从未生成过
	LastGeneratedAt *time.Time `json:"lastGeneratedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (OpponentProfile) TableName() string {
	return "opponent_profiles"
}
