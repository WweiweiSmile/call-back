package utils

import (
	"call-go/models"
	"math"
	"testing"
)

// 前注是每人一份，总额必须乘人数。
// 只按一份算会让前注局的底池系统性偏小，赔率推理跟着一起错
func TestBlindConfigPreflopPot(t *testing.T) {
	cases := []struct {
		name   string
		blinds models.BlindConfig
		want   float64
	}{
		{
			name:   "没记盲注",
			blinds: models.BlindConfig{},
			want:   0,
		},
		{
			name:   "只有小盲大盲",
			blinds: models.BlindConfig{SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9},
			want:   1.5,
		},
		{
			name:   "9人桌打1bb前注",
			blinds: models.BlindConfig{SmallBlindBB: 0.5, BigBlindBB: 1, AnteBB: 1, TableSize: 9},
			want:   10.5, // 1.5 + 1×9
		},
		{
			name:   "2人桌打0.2bb前注",
			blinds: models.BlindConfig{SmallBlindBB: 0.5, BigBlindBB: 1, AnteBB: 0.2, TableSize: 2},
			want:   1.9, // 1.5 + 0.2×2
		},
		{
			// 人数缺失时前注折算不出来，此时不该凭空加一份
			name:   "前注但人数为0",
			blinds: models.BlindConfig{SmallBlindBB: 0.5, BigBlindBB: 1, AnteBB: 1, TableSize: 0},
			want:   1.5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.blinds.PreflopPotBB()
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("前注底池应为 %v，实际 %v", tc.want, got)
			}
		})
	}
}

// IsZero 是"要不要按含盲注口径展示"的开关，判定错了文案就会撒谎
func TestBlindConfigIsZero(t *testing.T) {
	if !(models.BlindConfig{TableSize: 9}).IsZero() {
		t.Error("三个额度都是 0 时应判定为未记录")
	}
	if (models.BlindConfig{BigBlindBB: 1, TableSize: 9}).IsZero() {
		t.Error("只填了大盲也应判定为已记录")
	}
	if (models.BlindConfig{AnteBB: 0.5, TableSize: 9}).IsZero() {
		t.Error("只填了前注也应判定为已记录")
	}
}

// 认人的规则：位置对得上才记账。认不出来的宁可不记，也不能替没记录的人下注
func TestBlindConfigPostedBlinds(t *testing.T) {
	blinds := models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9,
		HeroPosition: models.PositionBB, KeyVillainPosition: models.PositionSB,
	}
	posted := blinds.PostedBlinds()
	if posted[models.ActorHero] != 1 {
		t.Errorf("hero 在 BB，应记 1bb，实际 %v", posted[models.ActorHero])
	}
	if posted[models.ActorVillain] != 0.5 {
		t.Errorf("关键对手在 SB，应记 0.5bb，实际 %v", posted[models.ActorVillain])
	}

	// 关键对手在 CO：两个盲注都不在已知的人头上，谁都不记
	unknown := models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9,
		HeroPosition: models.PositionBTN, KeyVillainPosition: models.PositionCO,
	}
	if got := unknown.PostedBlinds(); len(got) != 0 {
		t.Errorf("大小盲都认不出人时不该记到任何行动者头上，实际 %v", got)
	}
}

// 盲注必须真的进底池：翻前的 PotStartBB 不再是 0
func TestComputeStreetPotsIncludesBlinds(t *testing.T) {
	streets := []models.StreetRecord{
		{
			Street: models.StreetPreflop,
			Actions: []models.StreetAction{
				// BTN（hero）加注到 3bb
				{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bb(3)},
			},
		},
		{
			Street: models.StreetFlop,
			Actions: []models.StreetAction{
				{Actor: models.ActorHero, Action: models.ActionBet, AmountBB: bb(5)},
			},
		},
	}

	withBlinds := ComputeStreetPots(streets, models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9, HeroPosition: models.PositionBTN,
	})
	withoutBlinds := ComputeStreetPots(streets, models.BlindConfig{})

	// 开局 1.5bb 死钱，hero 再放 3bb
	if got := withBlinds[models.StreetPreflop].PotStartBB; math.Abs(got-1.5) > 1e-9 {
		t.Errorf("翻前起始底池应为 1.5，实际 %v", got)
	}
	if got := withBlinds[models.StreetPreflop].PotEndBB; math.Abs(got-4.5) > 1e-9 {
		t.Errorf("翻前结束底池应为 4.5，实际 %v", got)
	}
	if got := withBlinds[models.StreetRiver].PotEndBB; math.Abs(got-9.5) > 1e-9 {
		t.Errorf("最终底池应为 9.5，实际 %v", got)
	}

	// 没记盲注时行为必须和加这个功能之前一模一样，否则存量手牌的分析结论会整体偏移
	if got := withoutBlinds[models.StreetPreflop].PotStartBB; got != 0 {
		t.Errorf("未记录盲注时翻前起始底池应为 0，实际 %v", got)
	}
	if got := withoutBlinds[models.StreetRiver].PotEndBB; math.Abs(got-8) > 1e-9 {
		t.Errorf("未记录盲注时最终底池应为 8，实际 %v", got)
	}
}

