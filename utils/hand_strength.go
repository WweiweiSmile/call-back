package utils

import (
	"fmt"
	"sort"
)

// 本文件算的是**相对牌力**，不是绝对牌力。
//
// 为什么必须是相对的：牌力从来不是手牌自己的属性，而是「它相对于这个牌面有多强」。
// 同一个顺子：
//   - 在无同花面、无公对的面（如 9♠8♦2♣ 上持 JT）—— 只有更大的顺子能赢它，准坚果
//   - 在有同花面且公对的面（如 9♠8♠9♠ 上持 JT）—— 对手范围里合理存在同花与葫芦，
//     它只是中等牌
//
// 绝对牌力算出来两者都是「顺子」，而复盘里的决策（该不该打光筹码）完全不同。
//
// 两条设计约束：
//  1. 结果里带 Notes，逐条写清档位被调整的原因。牌力判断会直接影响技能路由与
//     点评口径，必须可审计、可回溯，不能只给一个数字。
//  2. 不猜。牌面张数不够就按已有信息算，不假设还没发的牌。
//
// v1 的非目标（刻意不做，见 教练技能库设计文档.md §4.3）：
//   - 踢脚的强弱（顶对好踢 vs 顶对弱踢，只在「无成牌」一侧区分了超对/顶对/弱对）
//   - 听牌的成牌概率与隐含赔率（只报「有没有听牌」，不算赔率）
//   - 按对手人数加权（人数在 hand 上，不在牌面上）
//   - 现代范围平衡与求解器口径

// MadeHand 成牌等级（绝对，与牌面结构无关）。顺序即强弱，可直接比大小
type MadeHand int

const (
	MadeNone MadeHand = iota
	MadeWeakPair
	MadeTopPair
	MadeOverpair
	MadeTwoPair
	MadeSet
	MadeTrips
	MadeStraight
	MadeFlush
	MadeFullHouse
	MadeQuads
	MadeStraightFlush
)

var madeHandNames = map[MadeHand]string{
	MadeNone:          "无成牌",
	MadeWeakPair:      "中对或底对",
	MadeTopPair:       "顶对",
	MadeOverpair:      "超对",
	MadeTwoPair:       "两对",
	MadeSet:           "三条（口袋对中牌）",
	MadeTrips:         "明三条（公对成三条）",
	MadeStraight:      "顺子",
	MadeFlush:         "同花",
	MadeFullHouse:     "葫芦",
	MadeQuads:         "四条",
	MadeStraightFlush: "同花顺",
}

func (m MadeHand) String() string {
	if name, ok := madeHandNames[m]; ok {
		return name
	}
	return fmt.Sprintf("未知成牌等级(%d)", int(m))
}

// Tier 相对牌力档位。它回答的不是「我是什么牌」，而是
// 「这副牌在这个牌面上值多少筹码」—— 也就是「该不该打大池」
type Tier int

const (
	// TierAir 空气：没有成牌，也没有听牌
	TierAir Tier = iota
	// TierWeak 弱：中对/底对。能看一条街，不值得投入大筹码
	TierWeak
	// TierMedium 中等：顶对，或被牌面削过的强牌。
	// 关键特征是**不该为它打光筹码**（用户口径：「相对牌力一般」）
	TierMedium
	// TierStrong 强：超对、两对。可以打大池，但要防被反超
	TierStrong
	// TierVeryStrong 很强：三条，以及未被牌面削弱的顺子与同花
	TierVeryStrong
	// TierNuts 坚果级：葫芦以上。牌面上几乎找不到能赢它的合理组合
	TierNuts
)

var tierNames = map[Tier]string{
	TierAir:        "空气",
	TierWeak:       "弱",
	TierMedium:     "中等",
	TierStrong:     "强",
	TierVeryStrong: "很强",
	TierNuts:       "坚果级",
}

func (t Tier) String() string {
	if name, ok := tierNames[t]; ok {
		return name
	}
	return fmt.Sprintf("未知档位(%d)", int(t))
}

