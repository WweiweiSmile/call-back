package services

import (
	"call-go/models"
	"call-go/utils"
	"sort"
	"time"
)

// 样本量门槛。两个数字都直接写进提示词，模型据此决定敢不敢下结论。
const (
	// OpponentMinSamplesForRead 方法论里"同一模式出现 3-4 次才算可剥削"的下限。
	// 低于它的观察只能进 unknowns，不许作为剥削方案的依据
	OpponentMinSamplesForRead = 3
	// OpponentThinSampleHands 总交手手数低于它 = 整体样本不足，
	// 提示词据此把形象强制打成 unknown、置信度打成 low
	OpponentThinSampleHands = 5
)

// 下注尺度分档的边界（相对该街起始底池）
var sizingBuckets = []struct {
	Label string
	Max   float64 // 上界，取严格小于；最后一档用 +Inf
}{
	{"1/3 池及以下", 1.0/3 + 1e-9},
	{"1/3 ~ 2/3 池", 2.0/3 + 1e-9},
	{"2/3 池以上", 0}, // 0 表示无上界
}

// ComputeOpponentStats 从手牌记录里算出某个对手的量化画像。
//
// 纯函数，不碰数据库：数据取好之后这里只做统计，测试可以直接构造手牌喂进来。
// hands 应当是"villains 里含该 opponentID"的全部手牌。
func ComputeOpponentStats(name string, hands []models.ReviewHand, opponentID uint) *models.OpponentStats {
	stats := &models.OpponentStats{
		Name:      name,
		Positions: []models.OpponentPositionStat{},
		Sizing:    []models.OpponentSizingBucket{},
		Showdown:  []models.OpponentShowdownItem{},
	}
	for _, b := range sizingBuckets {
		stats.Sizing = append(stats.Sizing, models.OpponentSizingBucket{Label: b.Label})
	}

	posStats := map[string]*models.OpponentPositionStat{}
	var first, last time.Time

	for i := range hands {
		hand := &hands[i]
		villain := findVillainByOpponentID(hand.Villains, opponentID)
		// 没有位置就认不出这手牌里哪条行动是他的。老数据走这条路，
		// 计数偏小是预期内的（对手名单里的"已交手 N 手"也是同一口径）
		if villain == nil || villain.Position == "" {
			continue
		}

		stats.Hands++
		if first.IsZero() || hand.CreatedAt.Before(first) {
			first = hand.CreatedAt
		}
		if hand.CreatedAt.After(last) {
			last = hand.CreatedAt
		}

		ps := posStats[villain.Position]
		if ps == nil {
			ps = &models.OpponentPositionStat{Position: villain.Position}
			posStats[villain.Position] = ps
		}
		ps.Hands++

		collectPreflop(ps, hand, villain.Position)
		collectPostflop(&stats.Postflop, hand, villain.Position)
		collectSizing(stats.Sizing, hand, villain.Position)
		collectShowdown(stats, hand, villain)
	}

	for _, ps := range posStats {
		stats.Positions = append(stats.Positions, *ps)
	}
	// 出现次数多的位置排前面：画像里最该先看的是他最常坐的位置
	sort.Slice(stats.Positions, func(i, j int) bool {
		if stats.Positions[i].Hands != stats.Positions[j].Hands {
			return stats.Positions[i].Hands > stats.Positions[j].Hands
		}
		return stats.Positions[i].Position < stats.Positions[j].Position
	})

	stats.ThinSample = stats.Hands < OpponentThinSampleHands
	if !first.IsZero() {
		stats.FirstHandAt = first.Format("2006-01-02")
	}
	if !last.IsZero() {
		stats.LastHandAt = last.Format("2006-01-02")
	}
	return stats
}

