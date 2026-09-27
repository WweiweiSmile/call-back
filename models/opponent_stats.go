package models

// 对手画像的量化统计结构。
//
// 放在 models 而不是 services：这些是要跨层传到 dto 的响应形状，而 dto 不能
// 反向 import services（services 已经 import 了 dto）。同类的先例见
// review_analysis.go 里的 AnalysisResult、review_insight.go 里的 ProfileLeakStat。
//
// 算它们的是 services.ComputeOpponentStats（纯函数，可单测）。

// OpponentPositionStat 按位置分层的翻前统计。
//
// **必须分层**：他在 BTN 的加注率与在 UTG 的加注率是两个数，
// 合成一个"平均加注率"会直接导出错误的剥削方案（把偷盲频率当成开池范围）
type OpponentPositionStat struct {
	Position string `json:"position"`
	Hands    int    `json:"hands"`
	// Vpip 主动投入筹码的手数（跟注或加注）。盲注不算 —— 盲注是记录里的独立列，
	// 不进行动序列，所以大盲在无人加注时过牌不会被误算成入池
	Vpip int `json:"vpip"`
	// Pfr 主动加注（raise/allin）的手数
	Pfr int `json:"pfr"`
	// ThreeBet 他加注时前面已经有人加过注的手数
	ThreeBet int `json:"threeBet"`
	// FacedRaise 他行动前已有人加注的手数（分母）
	FacedRaise int `json:"facedRaise"`
	// FoldToRaise 其中他弃牌的手数
	FoldToRaise int `json:"foldToRaise"`
}

// OpponentFacingStat 他面对我下注时的反应分布
type OpponentFacingStat struct {
	Fold  int `json:"fold"`
	Call  int `json:"call"`
	Raise int `json:"raise"`
}

// OpponentSizingBucket 下注尺度分档（相对该街起始底池）
type OpponentSizingBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// OpponentShowdownItem 一手"看到了他底牌"的手牌。
//
// 这份数据有**摊牌偏差**：只有打到摊牌（或他主动亮牌）的手才看得到底牌，
// 而打到摊牌本身就意味着他的牌不弱。所以它算不出"诈唬率"，
// 只能回答"已知的这几手里他亮出的牌力是什么"——提示词里必须这么声明
type OpponentShowdownItem struct {
	HandID   uint   `json:"handId"`
	Cards    string `json:"cards"`
	Position string `json:"position"`
	Board    string `json:"board"`
	// Made / Tier 由 utils.EvaluateHand 算出。**刻意在 Go 侧算**：
	// 让模型自己看牌面判成牌等级，它会算错，而错了没人看得出来。
	// 空串 = 公牌不足 3 张，这手无法评估
	Made string `json:"made"`
	Tier string `json:"tier"`
	// Aggressive 他在这手牌的翻后主动下过注或加过注
	Aggressive bool `json:"aggressive"`
	// Result 我方结果。只用于页面展示 —— **不能反推他的牌**，
	// "我这手输了"不等于"他那手是强牌"
	Result string `json:"result"`
}

// OpponentPostflopStat 翻后统计
type OpponentPostflopStat struct {
	// FlopHands 他进了翻牌的手数（翻牌里记到了他的行动）
	FlopHands int `json:"flopHands"`
	// CbetOpportunity 他是翻前最后一个加注者、且翻牌有他行动的手数（分母）
	CbetOpportunity int `json:"cbetOpportunity"`
	// CbetMade 其中他在翻牌第一个动作就是下注的手数
	CbetMade int `json:"cbetMade"`
	// TurnBarrelOpportunity 他翻牌下注、并且转牌还有他行动的手数。
	// "转牌还有行动"本身就说明翻牌那一枪被跟注了（全弃牌就不会有转牌）
	TurnBarrelOpportunity int `json:"turnBarrelOpportunity"`
	// TurnBarrelMade 其中他在转牌继续下注的手数 —— 用来识别"只有一发子弹"型
	TurnBarrelMade int `json:"turnBarrelMade"`
	// FacingHeroBet 我下注或加注后，他的下一个动作
	FacingHeroBet OpponentFacingStat `json:"facingHeroBet"`
	// CheckedToOpportunity 我在他之前过牌、轮到他说话的手数（分母）
	CheckedToOpportunity int `json:"checkedToOpportunity"`
	// BetWhenCheckedTo 其中他下注的手数
	BetWhenCheckedTo int `json:"betWhenCheckedTo"`
}

// OpponentStats 对手的量化画像。
//
// 这里每一项都是**事实**：由 Go 从手牌记录里数出来，模型只负责解释。
// 让模型自己数"12 手牌里他加注几次"是不可靠的 —— 五十多个动作它数错一个，
// 画像就是错的，而用户完全无从察觉。
type OpponentStats struct {
	Name string `json:"name"`
	// Hands 可归属的手牌数。老手牌的行动记在聚合角色 "villain" 上，
	// 认不出是哪个对手，不计入 —— 数字偏小是预期内的，不是 bug
	Hands int `json:"hands"`
	// KnownCardsHands 其中记了他底牌的手数
	KnownCardsHands int `json:"knownCardsHands"`
	// Positions 按位置分层，出现次数从多到少
	Positions []OpponentPositionStat `json:"positions"`
	Postflop  OpponentPostflopStat   `json:"postflop"`
	// Sizing 只统计 bet，不含 raise。
	//
	// raise 记录的是"加到多少"，拿它除以底池得到的是"加注到几倍池"，
	// 与下注尺度不是同一个量，混进同一张分布表会让分档失真
	Sizing []OpponentSizingBucket `json:"sizing"`
	// Showdown 已知他底牌的逐手明细（摊牌偏差样本，见 OpponentShowdownItem）
	Showdown []OpponentShowdownItem `json:"showdown"`
	// ThinSample 总手数不足 OpponentThinSampleHands。提示词据此强制降级
	ThinSample bool `json:"thinSample"`
	// FirstHandAt / LastHandAt 统计窗口，格式 YYYY-MM-DD。
	// 必须写进提示词：模型看到"出现 3 次"时得知道这是 12 手里的 3 次还是 200 手里的 3 次
	FirstHandAt string `json:"firstHandAt"`
	LastHandAt  string `json:"lastHandAt"`
}