// BoardTexture 公共牌的结构特征。
//
// 这份结构有两个消费者，所以必须只有一份实现：
//   - 技能触发器（决定展开哪几篇技能）
//   - 相对牌力的降级判断（本文件）
//
// 拆成两份会漂移，而漂移的后果是「触发器认为这是同花面、牌力判断认为不是」
// 这种自相矛盾的组合，而且极难查
type BoardTexture struct {
	// Paired 公牌里有对子。有人可能是葫芦
	Paired bool
	// TripsOnBoard 公牌里三条同点（如 KKK）
	TripsOnBoard bool
	// ThreeFlush 至少三张同花。再出一张就有人成同花
	ThreeFlush bool
	// FourFlush 至少四张同花。持该花色单张的人已成同花
	FourFlush bool
	// Connected 公牌点数连通（三张跨度不超过 4），顺面
	Connected bool
	// AceHigh 公牌含 A
	AceHigh bool
	// HighRank 公牌最大点数，2~14
	HighRank int
	// Cards 规范化后的公牌
	Cards []string
}

// AnalyzeBoard 解析公共牌结构。cards 需已规范化（见 NormalizeCards）
func AnalyzeBoard(cards []string) BoardTexture {
	t := BoardTexture{Cards: cards}

	counts := map[int]int{}
	suitCounts := map[byte]int{}
	ranks := make([]int, 0, len(cards))

	for _, c := range cards {
		if len(c) != 2 {
			continue
		}
		v := rankValue(c[0])
		if v == 0 {
			continue
		}
		ranks = append(ranks, v)
		counts[v]++
		suitCounts[c[1]]++
		if v > t.HighRank {
			t.HighRank = v
		}
		if v == 14 {
			t.AceHigh = true
		}
	}

	for _, n := range counts {
		if n >= 3 {
			t.TripsOnBoard = true
		}
		if n >= 2 {
			t.Paired = true
		}
	}
	for _, n := range suitCounts {
		if n >= 4 {
			t.FourFlush = true
		}
		if n >= 3 {
			t.ThreeFlush = true
		}
	}

	t.Connected = isConnected(ranks)

	return t
}

// isConnected 公牌是否连通到「顺面」的程度：三张牌的跨度不超过 4。
// A 同时按 1 与 14 参与，所以 A23 与 QKA 都算连通。
//
// 用「三张跨度」而不是逐对间隔，是因为后者在 9-8-2 这种面上会误报：
// 98 虽然只差 1，但没有第三张配合时它离顺子还很远
func isConnected(ranks []int) bool {
	if len(ranks) < 3 {
		return false
	}

	vals := append([]int(nil), ranks...)
	for _, v := range ranks {
		if v == 14 {
			vals = append(vals, 1)
			break
		}
	}
	sort.Ints(vals)

	uniq := make([]int, 0, len(vals))
	for i, v := range vals {
		if i == 0 || v != vals[i-1] {
			uniq = append(uniq, v)
		}
	}

	for i := 0; i+2 < len(uniq); i++ {
		if uniq[i+2]-uniq[i] <= 4 {
			return true
		}
	}
	return false
}

// HandStrength hero 在当前牌面上的牌力
type HandStrength struct {
	// Made 绝对成牌等级（与牌面结构无关）
	Made MadeHand
	// Tier 相对档位，已按牌面结构与本文件顶部的规则调整过
	Tier Tier
	// BoardPlays hero 的两张牌没有让牌变好 —— 公牌自己就是这副牌。
	// 这是最重的一条降级：**这副牌人人都有**
	BoardPlays bool
	// FlushDraw hero 有四张同花
	FlushDraw bool
	// StraightDraw hero 有四张顺
	StraightDraw bool
	// Notes 档位被调整的逐条原因
	Notes []string
}