// collectPreflop 累加一手牌的翻前统计。
//
// 用"他之前已经有几次加注"作为面对加注的判据，而不是看他前面有没有人加注：
// 后者在他自己就是那个加注者时会把他的加注动作也算成"面对加注"，
// 3bet 率与面对加注弃牌率会一起失真
func collectPreflop(ps *models.OpponentPositionStat, hand *models.ReviewHand, position string) {
	pre := streetRecordOf(hand.Streets, models.StreetPreflop)
	if pre == nil {
		return
	}

	raisesBefore := 0
	handVpip := false
	for _, action := range pre.Actions {
		if action.Actor != position {
			if isAggressiveAction(action.Action) {
				raisesBefore++
			}
			continue
		}

		if isVoluntaryAction(action.Action) {
			handVpip = true
		}
		// 先把他自己这一手排除在外：他加注的那一刻并没有"面对加注"。
		// 把自增写在判断之前，他自己的加注会被算成面对加注，
		// 3bet 率与面对加注弃牌率会一起失真
		facedRaise := raisesBefore > 0

		if isAggressiveAction(action.Action) {
			ps.Pfr++
			if facedRaise {
				ps.ThreeBet++
			}
			raisesBefore++
		}
		if facedRaise {
			ps.FacedRaise++
			if action.Action == models.ActionFold {
				ps.FoldToRaise++
			}
		}
	}
	if handVpip {
		ps.Vpip++
	}
}

// collectPostflop 累加翻后统计
func collectPostflop(out *models.OpponentPostflopStat, hand *models.ReviewHand, position string) {
	pre := streetRecordOf(hand.Streets, models.StreetPreflop)
	flop := streetRecordOf(hand.Streets, models.StreetFlop)
	turn := streetRecordOf(hand.Streets, models.StreetTurn)

	if flop != nil && hasActionBy(flop, position) {
		out.FlopHands++

		// 持续下注：他是翻前最后一个加注者，翻牌他第一个动作就是下注。
		// "最后一个"很关键 —— 他翻前加注后又被别人再加注，那这枪就不是他的 cbet 了
		if lastPreflopAggressor(pre) == position {
			out.CbetOpportunity++
			first := firstActionBy(flop, position)
			if first != nil && first.Action == models.ActionBet {
				out.CbetMade++

				// 转牌二次开火。分母要求"转牌还有他行动"：翻牌那一枪若是
				// 全弃牌，这手牌根本不会有转牌，他也就没有第二次机会
				if turn != nil && hasActionBy(turn, position) {
					out.TurnBarrelOpportunity++
					if next := firstActionBy(turn, position); next != nil && next.Action == models.ActionBet {
						out.TurnBarrelMade++
					}
				}
			}
		}
	}

	// 面对我的下注 / 我过牌后他下注。逐街扫，找"我做完动作之后他的第一个动作"
	for _, street := range []string{models.StreetFlop, models.StreetTurn, models.StreetRiver} {
		record := streetRecordOf(hand.Streets, street)
		if record == nil {
			continue
		}
		for i := range record.Actions {
			action := record.Actions[i]
			if action.Actor != models.ActorHero {
				continue
			}
			isBet := isAggressiveAction(action.Action)
			isCheck := action.Action == models.ActionCheck
			if !isBet && !isCheck {
				continue
			}

			next := nextActionBy(record.Actions[i+1:], position)
			if next == nil {
				continue
			}
			if isCheck {
				out.CheckedToOpportunity++
				if next.Action == models.ActionBet {
					out.BetWhenCheckedTo++
				}
				continue
			}
			switch next.Action {
			case models.ActionFold:
				out.FacingHeroBet.Fold++
			case models.ActionCall:
				out.FacingHeroBet.Call++
			case models.ActionRaise, models.ActionAllin:
				out.FacingHeroBet.Raise++
			}
		}
	}
}

// collectSizing 累加下注尺度。只统计 bet，理由见 models.OpponentStats.Sizing 的说明
func collectSizing(buckets []models.OpponentSizingBucket, hand *models.ReviewHand, position string) {
	pots := utils.ComputeStreetPots(hand.Streets, hand.Blinds())

	for _, street := range []string{models.StreetPreflop, models.StreetFlop, models.StreetTurn, models.StreetRiver} {
		record := streetRecordOf(hand.Streets, street)
		if record == nil {
			continue
		}
		for _, action := range record.Actions {
			if action.Actor != position || action.Action != models.ActionBet || action.AmountBB == nil {
				continue
			}
			potStart := streetPotStart(record, pots, street)
			if potStart <= 0 {
				// 底池推不出来（没记盲注又没记前面的行动）时这手不计入，
				// 硬拿一个 0 或猜一个数当分母会产出一堆假的比例
				continue
			}
			ratio := *action.AmountBB / potStart
			for i := range buckets {
				if i == len(buckets)-1 || ratio < sizingBuckets[i].Max {
					buckets[i].Count++
					break
				}
			}
		}
	}
}

