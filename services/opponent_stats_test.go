package services

import (
	"call-go/models"
	"testing"
)

const testOpponentID = 7

// statHand 造一手"对手是老王"的手牌。
// 行动里的 hero 用小写 "hero"（models.ActorHero），对手用位置字符串 —— 与前端存的一致
func statHand(id uint, position, cards string, streets ...models.StreetRecord) models.ReviewHand {
	return models.ReviewHand{
		ID:           id,
		TableSize:    9,
		HeroPosition: models.PositionBTN,
		HeroCards:    "AsKh",
		Villains: []models.VillainInfo{
			{Position: position, Name: "老王", OpponentID: testOpponentID, Cards: cards},
		},
		Streets: streets,
	}
}

func statActions(items ...models.StreetAction) []models.StreetAction { return items }

func statAct(actor, action string) models.StreetAction {
	return models.StreetAction{Actor: actor, Action: action}
}

func statBet(actor string, amount float64) models.StreetAction {
	return models.StreetAction{Actor: actor, Action: models.ActionBet, AmountBB: bbPtr(amount)}
}

// 翻前：入池 / 主动加注 / 3bet / 面对加注弃牌，四项口径各不相同，错一个就会污染画像
func TestComputeOpponentStats_Preflop(t *testing.T) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		// 我加注，他跟注 —— 入池但没主动加注
		statHand(1, co, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionCall))}),
		// 他先加注，我跟注 —— 他没面对加注
		statHand(2, co, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))}),
		// 我加注，他 3bet —— 面对加注 + 主动加注 + 3bet
		statHand(3, co, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionRaise),
			statAct(models.ActorHero, models.ActionCall))}),
		// 我加注，他弃牌 —— 不算法入池，但计入面对加注与弃牌
		statHand(4, co, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionFold))}),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)

	if len(stats.Positions) != 1 {
		t.Fatalf("应只有一个位置分层，实际 %d 个", len(stats.Positions))
	}
	ps := stats.Positions[0]
	if ps.Position != co {
		t.Errorf("位置应为 %s，实际 %s", co, ps.Position)
	}
	if ps.Hands != 4 {
		t.Errorf("交手手数应为 4，实际 %d", ps.Hands)
	}
	// 入池：跟注 + 加注 + 3bet 三手，弃牌那手不算
	if ps.Vpip != 3 {
		t.Errorf("入池应为 3，实际 %d", ps.Vpip)
	}
	// 主动加注：先加注那手 + 3bet 那手
	if ps.Pfr != 2 {
		t.Errorf("主动加注应为 2，实际 %d", ps.Pfr)
	}
	// 3bet：只有第三手（我加注后他再加注）
	if ps.ThreeBet != 1 {
		t.Errorf("3bet 应为 1，实际 %d", ps.ThreeBet)
	}
	// 面对加注：一、三、四手。第二手是他自己先加注，不算面对加注
	if ps.FacedRaise != 3 {
		t.Errorf("面对加注应为 3，实际 %d", ps.FacedRaise)
	}
	if ps.FoldToRaise != 1 {
		t.Errorf("面对加注弃牌应为 1，实际 %d", ps.FoldToRaise)
	}
}

// 位置必须分层。合成一个"平均加注率"会把 BTN 的偷盲频率当成他的开池范围，
// 直接导出错误的剥削方案
func TestComputeOpponentStats_SplitsByPosition(t *testing.T) {
	mk := func(id uint, position string) models.ReviewHand {
		return statHand(id, position, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(position, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))})
	}
	hands := []models.ReviewHand{
		mk(1, models.PositionBTN), mk(2, models.PositionBTN), mk(3, models.PositionBTN),
		mk(4, models.PositionUTG),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)

	if len(stats.Positions) != 2 {
		t.Fatalf("应有两个位置分层，实际 %d 个", len(stats.Positions))
	}
	// 次数多的排前面，画像里最该先看的是他最常坐的位置
	if stats.Positions[0].Position != models.PositionBTN || stats.Positions[0].Hands != 3 {
		t.Errorf("BTN 应排第一且为 3 手，实际 %+v", stats.Positions[0])
	}
	if stats.Positions[1].Position != models.PositionUTG || stats.Positions[1].Hands != 1 {
		t.Errorf("UTG 应排第二且为 1 手，实际 %+v", stats.Positions[1])
	}
}

