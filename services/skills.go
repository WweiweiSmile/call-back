package services

import (
	"call-go/models"
	"call-go/utils"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// skillMaxExpanded 本次最多展开几篇非常驻技能。
// 常驻技能不占这个额度 —— 它们不是「某场景的知识」，是所有场景的共同前提
const skillMaxExpanded = 6

//go:embed skills/*.md
var skillFS embed.FS

// 翻前局面形态。由行动序列算出（见 PreflopShapes），不是枚举字段能直接读出来的
const (
	// ShapeUnopened hero 行动前无人加注 —— 开池决策
	ShapeUnopened = "unopened"
	// ShapeSingleRaise hero 行动前恰好一次加注 —— 面对开池的决策
	ShapeSingleRaise = "single-raise"
	// ShapeThreeBet hero 行动前已有再加注 —— 3bet 底池的决策
	ShapeThreeBet = "three-bet"
)

// SkillTrigger 技能的路由条件。各非零维度取交集，全部通过才算命中；全零 = 不命中。
//
// 触发器写死在代码里而不是做成可配置项：技能库是开发者的产物，
// 触发条件错了应该在改代码的时候就暴露，而不是靠用户在界面上调。
//
// 刻意**没有**牌面结构与筹码深度两个维度：前者等 S3 的翻后技能真要用时再加，
// 后者要先定「有效筹码」的口径（min(hero, 各对手) 还是只看 hero）。
// 声明一个没人读的字段比不声明更危险。
type SkillTrigger struct {
	// Streets 手牌实际记录到行动的街（不是「应该有的街」）
	Streets []string
	// Positions 匹配 hand.HeroPosition
	Positions []string
	// PotTypes hu / multi
	PotTypes []string
	// TableSizes 2~9
	TableSizes []int
	// PreflopShapes 翻前局面形态，见 PreflopShapes
	PreflopShapes []string
	// MinVillains 至少要有几个记录了名字或位置的对手。
	// 用来把「对手读牌」类技能挡在没有对手信息的手牌之外
	MinVillains int
	// CbetSpot 要求 hero 处在「该不该持续下注」的决策点，见 isCbetSpot
	CbetSpot bool
	// MinMade 要求 hero 的**绝对**成牌等级至少到这一档。
	// 零值 MadeNone 表示不要求 —— MadeNone 本身就是最低档，不需要额外的「已设置」标记
	MinMade utils.MadeHand
	// RequireStrength 与 MinStrength/MaxStrength 一起构成相对牌力的闭区间。
	// 之所以要独立的开关：Tier 的零值是 TierAir，拿零值表示「未设置」就
	// 等于「只匹配空气」，静默匹配错人
	RequireStrength bool
	MinStrength     utils.Tier
	MaxStrength     utils.Tier
	// RequireDraw 要求 hero 有听牌（同花或顺子）
	RequireDraw bool
	// Keywords 扫 hand.HeroThought 与 hand.Title，任一命中即可
	Keywords []string
	// Always 常驻：跳过全部匹配，且不受 skillMaxExpanded 裁剪
	Always bool
}

// Skill 一篇教练技能。
//
// 元数据与触发条件在 Go 侧（编译期可见、可单测），正文在同目录的 skills/*.md
// （用 Markdown 维护，避免把表格与中文标点塞进 Go 的反引号字符串）。
type Skill struct {
	Code  string
	Title string
	// When 目录层里写的「什么时候用」
	When string
	// Summary 目录层里写的「一句话核心结论」。
	// 有它，未展开的技能才不是「完全不知道」，而是「细节不够」
	Summary  string
	Priority int
	Trigger  SkillTrigger

	body string
}

// skillDef 技能注册项。正文只在 loadSkills 里读一次。
type skillDef struct {
	Code     string
	Title    string
	When     string
	Summary  string
	Priority int
	Trigger  SkillTrigger
	file     string
}

// postflopStreets 翻后三条街。翻后技能基本都要它：
// 手牌只记到翻牌时，转牌与河牌的内容不该展开
var postflopStreets = []string{models.StreetFlop, models.StreetTurn, models.StreetRiver}

// skillDefs 技能注册表。
//
// 正文全部来自 coach_methodology.go 那 6 千字（Phil Gordon《小绿皮书》现金局部分），
// 按「决策场景」拆开。拆分一律用脚本按行切片并逐行核对，不手抄 ——
// 正文含 Markdown 表格与中文引号，手抄必丢（过程见 教练技能库设计文档.md §9）。
//
// Priority 决定展开时在提示词里的先后，也决定超出篇数上限时谁先被裁掉。
// 排序原则：与「这手牌打到哪条街」越相关的越靠前，翻前技能排最后 ——
// 河牌的手牌里，翻前开池细则没有河牌价值下注重要
var skillDefs = []skillDef{
	{
		Code:     "core-stance",
		Title:    "底层立场与决策流程",
		When:     "每一手牌",
		Summary:  "先问能不能下注或加注，再问要不要过牌或弃牌；说不出理由的下注与过牌都是漏损",
		Priority: 100,
		Trigger:  SkillTrigger{Always: true},
		file:     "skills/core-stance.md",
	},
	{
		Code:     "odds-table",
		Title:    "赔率与概率速查",
		When:     "任何要算赔率、outs 或成牌概率的地方",
		Summary:  "1/3 池需 20% 胜率、1/2 池 25%、1 倍池 33%、2 倍池 40%；转牌面对 1/2 池以上的下注，用任何听牌跟注都是错的",
		Priority: 95,
		Trigger:  SkillTrigger{Always: true},
		file:     "skills/odds-table.md",
	},
	{
		Code:     "opponent-read",
		Title:    "对手读牌",
		When:     "这手牌至少记录了一个对手的名字或位置",
		Summary:  "五格形象各有对策；样本不足 3 次归「未知」按标准线打，不做剥削性偏离",
		Priority: 88,
		Trigger:  SkillTrigger{MinVillains: 1},
		file:     "skills/opponent-read.md",
	},
	// ---- 河牌 ----
	{
		Code:     "river-value",
		Title:    "河牌价值下注",
		When:     "河牌你持强牌，考虑收价值",
		Summary:  "用钟形曲线定尺度，让价值下注看起来不像价值下注；过牌-加注使用率低于 1/10",
		Priority: 82,
		Trigger: SkillTrigger{
			Streets:         []string{models.StreetRiver},
			RequireStrength: true,
			MinStrength:     utils.TierStrong,
			MaxStrength:     utils.TierNuts,
		},
		file: "skills/river-value.md",
	},
	{
		Code:     "river-medium",
		Title:    "河牌中等牌力",
		When:     "河牌你持中等牌力（含被牌面削弱过的强牌）",
		Summary:  "无位置持中等牌下注是最糟的错误之一，该过牌引诱诈唬；对手在无关牌面下注，倾向认定是诈唬",
		Priority: 81,
		Trigger: SkillTrigger{
			Streets:         []string{models.StreetRiver},
			RequireStrength: true,
			MinStrength:     utils.TierWeak,
			MaxStrength:     utils.TierMedium,
		},
		file: "skills/river-medium.md",
	},
	// ---- 转牌 / 翻牌 ----
	{
		Code:     "postflop-turn",
		Title:    "转牌决策",
		When:     "这手牌打到了转牌",
		Summary:  "转牌不是耍花招的时候，有最好的牌就下注；吓人牌出现时把开火权让给对手、过牌-跟注控池",
		Priority: 78,
		Trigger:  SkillTrigger{Streets: []string{models.StreetTurn}},
		file:     "skills/postflop-turn.md",
	},
	{
		Code:     "postflop-made-hand",
		Title:    "成牌打法",
		When:     "翻后你持两对或更强的成牌",
		Summary:  "两对只有 17% 会再变大、顶底两对最脆弱；三条要让对手犯最大的错；顺子与同花按牌面结构定尺度",
		Priority: 76,
		Trigger:  SkillTrigger{MinMade: utils.MadeTwoPair, Streets: postflopStreets},
		file:     "skills/postflop-made-hand.md",
	},
	{
		Code:     "postflop-draw",
		Title:    "听牌与半诈唬",
		When:     "你在听花或听顺",
		Summary:  "优质抽牌对单一对手可以当强牌打，主动下注有两条赢路；转牌跟注只看隐含赔率",
		Priority: 74,
		Trigger:  SkillTrigger{RequireDraw: true, Streets: postflopStreets},
		file:     "skills/postflop-draw.md",
	},
	{
		Code:     "postflop-cbet",
		Title:    "持续下注",
		When:     "你是翻前最后的加注者，翻牌轮到你时还没有人下注",
		Summary:  "按单挑情形取频率（位置好的对手跟注 65%、位置差的对手过牌 85%）；尺度用四因素法；对子面先下注",
		Priority: 72,
		Trigger:  SkillTrigger{CbetSpot: true},
		file:     "skills/postflop-cbet.md",
	},
	{
		Code:     "postflop-multiway",
		Title:    "多人池铁律",
		When:     "翻后还有三个以上的人",
		Summary:  "人越多越不诈唬；认为牌最大就几乎肯定下注、几乎从不慢打",
		Priority: 70,
		Trigger:  SkillTrigger{PotTypes: []string{models.PotTypeMulti}, Streets: postflopStreets},
		file:     "skills/postflop-multiway.md",
	},
	// ---- 翻前 ----
	{
		Code:     "preflop-3bet-pot",
		Title:    "翻前 3bet 底池",
		When:     "翻前你行动时已经出现过再加注",
		Summary:  "再加注尺度按位置定；翻前投入超过一半筹码即 100% 套池，此时诈唬毫无意义",
		Priority: 66,
		Trigger:  SkillTrigger{PreflopShapes: []string{ShapeThreeBet}},
		file:     "skills/preflop-3bet-pot.md",
	},
	{
		Code:     "preflop-open",
		Title:    "翻前开池",
		When:     "翻前你行动时还没有人加注",
		Summary:  "前位 2.5-3 倍、后位 3.5-4 倍，不按牌力调尺度；第一个入池总是加注，平跟只允许三种情形",
		Priority: 62,
		Trigger:  SkillTrigger{PreflopShapes: []string{ShapeUnopened}},
		file:     "skills/preflop-open.md",
	},
	{
		Code:     "preflop-vs-open",
		Title:    "翻前面对开池",
		When:     "翻前你行动时前面已有一次加注",
		Summary:  "位置好倾向跟注设陷阱、盲注位倾向再加注；KK/QQ 几乎总是再加注",
		Priority: 60,
		Trigger:  SkillTrigger{PreflopShapes: []string{ShapeSingleRaise}},
		file:     "skills/preflop-vs-open.md",
	},
	{
		Code:    "mindset",
		Title:   "心态与资金",
		When:    "玩家自己的想法里出现情绪、上头、下风或资金相关的字眼",
		Summary: "Bad beat 是营业成本；只有「当时是领先下注」才值得复盘情绪；2000 手以内的波动不构成改策略的依据",
		// 优先级高于所有专项技能：它是**用户自己写的话**触发的。
		// 被翻前细则之类的挤掉，等于用户说"我上头了"而教练不理
		Priority: 86,
		Trigger:  SkillTrigger{Keywords: []string{"上头", "情绪", "下风", "tilt", "追损", "破产", "心态"}},
		file:     "skills/mindset.md",
	},
}

var (
	skillsOnce sync.Once
	skills     []Skill
)

// AllSkills 返回全部技能（已加载正文）。加载失败直接 panic。
func AllSkills() []Skill {
	skillsOnce.Do(func() {
		skills = loadSkills(skillDefs)
	})
	return skills
}

// loadSkills 读取技能正文。
//
// go:embed 保证文件在编译期就存在，所以这里的失败只可能是注册表写了不存在的
// 文件名 —— 那是编码错误。**直接 panic 而不是降级**：静默变成空技能库会让
// 所有分析悄悄失去依据，而且不会有任何报错，是最难查的一类故障。
func loadSkills(defs []skillDef) []Skill {
	out := make([]Skill, 0, len(defs))
	for _, d := range defs {
		raw, err := skillFS.ReadFile(d.file)
		if err != nil {
			panic(fmt.Sprintf("技能 %s 的正文读取失败（%s）：%v", d.Code, d.file, err))
		}
		out = append(out, Skill{
			Code:     d.Code,
			Title:    d.Title,
			When:     d.When,
			Summary:  d.Summary,
			Priority: d.Priority,
			Trigger:  d.Trigger,
			body:     string(raw),
		})
	}
	return out
}

// scored 技能 + 命中维度数，命中维度数参与排序
type scored struct {
	skill Skill
	hits  int
}

// SelectSkills 按手牌场景挑出本次要展开的技能，已排序并裁剪。
//
// 抽成纯函数（除了写日志没有别的副作用）是因为这是整套机制最容易出错的一环 ——
// 通配、交集、排序、裁剪都要能脱离整条链路单独测，与 AggregateInsights 同款处理。
//
// 裁剪是**按篇丢弃，不切断正文**：一篇被拦腰截断的方法论比没有更危险
// （比如「建议过牌必须命中以下之一」后面那串理由被切掉，就成了纯错误建议）。
func SelectSkills(all []Skill, hand *models.ReviewHand) []Skill {
	matched := make([]scored, 0, len(all))
	for _, s := range all {
		hits, ok := matchSkill(s.Trigger, hand)
		if !ok {
			continue
		}
		matched = append(matched, scored{skill: s, hits: hits})
	}

	// 排序必须完全确定：Go 的 map 遍历顺序随机，而触发器里用了 map。
	// 不钉死的话同一手牌两次分析的提示词会不一样，input_snapshot 就失去了
	// 可复现性，调试时会被误导。所以最后一级用 Code 字典序兜底
	sort.SliceStable(matched, func(i, j int) bool {
		a, b := matched[i], matched[j]
		if a.skill.Priority != b.skill.Priority {
			return a.skill.Priority > b.skill.Priority
		}
		if a.hits != b.hits {
			return a.hits > b.hits
		}
		return a.skill.Code < b.skill.Code
	})

	out := make([]Skill, 0, len(matched))
	expanded := 0
	for _, m := range matched {
		if !m.skill.Trigger.Always {
			if expanded >= skillMaxExpanded {
				// 截断必须留痕（设计文档 §6.2）：这条日志是 SelectSkills 唯一的副作用，
				// 不影响返回值。没有它，「某篇技能从没生效过」这件事永远查不出来
				log.Printf("技能 %s 命中但被篇数上限（%d）裁掉，手牌 %v",
					m.skill.Code, skillMaxExpanded, handID(hand))
				continue
			}
			expanded++
		}
		out = append(out, m.skill)
	}
	return out
}

// matchSkill 判断一篇技能的触发器是否命中当前手牌，并返回命中的维度数。
// 命中维度数越大说明触发器填得越具体，也就是越针对当前场景，排序时优先保留。
func matchSkill(t SkillTrigger, hand *models.ReviewHand) (int, bool) {
	if t.Always {
		return 0, true
	}
	if hand == nil {
		// 非常驻技能没有手牌就没有场景可匹配。常驻技能上面已经返回了
		return 0, false
	}

	hits := 0

	if len(t.Streets) > 0 {
		if !intersects(t.Streets, recordedStreets(hand)) {
			return 0, false
		}
		hits++
	}
	if len(t.Positions) > 0 {
		if !containsString(t.Positions, hand.HeroPosition) {
			return 0, false
		}
		hits++
	}
	if len(t.PotTypes) > 0 {
		if !containsString(t.PotTypes, hand.PotType) {
			return 0, false
		}
		hits++
	}
	if len(t.TableSizes) > 0 {
		if !containsInt(t.TableSizes, hand.TableSize) {
			return 0, false
		}
		hits++
	}
	if len(t.PreflopShapes) > 0 {
		if !intersects(t.PreflopShapes, PreflopShapes(hand)) {
			return 0, false
		}
		hits++
	}
	if t.MinVillains > 0 {
		if countKnownVillains(hand) < t.MinVillains {
			return 0, false
		}
		hits++
	}
	if t.CbetSpot {
		if !isCbetSpot(hand) {
			return 0, false
		}
		hits++
	}
	// 三项牌力维度都来自同一次计算，所以合并成一次调用
	if t.MinMade > utils.MadeNone || t.RequireStrength || t.RequireDraw {
		st, ok := handStrength(hand)
		if !ok {
			return 0, false
		}
		if t.MinMade > utils.MadeNone {
			if st.Made < t.MinMade {
				return 0, false
			}
			hits++
		}
		if t.RequireStrength {
			if st.Tier < t.MinStrength || st.Tier > t.MaxStrength {
				return 0, false
			}
			hits++
		}
		if t.RequireDraw {
			if !st.FlushDraw && !st.StraightDraw {
				return 0, false
			}
			hits++
		}
	}
	if len(t.Keywords) > 0 {
		if !anyKeywordHit(t.Keywords, hand) {
			return 0, false
		}
		hits++
	}

	// 全零触发器：hits=0 且所有分支都没进 —— 不命中。
	// 这是刻意的：没写适用范围的技能不该默认命中并霸占预算
	return hits, hits > 0
}

// PreflopShapes 返回 hero 在翻前面对过的局面形态（去重、已排序）。
//
// 为什么不能只数行动条数：下面两手牌翻前都是 3 个行动，决策场景却完全不同 ——
//
//	BTN 加注 / SB 弃牌 / BB 弃牌      → hero 只面对「无人加注」，是开池决策
//	BTN 加注 / SB 再加注 / BB 弃牌    → hero 面对的是 3bet
//
// 数条数会把后者也路由成开池技能。判据必须是「hero 行动之前有几次加注」。
//
// hero 的每一次翻前行动都对应一个决策点，所以返回的是**集合**：
// 「hero 开池后被 3bet」会同时得到 unopened 与 three-bet，两篇技能都该展开。
func PreflopShapes(hand *models.ReviewHand) []string {
	if hand == nil {
		return nil
	}

	actions := streetActions(hand, models.StreetPreflop)
	if len(actions) == 0 {
		return nil
	}

	seen := map[string]bool{}
	out := make([]string, 0, 3)
	add := func(shape string) {
		if !seen[shape] {
			seen[shape] = true
			out = append(out, shape)
		}
	}

	raises := 0
	heroActed := false
	for _, a := range actions {
		if isHeroAction(hand, a.Actor) {
			heroActed = true
			add(shapeFor(raises))
		}
		if isAggression(a.Action) {
			raises++
		}
	}

	// hero 在翻前没有行动（如 BB 上直接走牌）。按整条序列的加注次数给一个形态，
	// 否则这手牌拿不到任何翻前技能
	if !heroActed {
		add(shapeFor(raises))
	}

	sort.Strings(out)
	return out
}

func shapeFor(raisesBefore int) string {
	switch {
	case raisesBefore == 0:
		return ShapeUnopened
	case raisesBefore == 1:
		return ShapeSingleRaise
	default:
		return ShapeThreeBet
	}
}

// isAggression 这一手算不算「加注」。
//
// allin 也计入：翻前的全下几乎总是再加注，不计的话会把短筹码的 3bet 底池
// 判成无人加注。代价是「全下跟注」会被误计一次，那种情形在真实牌局里极少
func isAggression(action string) bool {
	return action == models.ActionRaise || action == models.ActionAllin
}

// isHeroAction 判断这条行动是不是 hero 做的。
// 新数据（M7.1 起）对手按位置记、hero 也按位置记；老数据用聚合角色 hero
func isHeroAction(hand *models.ReviewHand, actor string) bool {
	if actor == models.ActorHero {
		return true
	}
	return hand.HeroPosition != "" && actor == hand.HeroPosition
}

// countKnownVillains 记了名字或位置的对手数量。
// 只记了筹码的老数据不算 —— 那种对手信息不足以做任何形象判断
func countKnownVillains(hand *models.ReviewHand) int {
	n := 0
	for i := range hand.Villains {
		if hand.Villains[i].Name != "" || hand.Villains[i].Position != "" {
			n++
		}
	}
	return n
}

// streetActions 某条街的行动序列，没有则返回 nil
func streetActions(hand *models.ReviewHand, street string) []models.StreetAction {
	for i := range hand.Streets {
		if hand.Streets[i].Street == street {
			return hand.Streets[i].Actions
		}
	}
	return nil
}

// isPostflopAggression 翻后已经有人把钱投进去了。
//
// 必须与 isAggression 分开：翻前的「第一个入池」是加注（平跟不算进攻），
// 而翻后的下注（bet）本身就是进攻。用同一个判据会让翻后的下注被漏掉，
// 于是「面对下注」被当成「轮到我先说话」
func isPostflopAggression(action string) bool {
	return action == models.ActionBet || action == models.ActionRaise || action == models.ActionAllin
}

// isPreflopAggressor hero 是不是翻前最后一个加注的人 —— 持续下注的前提。
// 全是平跟时没有任何加注者，那种局面没有「持续下注」可言
func isPreflopAggressor(hand *models.ReviewHand) bool {
	last := ""
	for _, a := range streetActions(hand, models.StreetPreflop) {
		if isAggression(a.Action) {
			last = a.Actor
		}
	}
	return last != "" && isHeroAction(hand, last)
}

// isCbetSpot hero 处在「该不该持续下注」这个决策点：他是翻前最后一个加注的人，
// 并且翻牌轮到他时还没有人下注。
//
// 后半条不能省：翻前加注者面对别人在翻牌的下注时，那是「要不要跟注或加注」，
// 不是持续下注。拿持续下注的频率表去点评那种局面会给出方向相反的建议
func isCbetSpot(hand *models.ReviewHand) bool {
	if !isPreflopAggressor(hand) {
		return false
	}

	faced := false
	for _, a := range streetActions(hand, models.StreetFlop) {
		if isHeroAction(hand, a.Actor) {
			return !faced
		}
		if isPostflopAggression(a.Action) {
			faced = true
		}
	}
	// hero 在翻牌没有行动，说明这条街没有他的决策点
	return false
}

// handStrength 算 hero 的相对牌力（见 utils.EvaluateHand）。
//
// 牌面缺失或非法时 ok=false，调用方必须把它当作「这一维不命中」而不是当成空气：
// 把算不出来的牌力当成最弱，会让「河牌中等牌」这类技能在手牌记不全时错误展开，
// 而那正是最不该给建议的时候
func handStrength(hand *models.ReviewHand) (utils.HandStrength, bool) {
	if hand == nil {
		return utils.HandStrength{}, false
	}
	st, err := utils.EvaluateHand(hand.HeroCards, hand.Board)
	if err != nil {
		return utils.HandStrength{}, false
	}
	return st, true
}

// recordedStreets 手牌里实际记录到行动的街。
// 只有行动条数大于 0 的街才算 —— 表单里可能存在空街记录
func recordedStreets(hand *models.ReviewHand) []string {
	out := make([]string, 0, len(hand.Streets))
	for i := range hand.Streets {
		if len(hand.Streets[i].Actions) > 0 {
			out = append(out, hand.Streets[i].Street)
		}
	}
	return out
}

// anyKeywordHit 关键词是否命中玩家的自由文本。
// 这是唯一会读用户输入的触发维度，所以只做子串匹配、不做任何解释执行
func anyKeywordHit(keywords []string, hand *models.ReviewHand) bool {
	haystack := hand.HeroThought + "\n" + hand.Title
	for _, kw := range keywords {
		if kw != "" && strings.Contains(haystack, kw) {
			return true
		}
	}
	return false
}

func handID(hand *models.ReviewHand) uint {
	if hand == nil {
		return 0
	}
	return hand.ID
}

// RenderSkillCatalog 拼装技能目录（渐进性披露的第一层，永远注入）。
//
// 目录不是纯索引：每篇带一句话结论，所以「未展开」的代价是「细节不够」而不是
// 「完全不知道」。★ 标出本次已展开全文的，防止模型拿一句结论硬编细则。
//
// 方法论的出处与「结论必须能指回规则」这条总要求写在这里 —— 它原来写在
// 方法论正文的开头，拆成多篇之后没有哪一篇是它的自然归属了
func RenderSkillCatalog(all []Skill, selected []Skill) string {
	if len(all) == 0 {
		return ""
	}

	expanded := make(map[string]bool, len(selected))
	for _, s := range selected {
		expanded[s.Code] = true
	}

	width := 0
	for _, s := range all {
		if n := len([]rune(s.Code)); n > width {
			width = n
		}
	}

	var sb strings.Builder
	// 头部只留出处与「结论要能指回规则」这一条 —— 它不在硬性约束里，只存在于这里。
	// ★ 的含义与「不要凭一句话结论展开细节」写在硬性约束 13/15，不在这里重复：
	// 目录每多一行说明，就是每一手牌都要多付一次
	sb.WriteString("## 教练技能目录\n")
	sb.WriteString("本次点评的依据来自 Phil Gordon《德州扑克小绿皮书》（仅现金局部分）。\n")
	sb.WriteString("你的每一条结论都要能指回某一条技能规则；另外补充的判断要显式标注「这是我加的，书里没有」。\n")
	sb.WriteString("★ = 本次已展开全文，正文在下面。\n\n")

	for _, s := range all {
		pad := strings.Repeat(" ", width-len([]rune(s.Code)))
		// 已展开的技能只列代号与标题：它的「什么时候用」和一句话结论，
		// 在下面紧跟着的全文里本来就有，再列一遍是纯冗余。
		// 15 篇技能时这一项能省掉近一半目录字数（见设计文档 §6.2 的预算）
		if expanded[s.Code] {
			fmt.Fprintf(&sb, "★ %s%s — %s\n", s.Code, pad, s.Title)
			continue
		}
		fmt.Fprintf(&sb, "  %s%s — %s｜结论：%s\n", s.Code, pad, s.When, s.Summary)
	}
	return sb.String()
}

// RenderSkillBlock 把选中的技能拼成提示词里的技能正文层（第二层）。
//
// 每篇的形态是 "## <标题>\n<正文>"，正文原样来自 .md（含来源注记），中间不插入
// 任何东西 —— 这样任何一次正文变更都能在 input_snapshot 的 diff 里一眼看到。
//
// 结尾补换行是**归一化而不是美化**：节间靠 "\n" 分隔，如果某一篇的正文
// 恰好不以换行结尾（.md 末尾少一个换行就会这样），下一篇的 "## " 会被顶到
// 同一行，两篇技能黏成一篇。不能把这件事交给「文件末尾应该有空行」这种约定。
func RenderSkillBlock(selected []Skill) string {
	sections := make([]string, 0, len(selected))
	for _, s := range selected {
		section := "## " + s.Title + "\n" + s.body
		if !strings.HasSuffix(section, "\n") {
			section += "\n"
		}
		sections = append(sections, section)
	}
	return strings.Join(sections, "\n")
}

// SkillBodyRunes 本次展开的正文体量，写进日志用于观测（设计文档 §6.2 的预算依据）
func SkillBodyRunes(selected []Skill) int {
	n := 0
	for _, s := range selected {
		n += len([]rune(s.body))
	}
	return n
}

// LogSkillLayerSizes 记录技能层两层的真实体量，返回 (目录层字数, 正文层字数)。
// 调用点在 BuildSystemPrompt 拼完技能层之后 —— 它**不改变任何输出**，只观测。
//
// **刻意不设数值上限。** 曾经定过两档（目录 900 / 正文 4000），但两个数字都站不住：
// 正文那边常驻两篇（core-stance 1280 + odds-table 2000）自己就占 3280 字，
// **展开第 1 篇非常驻技能就超顶**，等于每次分析都响；目录那边只在简单手牌
// （展开 ≤4 篇，★ 行少所以目录长）上响。**一条常态触发的告警等于没有告警。**
//
// 所以改成每手牌记录真实体量：要定阈值的时候，先拿一段时间序列出来看，
// 而不是拍一个数字然后让它天天响。
func LogSkillLayerSizes(catalog string, selected []Skill) (catalogRunes, bodyRunes int) {
	resident := 0
	for _, s := range selected {
		if s.Trigger.Always {
			resident++
		}
	}
	catalogRunes, bodyRunes = len([]rune(catalog)), SkillBodyRunes(selected)
	log.Printf("技能层体量：目录 %d 字 + 正文 %d 字 = %d 字（常驻 %d 篇，本次展开 %d 篇）",
		catalogRunes, bodyRunes, catalogRunes+bodyRunes, resident, len(selected))
	return
}

// ---------- 小工具 ----------

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func containsInt(list []int, v int) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func intersects(a, b []string) bool {
	for _, item := range a {
		if containsString(b, item) {
			return true
		}
	}
	return false
}