// EvaluateHand 计算 hero 在某个牌面上的相对牌力。
//
// heroCards 两张，board 三到五张（都要先规范化）。街道不用传 —— 它由 board
// 的张数唯一决定，多一个参数就多一处可以不一致的地方。
//
// 牌面不合法时返回错误。调用方（技能触发器）应当把错误当作「无法按牌力路由」
// 并记日志：技能不展开只是让点评少一层细节，而拿错误的牌力去路由会让模型
// 按错误的依据点评，后者难发现得多。
func EvaluateHand(heroCards, board string) (HandStrength, error) {
	hero := NormalizeCards(heroCards)
	bd := NormalizeCards(board)

	if err := ValidateCards(hero); err != nil {
		return HandStrength{}, fmt.Errorf("hero 手牌不合法：%w", err)
	}
	if len(hero) != 4 {
		return HandStrength{}, fmt.Errorf("hero 手牌必须是 2 张，实际 %d 张：%s", len(hero)/2, hero)
	}
	if err := ValidateCards(bd); err != nil {
		return HandStrength{}, fmt.Errorf("公共牌不合法：%w", err)
	}
	if n := len(bd) / 2; n != 3 && n != 4 && n != 5 {
		return HandStrength{}, fmt.Errorf("公共牌必须是 3~5 张，实际 %d 张：%s", n, bd)
	}

	heroRanks, heroSuits := splitCards(hero)
	boardCards := splitString(bd)
	boardRanks, boardSuits := splitCards(bd)

	allRanks := append(append([]int(nil), heroRanks...), boardRanks...)
	allSuits := append(append([]byte(nil), heroSuits...), boardSuits...)

	s := HandStrength{Made: evaluateMade(heroRanks, boardRanks, allRanks, allSuits)}

	// 「公牌自己成牌」不能靠成牌等级相同来判断：公牌是 9 高顺、hero 用 TJ 接成
	// J 高顺时，两者等级都是「顺子」，但 hero 的牌明显更好。必须比具体分值
	if cmpScore(handScore(allRanks, allSuits), handScore(boardRanks, boardSuits)) <= 0 {
		s.BoardPlays = true
	}

	// 听牌要同时满足两个条件：
	//
	//  1. **还有牌可发**。河牌上已经没有下一张，所谓「听牌」不成立 ——
	//     真牌局里没人会在河牌谈听牌。不挡这一条，一手已经打完的牌也会被判成
	//     「在听牌」，`postflop-draw` 那篇技能就会在河牌上展开，是纯噪音。
	//     这是真机验证时发现的：河牌持 A 高被算出了 A2345 的卡顺听牌。
	//  2. 没有成牌，或只有弱对。已经成了顺子还报「顺子听牌」同样是噪音
	if len(boardCards) < 5 && s.Made <= MadeWeakPair {
		s.FlushDraw = hasFlushDraw(heroSuits, allSuits)
		s.StraightDraw = hasStraightDraw(allRanks)
	}

	s.Tier = relativeTier(s.Made, s.BoardPlays, AnalyzeBoard(boardCards), &s.Notes)

	return s, nil
}

// ---------- 绝对成牌等级 ----------

// evaluateMade 算出 hero 的成牌等级。
//
// 与 handScore 的分工：handScore 只回答「哪副牌更大」，这里要回答
// 「hero 的牌属于哪一类」，而后者需要 hero 与公牌的上下文 —— 同样是「一对」，
// 口袋对 K 在 Q72 上是超对，配公牌 Q 才是顶对，配公牌 2 是底对。
//
// boardRanks 必须单独传：顶对的定义是「配对的是**公牌**里最大的那张」，
// 拿 hero + 公牌的最大点数去比，hero 的 A 踢脚会把顶对判成中对
func evaluateMade(heroRanks []int, boardRanks []int, ranks []int, suits []byte) MadeHand {
	if len(ranks) == 0 {
		return MadeNone
	}

	counts := map[int]int{}
	for _, v := range ranks {
		counts[v]++
	}

	suitRanks := map[byte][]int{}
	for i, s := range suits {
		suitRanks[s] = append(suitRanks[s], ranks[i])
	}

	// 同花顺与四条都要先于其它判断。同花顺排在四条前面只是为了让顺序与
	// handScore 一致 —— 七张牌里两者不可能同时出现（四条占满四个花色），
	// 但顺序不一致将来会变成真的坑
	for _, rs := range suitRanks {
		if len(rs) >= 5 && bestStraight(rs) != nil {
			return MadeStraightFlush
		}
	}

	for _, n := range counts {
		if n >= 4 {
			return MadeQuads
		}
	}

	// 葫芦
	if _, _, ok := fullHouseRanks(counts); ok {
		return MadeFullHouse
	}

	// 同花（七张牌里最多只有一个花色能到五张）
	for _, rs := range suitRanks {
		if len(rs) >= 5 {
			return MadeFlush
		}
	}

	if bestStraight(ranks) != nil {
		return MadeStraight
	}

	// 三条 / 明三条
	trips := ranksWithCount(counts, 3)
	if len(trips) > 0 {
		sort.Sort(sort.Reverse(sort.IntSlice(trips)))
		// hero 两张都是这个点数 = 口袋对中牌（set，最隐蔽）
		// 只有一张参与 = 公对配手牌（明三条，隐蔽性差很多）
		if len(heroRanks) == 2 && heroRanks[0] == trips[0] && heroRanks[1] == trips[0] {
			return MadeSet
		}
		return MadeTrips
	}

	// 两对（走到这里已经没有三条）
	pairs := ranksWithCount(counts, 2)
	sort.Sort(sort.Reverse(sort.IntSlice(pairs)))
	if len(pairs) >= 2 {
		return MadeTwoPair
	}

	// 一对：超对 / 顶对 / 中对底对。这三者的相对牌力差得很远。
	// 三条已在上面返回，所以这里 pairs 恰好一个点数
	if len(pairs) == 1 {
		paired := pairs[0]
		boardMax := maxOf(boardRanks)

		// 口袋对且比公牌最大的那张还大 —— 超对
		if len(heroRanks) == 2 && heroRanks[0] == paired && heroRanks[1] == paired && paired > boardMax {
			return MadeOverpair
		}
		// 配对的是公牌里最大的那张 —— 顶对
		if paired == boardMax {
			return MadeTopPair
		}
		return MadeWeakPair
	}

	return MadeNone
}

