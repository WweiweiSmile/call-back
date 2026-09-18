package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// 街道
const (
	StreetPreflop = "preflop"
	StreetFlop    = "flop"
	StreetTurn    = "turn"
	StreetRiver   = "river"
)

// 行动类型
const (
	ActionFold  = "fold"
	ActionCheck = "check"
	ActionCall  = "call"
	ActionBet   = "bet"
	ActionRaise = "raise"
	ActionAllin = "allin"
)

// 行动者。other 表示不关注的其他玩家，聚合成一个角色即可
const (
	ActorHero    = "hero"
	ActorVillain = "villain"
	ActorOther   = "other"
)

// 位置。取值随人数（TableSize）变化，合法组合见 utils.PositionsForTableSize。
//
// 原有的 MP 已废弃：它既可能指 LJ 也可能指 HJ，两种读法对应的翻前范围差别很大，
// 交给模型分析时是实打实的歧义。存量数据已迁移到 HJ。
const (
	PositionSB   = "SB"
	PositionBB   = "BB"
	PositionUTG  = "UTG"
	PositionUTG1 = "UTG+1"
	PositionUTG2 = "UTG+2"
	PositionLJ   = "LJ"
	PositionHJ   = "HJ"
	PositionCO   = "CO"
	PositionBTN  = "BTN"
)

// 人数取值范围
const (
	MinTableSize = 2
	MaxTableSize = 9
	// DefaultTableSize 未填写时按满员桌处理
	DefaultTableSize = 9
)

// 底池类型
const (
	PotTypeHU    = "hu"    // 单挑
	PotTypeMulti = "multi" // 多人池
)

// 结果
const (
	ResultWin     = "win"
	ResultLose    = "lose"
	ResultFold    = "fold"
	ResultUnknown = "unknown"
)

// 分析状态。M1/M2 不使用，预留给 M3 的 AI 分析
const (
	AnalyzeStatusNone    = "none"
	AnalyzeStatusPending = "pending"
	AnalyzeStatusDone    = "done"
	AnalyzeStatusFailed  = "failed"
)

// StreetAction 一条行动记录
type StreetAction struct {
	Actor    string   `json:"actor"`              // hero / villain / other
	Action   string   `json:"action"`             // fold / check / call / bet / raise / allin
	AmountBB *float64 `json:"amountBb,omitempty"` // bet/raise 的金额（BB）；raise 记录"加到多少"
}

// StreetRecord 一条街的完整行动序列
type StreetRecord struct {
	Street     string         `json:"street"`               // preflop / flop / turn / river
	Actions    []StreetAction `json:"actions"`              // 按发生顺序
	PotStartBB *float64       `json:"potStartBb,omitempty"` // 该街开始时的底池
}

// VillainInfo 对手信息。
//
// M7.1 起每个对手都是具名的实体（M7.1 之前只要求标出关键对手）。两个新字段
// **必须带 omitempty**：老手牌的 Villains 序列化结果要一字不变，否则
// ComputeHandHash 会认为内容变了，把已有分析状态重置、还得多花一次模型额度。
type VillainInfo struct {
	Position string   `json:"position"`
	StackBB  *float64 `json:"stackBb,omitempty"`
	IsKey    bool     `json:"isKey,omitempty"` // 是否为关键对手，每手最多一个
	// OpponentID 对手表 id，由后端按 Name 解析后写入，客户端传的值不参与写入
	OpponentID uint `json:"opponentId,omitempty"`
	// Name 对手的称呼，进提示词。为空表示 M7.1 之前的老数据（或只记了位置没记名字）
	Name string `json:"name,omitempty"`
}

// villainPositions 本手牌记录过的对手位置集合
func (h *ReviewHand) villainPositions() map[string]bool {
	positions := make(map[string]bool, len(h.Villains))
	for i := range h.Villains {
		if h.Villains[i].Position != "" {
			positions[h.Villains[i].Position] = true
		}
	}
	return positions
}

// IsKnownActor 这个 actor 值能不能落到本手牌的某个人头上。
//
// M7.1 起对手按位置记录（"CO"），所以位置必须真的在这手牌的对手列表里 ——
// 否则会存下一份指不到人的行动序列，提示词里也说不清是谁在行动。
// hero/villain/other 是 M7.1 之前的老数据口径，永久放行（老手牌要能原样编辑保存）。
func (h *ReviewHand) IsKnownActor(actor string) bool {
	switch actor {
	case ActorHero, ActorVillain, ActorOther:
		return true
	}
	return h.villainPositions()[actor]
}

// NormalizeActor 规范化行动者取值：位置统一成大写（客户端可能传 "co"），
// hero/villain/other 是小写枚举，不能跟着一起转。
// 本手牌没有的位置原样返回，交给 IsKnownActor 去报错
func (h *ReviewHand) NormalizeActor(actor string) string {
	trimmed := strings.TrimSpace(actor)
	switch trimmed {
	case ActorHero, ActorVillain, ActorOther:
		return trimmed
	}
	if upper := strings.ToUpper(trimmed); h.villainPositions()[upper] {
		return upper
	}
	return trimmed
}

