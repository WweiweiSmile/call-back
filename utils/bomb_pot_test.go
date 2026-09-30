package utils

import (
	"testing"

	"call-go/models"
)

// bombPotHand 一手 6 人桌的爆炸底池：没有翻前行动，每人先投 5bb 直接看翻牌。
//
// 刻意不用 validHand 的翻前街 —— 爆炸底池与"翻前有行动"是互斥的（校验层拦着），
// 拿一份带翻前的牌去测会把不存在的局面当成合法输入
func bombPotHand() *models.ReviewHand {
	h := validHand()
	h.TableSize = 6
	h.BombPotBB = 5
	h.Streets = []models.StreetRecord{
		{Street: models.StreetFlop, Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionBet, AmountBB: bb(10)},
			{Actor: models.ActorVillain, Action: models.ActionCall},
		}},
	}
	return h
}

// 起始底池 = 每人先投 × 人数，且不看盲注与前注
func TestPreflopPotBBForBombPot(t *testing.T) {
	if got := (models.BlindConfig{TableSize: 6, BombPotBB: 5}).PreflopPotBB(); got != 30 {
		t.Errorf("6 人桌爆炸底池的起始底池应为 5×6=30，实际 %v", got)
	}

	// 盲注与前注即便有值也不参与：校验层不允许这种组合，这里再钉一次优先级，
	// 免得以后有人在 PreflopPotBB 里把它们加回去（那会让起始底池偏大）
	mixed := models.BlindConfig{
		TableSize: 6, BombPotBB: 5,
		SmallBlindBB: 0.5, BigBlindBB: 1, AnteBB: 1,
	}
	if got := mixed.PreflopPotBB(); got != 30 {
		t.Errorf("爆炸底池不应叠加盲注与前注，应为 30，实际 %v", got)
	}

	// 人数没记录时算不出总额，返回 0 而不是凭空按 1 人折算
	if got := (models.BlindConfig{BombPotBB: 5}).PreflopPotBB(); got != 0 {
		t.Errorf("人数为 0 时起始底池应为 0，实际 %v", got)
	}

	// 回归：常规牌局的口径一字未改（小盲 + 大盲 + 前注 × 人数）
	plain := models.BlindConfig{TableSize: 9, SmallBlindBB: 0.5, BigBlindBB: 1, AnteBB: 1}
	if got := plain.PreflopPotBB(); got != 10.5 {
		t.Errorf("常规牌局起始底池应为 0.5+1+9=10.5，实际 %v", got)
	}
}

// 爆炸底池没人下盲注，棋盘必须是空的。
//
// 摆了棋盘就会把"某人已投入"记成盲注额，翻前的跟注差额随之算错 ——
// 而爆炸底池根本没有翻前行动，这份错误会静默地传到底池里
func TestPostedBlindsEmptyForBombPot(t *testing.T) {
	b := models.BlindConfig{
		TableSize: 6, BombPotBB: 5,
		SmallBlindBB: 0.5, BigBlindBB: 1,
		HeroPosition:     models.PositionBB,
		VillainPositions: []string{models.PositionSB},
	}
	if got := b.PostedBlinds(); len(got) != 0 {
		t.Errorf("爆炸底池不该有任何盲注记账，实际 %v", got)
	}
}

// 死钱只进一次：起始底池就是 5×人数，翻牌的行动在此之上累加
func TestComputeStreetPotsForBombPot(t *testing.T) {
	h := bombPotHand()
	pots := ComputeStreetPots(h.Streets, h.Blinds())

	preflop := pots[models.StreetPreflop]
	if preflop.PotStartBB != 30 || preflop.PotEndBB != 30 {
		t.Errorf("翻前没有行动，底池应恒为 30，实际 %v → %v", preflop.PotStartBB, preflop.PotEndBB)
	}

	flop := pots[models.StreetFlop]
	if flop.PotStartBB != 30 {
		t.Errorf("翻牌起始底池应为 30（爆炸底池的死钱），实际 %v", flop.PotStartBB)
	}
	// 我下注 10、对手跟注 10
	if flop.PotEndBB != 50 {
		t.Errorf("翻牌结束底池应为 30+20=50，实际 %v", flop.PotEndBB)
	}
}