// 认不出人的手牌必须整手跳过。算进去的话，"没记位置的老手牌"会被当成
// 一个从不行动的对手，把入池率整体拉低
func TestComputeOpponentStats_SkipsUnattributable(t *testing.T) {
	hands := []models.ReviewHand{
		// 对手表 id 对得上，但没记位置 —— 认不出哪条行动是他的
		statHand(1, "", "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(models.ActorVillain, models.ActionRaise))}),
		// 是别人（另一个 opponentId）
		{
			ID: 2, TableSize: 9, HeroPosition: models.PositionBTN, HeroCards: "AsKh",
			Villains: []models.VillainInfo{
				{Position: models.PositionCO, Name: "小李", OpponentID: 99},
			},
			Streets: []models.StreetRecord{{Street: models.StreetPreflop, Actions: statActions(
				statAct(models.PositionCO, models.ActionRaise))}},
		},
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	if stats.Hands != 0 {
		t.Errorf("认不出人的手牌应被跳过，实际计入 %d 手", stats.Hands)
	}
}

// 持续下注 + 转牌二次开火。"只有一发子弹"型是这个统计最想抓出来的画像之一
func TestComputeOpponentStats_CbetAndBarrel(t *testing.T) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		// 他翻前加注、翻牌下注、转牌继续下注
		statHand(1, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
				statBet(co, 5), statAct(models.ActorHero, models.ActionCall))},
			models.StreetRecord{Street: models.StreetTurn, Actions: statActions(
				statBet(co, 12), statAct(models.ActorHero, models.ActionFold))},
		),
		// 他翻前加注、翻牌下注，转牌他过牌 —— 只有一发子弹
		statHand(2, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
				statBet(co, 5), statAct(models.ActorHero, models.ActionCall))},
			models.StreetRecord{Street: models.StreetTurn, Actions: statActions(
				statAct(co, models.ActionCheck), statAct(models.ActorHero, models.ActionCheck))},
		),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	post := stats.Postflop

	if post.FlopHands != 2 {
		t.Errorf("进翻牌应为 2 手，实际 %d", post.FlopHands)
	}
	if post.CbetOpportunity != 2 || post.CbetMade != 2 {
		t.Errorf("持续下注应为 2/2，实际 %d/%d", post.CbetMade, post.CbetOpportunity)
	}
	// 第二手转牌他过牌了，所以分母 2、分子 1
	if post.TurnBarrelOpportunity != 2 || post.TurnBarrelMade != 1 {
		t.Errorf("转牌二次开火应为 1/2，实际 %d/%d", post.TurnBarrelMade, post.TurnBarrelOpportunity)
	}
}

// 翻前最后加注者不是他时，不该给他算 cbet 机会 ——
// 他加注后又被我 3bet、翻牌我下注，那这一枪不是他的持续下注
func TestComputeOpponentStats_CbetRequiresLastAggressor(t *testing.T) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		statHand(1, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionRaise),
				statAct(co, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
				statBet(co, 5), statAct(models.ActorHero, models.ActionFold))},
		),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	if stats.Postflop.CbetOpportunity != 0 {
		t.Errorf("翻前最后加注者是我，不该给他算 cbet 机会，实际 %d", stats.Postflop.CbetOpportunity)
	}
}

// 面对我下注的反应分布 + 我过牌后他下注。
// 后者是"他过牌我就下注"这条剥削方案的反向验证
func TestComputeOpponentStats_FacingHero(t *testing.T) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		// 我下注，他加注
		statHand(1, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
				statBet(models.ActorHero, 5), statRaiseTo(co, models.ActionRaise, 15),
				statAct(models.ActorHero, models.ActionCall))},
		),
		// 我过牌，他下注
		statHand(2, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
				statAct(models.ActorHero, models.ActionCheck), statBet(co, 5),
				statAct(models.ActorHero, models.ActionFold))},
		),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	post := stats.Postflop

	if post.FacingHeroBet.Raise != 1 || post.FacingHeroBet.Fold != 0 || post.FacingHeroBet.Call != 0 {
		t.Errorf("面对我下注应为 加 1 / 跟 0 / 弃 0，实际 %+v", post.FacingHeroBet)
	}
	if post.CheckedToOpportunity != 1 || post.BetWhenCheckedTo != 1 {
		t.Errorf("我过牌后他下注应为 1/1，实际 %d/%d",
			post.BetWhenCheckedTo, post.CheckedToOpportunity)
	}
}

// statRaiseTo 构造一个"加注到多少"的动作。raise 的金额语义是"加到多少"，
// 不是增量 —— 这点在尺度统计里被刻意区别对待（见 TestComputeOpponentStats_SizingIgnoresRaises）
func statRaiseTo(actor, action string, amount float64) models.StreetAction {
	return models.StreetAction{Actor: actor, Action: action, AmountBB: bbPtr(amount)}
}