// fullHouseRanks 返回构成葫芦的（三条点数, 对子点数）。ok=false 表示没有葫芦。
//
// 必须单独抽出来：ranksWithCount(counts, 2) 会把**三条那个点数本身**也算成对子，
// 于是「公牌三条 + hero 两张杂牌」会被误判成葫芦 —— 那是最糟的一类错误，
// 它会把一个「人人都有的三条」说成坚果
func fullHouseRanks(counts map[int]int) (int, int, bool) {
	trips := ranksWithCount(counts, 3)
	sort.Sort(sort.Reverse(sort.IntSlice(trips)))
	if len(trips) == 0 {
		return 0, 0, false
	}

	top := trips[0]
	// 两组三条也算葫芦，另一组当对子（如 999 + 888）
	if len(trips) >= 2 {
		return top, trips[1], true
	}

	bestPair := 0
	for v, n := range counts {
		if v != top && n >= 2 && v > bestPair {
			bestPair = v
		}
	}
	if bestPair > 0 {
		return top, bestPair, true
	}
	return 0, 0, false
}

// ---------- 相对档位 ----------

// relativeTier 把绝对等级换算成相对档位：先取基准档位，再按牌面结构逐条降级。
//
// 每条降级都往 notes 里写一行原因 —— 牌力判断直接决定点评口径，
// 必须能回溯「为什么这个顺子只算中等」
func relativeTier(made MadeHand, boardPlays bool, tex BoardTexture, notes *[]string) Tier {
	// 公牌自己成牌是最重的一条：这副牌人人都有，相对牌力接近于零。
	// 直接给弱档，不走下面的逐条降级
	if boardPlays && made > MadeNone {
		addNote(notes, "公牌自己就构成了"+made.String()+"，hero 的两张牌没有让牌变好")
		return TierWeak
	}

	tier := baseTier(made)
	drop := func(reason string) {
		before := tier
		if tier > TierAir {
			tier--
		}
		if tier != before {
			addNote(notes, fmt.Sprintf("%s：%s → %s", reason, before, tier))
		}
	}

	// 同花面：顺子与三条类会被别人的同花压过
	if tex.ThreeFlush && (made == MadeStraight || made == MadeSet || made == MadeTrips) {
		reason := "牌面有三张同花，对手可能是同花"
		if tex.FourFlush {
			reason = "牌面有四张同花，任何持该花色单张的人已成同花"
		}
		drop(reason)
	}

	// 公对：顺子与同花会被葫芦压过
	if tex.Paired && (made == MadeStraight || made == MadeFlush) {
		drop("公牌有对子，对手可能是葫芦")
	}

	// 公牌三条：hero 的三条跟公牌那副是一样的，只比踢脚
	if tex.TripsOnBoard && (made == MadeSet || made == MadeTrips) {
		drop("公牌自己有三条，hero 的三条并不比别人的强多少")
	}

	// 连通面：顶对与超对会被顺子压过；两对会被反超（顶底两对最脆弱）
	if tex.Connected && (made == MadeTopPair || made == MadeOverpair || made == MadeTwoPair) {
		drop("公牌点数连通，对手可能是顺子")
	}

	return tier
}