// 这是整个盲注功能里最容易做错的一处，单独钉死。
//
// SB 0.5 / BB 1 / BTN 加注到 3 / SB 弃牌 / BB 跟注：真实底池 = 0.5+1+3+2 = 6.5。
// 若只把盲注当死钱、不记到 BB 头上，BB 的跟注会被算成再掏 3bb，得到 7.5 —— 比
// 完全不记盲注的 6 偏得还多。盲注必须记到人头上，跟注才只补差额
func TestBigBlindCallIsNotDoubleCounted(t *testing.T) {
	streets := []models.StreetRecord{{
		Street: models.StreetPreflop,
		Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bb(3)}, // BTN 加注到 3
			{Actor: models.ActorOther, Action: models.ActionFold},                  // SB 弃牌
			{Actor: models.ActorVillain, Action: models.ActionCall},                // BB 跟注
		},
	}}

	// BB 是关键对手，位置已知，能认出来
	pots := ComputeStreetPots(streets, models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9,
		HeroPosition: models.PositionBTN, KeyVillainPosition: models.PositionBB,
	})

	if got := pots[models.StreetPreflop].PotEndBB; math.Abs(got-6.5) > 1e-9 {
		t.Errorf("底池应为 6.5（BB 只需补 2bb），实际 %v", got)
	}

	// 对照：BB 的身份认不出来时退化成近似值。
	// 这时按死钱算会得到 7.5，比不记盲注更偏 —— 所以 UI 上要鼓励用户标出关键对手位置
	approx := ComputeStreetPots(streets, models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9, HeroPosition: models.PositionBTN,
	})
	if got := approx[models.StreetPreflop].PotEndBB; math.Abs(got-7.5) > 1e-9 {
		t.Errorf("认不出大盲时应退化为 7.5 的近似值，实际 %v", got)
	}
}

// 大盲过牌是免费看翻牌：大盲的 1bb 已在 contributed 里，currentBet 也是 1bb，
// 过牌本身不投入。若忘了预设 currentBet，后面第一个跟注的人会少算
func TestBigBlindCheckIsFree(t *testing.T) {
	streets := []models.StreetRecord{{
		Street: models.StreetPreflop,
		Actions: []models.StreetAction{
			{Actor: models.ActorOther, Action: models.ActionFold}, // SB 弃牌
			{Actor: models.ActorHero, Action: models.ActionCheck}, // BB（hero）过牌
		},
	}}

	pots := ComputeStreetPots(streets, models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, TableSize: 9, HeroPosition: models.PositionBB,
	})

	// 弃牌与过牌都不投入，底池停在死钱 1.5bb
	if got := pots[models.StreetPreflop].PotEndBB; math.Abs(got-1.5) > 1e-9 {
		t.Errorf("大盲过牌后底池应为 1.5，实际 %v", got)
	}
}

// 前注不参与跟注抵消：下了前注的人仍要补足到开池额
func TestAnteDoesNotOffsetCallAmount(t *testing.T) {
	streets := []models.StreetRecord{{
		Street: models.StreetPreflop,
		Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bb(3)}, // BTN 加注到 3
			{Actor: models.ActorVillain, Action: models.ActionCall},                // BB 跟注
		},
	}}

	// SB 0.5 + BB 1 + 前注 1×9人 = 10.5 死钱
	pots := ComputeStreetPots(streets, models.BlindConfig{
		SmallBlindBB: 0.5, BigBlindBB: 1, AnteBB: 1, TableSize: 9,
		HeroPosition: models.PositionBTN, KeyVillainPosition: models.PositionBB,
	})

	// 10.5 + BTN 的 3 + BB 补的 2 = 15.5。前注若被拿去抵消，BB 只会补 1
	if got := pots[models.StreetPreflop].PotEndBB; math.Abs(got-15.5) > 1e-9 {
		t.Errorf("底池应为 15.5，实际 %v", got)
	}
}