// 没有翻前行动 + 有翻牌本身是合法的（爆炸底池就是这个形状），不该被拦
func TestValidateReviewHandAcceptsBombPot(t *testing.T) {
	if err := ValidateReviewHand(bombPotHand()); err != nil {
		t.Errorf("合法的爆炸底池手牌不该校验失败：%v", err)
	}
}

// 爆炸底池与盲注互斥：同时有值是自相矛盾的数据，会按两套口径各算一遍底池
func TestValidateBombPot(t *testing.T) {
	cases := []struct {
		name                      string
		bombPot, small, big, ante float64
		wantErr                   bool
	}{
		{"不是爆炸底池", 0, 0.5, 1, 0, false},
		{"没记录盲注的常规手牌", 0, 0, 0, 0, false},
		{"爆炸底池", 5, 0, 0, 0, false},
		{"额度为负", -1, 0, 0, 0, true},
		{"与小盲冲突", 5, 0.5, 0, 0, true},
		{"与大盲冲突", 5, 0, 1, 0, true},
		{"与前注冲突", 5, 0, 0, 1, true},
	}

	for _, tc := range cases {
		err := ValidateBombPot(tc.bombPot, tc.small, tc.big, tc.ante)
		if tc.wantErr && err == nil {
			t.Errorf("%s：应当报错，却通过了", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s：不该报错，却失败了：%v", tc.name, err)
		}
	}
}

// 爆炸底池记了翻前行动就是在描述一手不存在的牌局。
//
// 前端勾上开关时会清空翻前，这条拦的是绕过前端的提交 ——
// 放进去的话底池与 AI 点评都会错得没法解释
func TestValidateReviewHandRejectsPreflopActionsInBombPot(t *testing.T) {
	h := bombPotHand()
	h.Streets = append(h.Streets, models.StreetRecord{
		Street: models.StreetPreflop,
		Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bb(3)},
		},
	})
	if err := ValidateReviewHand(h); err == nil {
		t.Error("爆炸底池带翻前行动应当校验失败")
	}

	// 去掉翻前行动后应恢复合法，确认上一条失败的原因就是它
	h.Streets = h.Streets[:1]
	if err := ValidateReviewHand(h); err != nil {
		t.Errorf("清空翻前后应通过校验：%v", err)
	}
}

// 非爆炸底池手牌的指纹**必须**与加这个功能之前逐字一致。
//
// 期望值是 2026-09-29 加 BombPotBB 之前实测的 ComputeHandHash(validHand())：
// 量法是临时把 BombPotBB 从指纹载荷里摘掉、跑一次把值抄下来再还原。
// 它守的是：新字段若忘了带 omitempty，每一条老手牌的指纹都会变，
// 用户一打开旧牌再保存就被判成"内容变了"，白花一次模型额度重分析
const wantLegacyHash = "95b640acb24c940dc16a2dc62ff617782142e34870b3ad39df681a81216518d2"

func TestComputeHandHashIsUnchangedForNonBombPot(t *testing.T) {
	if got := ComputeHandHash(validHand()); got != wantLegacyHash {
		t.Errorf("非爆炸底池手牌的指纹变了（%s），老手牌的分析会被误判成过期。\n"+
			"多半是 BombPotBB 的 omitempty 被去掉了 —— 值 0 时必须不出现在指纹载荷里", got)
	}
}

// 爆炸底池标记进了底池与提示词，就必须进指纹：改了它不重算，旧结论会一直挂着
func TestComputeHandHashCoversBombPot(t *testing.T) {
	plain := validHand()
	withBombPot := validHand()
	withBombPot.BombPotBB = 5

	if ComputeHandHash(plain) == ComputeHandHash(withBombPot) {
		t.Error("改爆炸底池标记后指纹必须变化")
	}

	// 同样是爆炸底池、其它内容一致时必须同指纹，否则会白花一次模型调用
	a := validHand()
	a.BombPotBB = 5
	b := validHand()
	b.BombPotBB = 5
	if ComputeHandHash(a) != ComputeHandHash(b) {
		t.Error("爆炸底池额度相同时指纹应一致")
	}

	// 额度不同要换指纹：5bb 与 10bb 的牌局体量不同，结论不能复用
	c := validHand()
	c.BombPotBB = 10
	if ComputeHandHash(a) == ComputeHandHash(c) {
		t.Error("爆炸底池额度变化必须改变指纹")
	}
}