// VillainByPosition 按位置取对手，取不到返回 nil
func (h *ReviewHand) VillainByPosition(position string) *VillainInfo {
	if position == "" {
		return nil
	}
	for i := range h.Villains {
		if h.Villains[i].Position == position {
			return &h.Villains[i]
		}
	}
	return nil
}

// ReviewHand 复盘手牌表
//
// 这是复盘功能的聚合根：一手牌永远整手读写，不存在"只查某条 action"的场景，
// 所以 Streets / Villains / HeroTags 用 JSON 列存，避免拆表后每次都要 join 组装。
// 需要检索的维度（位置、场次、时间）已经冗余成独立列。
type ReviewHand struct {
	ID           uint    `json:"id" gorm:"primaryKey"`
	UserID       uint    `json:"userId" gorm:"not null;index:idx_rh_user_created,priority:1;comment:记录者ID，数据隔离依据"`
	GameID       *uint   `json:"gameId" gorm:"index;comment:关联场次ID，可为空（支持独立复盘）"`
	Title        string  `json:"title" gorm:"size:255;comment:标题，为空时前端按位置+底牌自动生成"`
	TableSize    int     `json:"tableSize" gorm:"not null;default:9;comment:几人桌(2-9)，默认9。决定 HeroPosition 的合法取值"`
	HeroPosition string  `json:"heroPosition" gorm:"size:10;comment:我的位置，取值随 table_size 变化，如 UTG+2/LJ/HJ"`
	HeroCards    string  `json:"heroCards" gorm:"size:8;comment:我的底牌，规范格式如 AsKh"`
	HeroStackBB  float64 `json:"heroStackBb" gorm:"comment:我的有效筹码(BB)"`
	Stakes       string  `json:"stakes" gorm:"size:20;comment:盲注级别，如 5/10"`

	// 盲注与前注的额度（BB）。单独成列而不是伪造成"对手下注 0.5bb"塞进 Streets：
	// 那样模型会以为翻前真有人下注，底池推算的中间状态也会被污染。
	// 三个都是 0 表示没记录，此时底池估算与加这个功能之前完全一致
	SmallBlindBB float64 `json:"smallBlindBb" gorm:"default:0;comment:小盲(BB)"`
	BigBlindBB   float64 `json:"bigBlindBb" gorm:"default:0;comment:大盲(BB)"`
	AnteBB       float64 `json:"anteBb" gorm:"default:0;comment:前注(BB)，每人一份"`

	Board        string `json:"board" gorm:"size:10;comment:公共牌，按发牌顺序拼接如 Qs7h2d3c9s"`
	VillainCount int    `json:"villainCount" gorm:"comment:对手数量"`

	Villains []VillainInfo `json:"villains" gorm:"serializer:json;type:json;comment:对手信息"`

	PotType string `json:"potType" gorm:"size:10;default:'hu';comment:hu-单挑, multi-多人池"`

	Streets []StreetRecord `json:"streets" gorm:"serializer:json;type:json;comment:按街的行动序列"`

	HeroThought  string   `json:"heroThought" gorm:"type:text;comment:我当时是怎么想的"`
	Result       string   `json:"result" gorm:"size:10;default:'unknown';comment:win/lose/fold/unknown"`
	ResultAmount *float64 `json:"resultAmount" gorm:"comment:输赢金额(BB)"`

	HeroTags []string `json:"heroTags" gorm:"serializer:json;type:json;comment:用户自打标签"`

	// ContentHash 手牌内容指纹。内容未变更时不必重复调用 AI 分析，直接复用上次结果
	ContentHash string `json:"-" gorm:"size:64;comment:内容指纹，用于判断是否需要重新分析"`

	// AnalyzeStatus 冗余字段，列表页直接展示，避免为了一个状态去 join 分析表
	AnalyzeStatus string `json:"analyzeStatus" gorm:"size:20;default:'none';comment:none/pending/done/failed"`

	CreatedAt time.Time      `json:"createdAt" gorm:"index:idx_rh_user_created,priority:2"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (ReviewHand) TableName() string {
	return "review_hands"
}

// Blinds 取出手牌的盲注配置。
//
// 位置一并带上：底池推算要把大小盲认到具体行动者头上，认得出人才能算对跟注差额。
// M7.1 起每个对手都有位置，大小盲是谁一目了然；老手牌只有关键对手有位置，
// 行为与之前完全一致。
func (h *ReviewHand) Blinds() BlindConfig {
	b := BlindConfig{
		SmallBlindBB: h.SmallBlindBB,
		BigBlindBB:   h.BigBlindBB,
		AnteBB:       h.AnteBB,
		TableSize:    h.TableSize,
		HeroPosition: h.HeroPosition,
	}
	for i := range h.Villains {
		v := &h.Villains[i]
		if v.Position == "" {
			continue
		}
		// 有名字的是 M7.1 之后的记录：行动按位置记，账也记在位置上
		if v.Name != "" {
			b.VillainPositions = append(b.VillainPositions, v.Position)
			continue
		}
		// 没名字的是老数据：当时行动记在聚合角色 "villain" 上，只认第一个
		if b.LegacyVillainPosition == "" {
			b.LegacyVillainPosition = v.Position
		}
	}
	return b
}