func baseTier(made MadeHand) Tier {
	switch made {
	case MadeNone:
		return TierAir
	case MadeWeakPair:
		return TierWeak
	case MadeTopPair:
		return TierMedium
	case MadeOverpair, MadeTwoPair:
		return TierStrong
	case MadeSet, MadeTrips, MadeStraight, MadeFlush:
		return TierVeryStrong
	case MadeFullHouse, MadeQuads, MadeStraightFlush:
		return TierNuts
	}
	return TierAir
}

// ---------- 听牌 ----------

func hasFlushDraw(heroSuits, allSuits []byte) bool {
	counts := map[byte]int{}
	for _, s := range allSuits {
		counts[s]++
	}
	for suit, n := range counts {
		// 必须有一张在 hero 手上，否则那是「公牌四张同花」而不是 hero 的听牌
		if n == 4 && suitIn(heroSuits, suit) {
			return true
		}
	}
	return false
}

// hasStraightDraw 差一张就能成顺子。
//
// 判据是「存在某个五连点数集合，其中恰好缺一张」，而不是逐个去猜两头顺/卡顺 ——
// 后者的边界情况（A 当 1、公牌自己成顺、重复点数）太容易漏
func hasStraightDraw(ranks []int) bool {
	present := map[int]bool{}
	for _, v := range ranks {
		present[v] = true
		if v == 14 {
			present[1] = true
		}
	}

	for hi := 14; hi >= 5; hi-- {
		missing := 0
		for k := 0; k < 5; k++ {
			if !present[hi-k] {
				missing++
			}
		}
		if missing == 1 {
			return true
		}
	}
	return false
}

// ---------- 可比分值 ----------

// 比较用的成牌等级。与 MadeHand 的区别：这里不区分 set 与明三条、
// 不区分超对与顶对 —— 那些区分要 hero 上下文，而这里只需要「哪副牌更大」
const (
	scoreHighCard = iota
	scorePair
	scoreTwoPair
	scoreTrips
	scoreStraight
	scoreFlush
	scoreFullHouse
	scoreQuads
	scoreStraightFlush
)

// handScore 返回可比较的牌力分值：[0] 是比较等级，其后是同类内的比较位
// （踢脚或顺子高张），按字典序比较即可
func handScore(ranks []int, suits []byte) [6]int {
	var out [6]int
	if len(ranks) == 0 {
		return out
	}

	counts := map[int]int{}
	for _, v := range ranks {
		counts[v]++
	}

	suitRanks := map[byte][]int{}
	for i, s := range suits {
		suitRanks[s] = append(suitRanks[s], ranks[i])
	}

	// 同花顺
	bestSFHigh := 0
	for _, rs := range suitRanks {
		if len(rs) < 5 {
			continue
		}
		if st := bestStraight(rs); st != nil && st[0] > bestSFHigh {
			bestSFHigh = st[0]
		}
	}
	if bestSFHigh > 0 {
		out[0], out[1] = scoreStraightFlush, bestSFHigh
		return out
	}

	// 四条
	for v, n := range counts {
		if n >= 4 {
			out[0], out[1], out[2] = scoreQuads, v, maxExcept(ranks, v)
			return out
		}
	}

	// 葫芦
	if t, p, ok := fullHouseRanks(counts); ok {
		out[0], out[1], out[2] = scoreFullHouse, t, p
		return out
	}

	// 同花
	if rs, ok := bestFlushRanks(suitRanks); ok {
		out[0] = scoreFlush
		copy(out[1:], rs)
		return out
	}

	// 顺子
	if st := bestStraight(ranks); st != nil {
		out[0], out[1] = scoreStraight, st[0]
		return out
	}

	trips := ranksWithCount(counts, 3)
	sort.Sort(sort.Reverse(sort.IntSlice(trips)))
	pairs := ranksWithCount(counts, 2)
	sort.Sort(sort.Reverse(sort.IntSlice(pairs)))

	// 三条
	if len(trips) > 0 {
		out[0], out[1] = scoreTrips, trips[0]
		kickers := ranksExcept(ranks, trips[0])
		sort.Sort(sort.Reverse(sort.IntSlice(kickers)))
		for i := 0; i < 2 && i < len(kickers); i++ {
			out[2+i] = kickers[i]
		}
		return out
	}

	// 两对
	if len(pairs) >= 2 {
		out[0], out[1], out[2] = scoreTwoPair, pairs[0], pairs[1]
		kickers := ranksExcept(ranks, pairs[0], pairs[1])
		out[3] = maxOf(kickers)
		return out
	}

	// 一对
	if len(pairs) == 1 {
		out[0], out[1] = scorePair, pairs[0]
		kickers := ranksExcept(ranks, pairs[0])
		sort.Sort(sort.Reverse(sort.IntSlice(kickers)))
		for i := 0; i < 3 && i < len(kickers); i++ {
			out[2+i] = kickers[i]
		}
		return out
	}

	// 高牌
	out[0] = scoreHighCard
	kickers := append([]int(nil), ranks...)
	sort.Sort(sort.Reverse(sort.IntSlice(kickers)))
	for i := 0; i < 5 && i < len(kickers); i++ {
		out[1+i] = kickers[i]
	}
	return out
}