// collectShowdown 记录"看到了他底牌"的手牌
func collectShowdown(stats *models.OpponentStats, hand *models.ReviewHand, villain *models.VillainInfo) {
	if villain.Cards == "" {
		return
	}
	stats.KnownCardsHands++

	item := models.OpponentShowdownItem{
		HandID:     hand.ID,
		Cards:      villain.Cards,
		Position:   villain.Position,
		Board:      hand.Board,
		Result:     hand.Result,
		Aggressive: hasPostflopAggression(hand.Streets, villain.Position),
	}
	// 公牌不足 3 张就评不出成牌（翻前结束的手牌）。这时 Made/Tier 留空，
	// 而不是硬给一个"无成牌"——那会把"没法评估"说成"他什么都没有"
	if len(hand.Board)/2 >= 3 {
		if strength, err := utils.EvaluateHand(villain.Cards, hand.Board); err == nil {
			item.Made = strength.Made.String()
			item.Tier = strength.Tier.String()
		}
	}
	stats.Showdown = append(stats.Showdown, item)
}

// ---------- 小工具 ----------

func findVillainByOpponentID(villains []models.VillainInfo, opponentID uint) *models.VillainInfo {
	for i := range villains {
		if villains[i].OpponentID == opponentID {
			return &villains[i]
		}
	}
	return nil
}

func streetRecordOf(streets []models.StreetRecord, street string) *models.StreetRecord {
	for i := range streets {
		if streets[i].Street == street {
			return &streets[i]
		}
	}
	return nil
}

func isAggressiveAction(action string) bool {
	return action == models.ActionBet || action == models.ActionRaise || action == models.ActionAllin
}

func isVoluntaryAction(action string) bool {
	return action == models.ActionCall || isAggressiveAction(action)
}

func hasActionBy(record *models.StreetRecord, actor string) bool {
	return firstActionBy(record, actor) != nil
}

func firstActionBy(record *models.StreetRecord, actor string) *models.StreetAction {
	if record == nil {
		return nil
	}
	for i := range record.Actions {
		if record.Actions[i].Actor == actor {
			return &record.Actions[i]
		}
	}
	return nil
}

// nextActionBy 在 actions 里找该行动者的第一个动作。
// 用在"我做过某个动作之后，他做了什么"这种时序判断上
func nextActionBy(actions []models.StreetAction, actor string) *models.StreetAction {
	for i := range actions {
		if actions[i].Actor == actor {
			return &actions[i]
		}
	}
	return nil
}

// lastPreflopAggressor 翻前最后一个主动加注的人。
// 返回空串表示翻前没人加注（全都跟注或弃牌），此时谁都不算 cbet 机会
func lastPreflopAggressor(pre *models.StreetRecord) string {
	if pre == nil {
		return ""
	}
	last := ""
	for _, action := range pre.Actions {
		if isAggressiveAction(action.Action) {
			last = action.Actor
		}
	}
	return last
}

// hasPostflopAggression 他在翻后有没有主动下注或加注过
func hasPostflopAggression(streets []models.StreetRecord, position string) bool {
	for _, street := range []string{models.StreetFlop, models.StreetTurn, models.StreetRiver} {
		record := streetRecordOf(streets, street)
		if record == nil {
			continue
		}
		for _, action := range record.Actions {
			if action.Actor == position && isAggressiveAction(action.Action) {
				return true
			}
		}
	}
	return false
}

// streetPotStart 取某条街开始时的底池。
// 优先用记录里手填的 PotStartBB，没有就用推算值
func streetPotStart(record *models.StreetRecord, pots map[string]utils.PotStep, street string) float64 {
	if record.PotStartBB != nil {
		return *record.PotStartBB
	}
	if step, ok := pots[street]; ok {
		return step.PotStartBB
	}
	return 0
}