// 尺度分档。分母优先用手填的 PotStartBB，没有才用推算值
func TestComputeOpponentStats_Sizing(t *testing.T) {
	co := models.PositionCO
	mk := func(id uint, amount float64) models.ReviewHand {
		potStart := 6.0
		return statHand(id, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, PotStartBB: &potStart, Actions: statActions(
				statAct(models.ActorHero, models.ActionCheck), statBet(co, amount),
				statAct(models.ActorHero, models.ActionFold))},
		)
	}
	hands := []models.ReviewHand{
		mk(1, 2), // 2/6 ≈ 1/3 池
		mk(2, 3), // 3/6 = 1/2 池
		mk(3, 5), // 5/6 ≈ 0.83 池
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	if len(stats.Sizing) != 3 {
		t.Fatalf("应有 3 个分档，实际 %d", len(stats.Sizing))
	}
	want := []int{1, 1, 1}
	for i, w := range want {
		if stats.Sizing[i].Count != w {
			t.Errorf("分档 %q 应为 %d 次，实际 %d", stats.Sizing[i].Label, w, stats.Sizing[i].Count)
		}
	}
}

// 加注不进尺度分布：raise 记的是"加到多少"，与下注尺度不是同一个量
func TestComputeOpponentStats_SizingIgnoresRaises(t *testing.T) {
	co := models.PositionCO
	potStart := 6.0
	hands := []models.ReviewHand{
		statHand(1, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionCall))},
			models.StreetRecord{Street: models.StreetFlop, PotStartBB: &potStart, Actions: statActions(
				statBet(models.ActorHero, 4), statRaiseTo(co, models.ActionRaise, 18),
				statAct(models.ActorHero, models.ActionFold))},
		),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	for _, bucket := range stats.Sizing {
		if bucket.Count != 0 {
			t.Errorf("加注不该进尺度分布，分档 %q 却有 %d 次", bucket.Label, bucket.Count)
		}
	}
}

// 已知底牌的手牌：成牌等级由 Go 算，不让模型自己看牌面
func TestComputeOpponentStats_Showdown(t *testing.T) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		// 公牌 3 张，他的 AA 是超对
		func() models.ReviewHand {
			h := statHand(1, co, "AhAd",
				models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
					statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionCall))},
				models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
					statAct(models.ActorHero, models.ActionCheck), statBet(co, 5))},
			)
			h.Board = "Qs7h2d"
			return h
		}(),
		// 公牌不足 3 张 —— 评不出成牌，给空串而不是硬说"无成牌"
		func() models.ReviewHand {
			h := statHand(2, co, "3c3d",
				models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
					statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionFold))},
			)
			h.Board = ""
			return h
		}(),
		// 没记底牌的手牌不计入摊牌样本
		statHand(3, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise))}),
	}

	stats := ComputeOpponentStats("老王", hands, testOpponentID)

	if stats.KnownCardsHands != 2 {
		t.Errorf("已知底牌应为 2 手，实际 %d", stats.KnownCardsHands)
	}
	if len(stats.Showdown) != 2 {
		t.Fatalf("摊牌明细应为 2 条，实际 %d", len(stats.Showdown))
	}

	overpair := stats.Showdown[0]
	if overpair.Cards != "AhAd" || overpair.Made != "超对" {
		t.Errorf("第一手应是 AhAd 超对，实际 %+v", overpair)
	}
	if overpair.Tier == "" {
		t.Error("有公牌时相对档位不该为空")
	}
	if !overpair.Aggressive {
		t.Error("他在翻牌下过注，Aggressive 应为 true")
	}

	noBoard := stats.Showdown[1]
	if noBoard.Made != "" || noBoard.Tier != "" {
		t.Errorf("公牌不足 3 张时应留空，实际 %+v", noBoard)
	}
	if noBoard.Aggressive {
		t.Error("他翻前就弃牌了，不该标成翻后主动")
	}
}

// 样本门槛：低于 OpponentThinSampleHands 时提示词要强制降级，这个标志就是判据
func TestComputeOpponentStats_ThinSample(t *testing.T) {
	co := models.PositionCO
	hand := func(id uint) models.ReviewHand {
		return statHand(id, co, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))})
	}

	thin := ComputeOpponentStats("老王", []models.ReviewHand{hand(1), hand(2)}, testOpponentID)
	if !thin.ThinSample {
		t.Error("2 手应判为样本不足")
	}

	enough := []models.ReviewHand{hand(1), hand(2), hand(3), hand(4), hand(5)}
	if ComputeOpponentStats("老王", enough, testOpponentID).ThinSample {
		t.Error("5 手不该再判为样本不足")
	}
}