// cmpScore 字典序比较两个分值。-1 / 0 / 1
func cmpScore(a, b [6]int) int {
	for i := 0; i < len(a); i++ {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		}
	}
	return 0
}

func bestFlushRanks(suitRanks map[byte][]int) ([]int, bool) {
	var best []int
	for _, rs := range suitRanks {
		if len(rs) < 5 {
			continue
		}
		cur := append([]int(nil), rs...)
		sort.Sort(sort.Reverse(sort.IntSlice(cur)))
		if best == nil {
			best = cur
			continue
		}
		// 同花之间比最大五张，比不出就比最长的（花色张数不同的极端情况）
		if cmpInts(cur[:5], best[:5]) > 0 {
			best = cur
		}
	}
	if best == nil {
		return nil, false
	}
	return best[:5], true
}

// ---------- 小工具 ----------

func rankValue(r byte) int {
	switch {
	case r >= '2' && r <= '9':
		return int(r - '0')
	case r == 'T':
		return 10
	case r == 'J':
		return 11
	case r == 'Q':
		return 12
	case r == 'K':
		return 13
	case r == 'A':
		return 14
	}
	return 0
}

// splitCards 把规范化后的牌串拆成点数与花色。输入需先过 NormalizeCards
func splitCards(cards string) ([]int, []byte) {
	ranks := make([]int, 0, len(cards)/2)
	suits := make([]byte, 0, len(cards)/2)
	for i := 0; i+1 < len(cards); i += 2 {
		ranks = append(ranks, rankValue(cards[i]))
		suits = append(suits, cards[i+1])
	}
	return ranks, suits
}

func splitString(cards string) []string {
	out := make([]string, 0, len(cards)/2)
	for i := 0; i+1 < len(cards); i += 2 {
		out = append(out, cards[i:i+2])
	}
	return out
}

// bestStraight 返回构成顺子的 5 个点数（降序），没有则 nil。
// 优先返回最大的那一个顺子。A 同时按 14 与 1 参与，所以 A2345 能成顺
func bestStraight(ranks []int) []int {
	present := map[int]bool{}
	for _, v := range ranks {
		if v == 0 {
			continue
		}
		present[v] = true
		if v == 14 {
			present[1] = true
		}
	}

	for hi := 14; hi >= 5; hi-- {
		ok := true
		for k := 0; k < 5; k++ {
			if !present[hi-k] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		out := make([]int, 0, 5)
		for k := 0; k < 5; k++ {
			out = append(out, hi-k)
		}
		return out
	}
	return nil
}

func ranksWithCount(counts map[int]int, n int) []int {
	out := make([]int, 0, len(counts))
	for v, c := range counts {
		if c >= n {
			out = append(out, v)
		}
	}
	return out
}

func ranksExcept(ranks []int, exclude ...int) []int {
	out := make([]int, 0, len(ranks))
	for _, v := range ranks {
		skip := false
		for _, e := range exclude {
			if v == e {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, v)
		}
	}
	return out
}

func maxExcept(ranks []int, exclude int) int {
	return maxOf(ranksExcept(ranks, exclude))
}

func maxOf(v []int) int {
	max := 0
	for _, x := range v {
		if x > max {
			max = x
		}
	}
	return max
}

func suitIn(suits []byte, suit byte) bool {
	for _, s := range suits {
		if s == suit {
			return true
		}
	}
	return false
}

func cmpInts(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i] > b[i]:
			return 1
		case a[i] < b[i]:
			return -1
		}
	}
	return 0
}

func addNote(notes *[]string, s string) {
	if notes != nil {
		*notes = append(*notes, s)
	}
}
