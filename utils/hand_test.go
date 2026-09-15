package utils

import (
	"call-go/models"
	"testing"
)

func bb(v float64) *float64 { return &v }

// ptrUint 便于构造可空外键
func ptrUint(v uint) *uint { return &v }

func validHand() *models.ReviewHand {
	return &models.ReviewHand{
		HeroPosition: models.PositionBTN,
		HeroCards:    "AsKh",
		HeroStackBB:  100,
		Board:        "Qs7h2d",
		VillainCount: 1,
		Streets: []models.StreetRecord{
			{Street: models.StreetPreflop, Actions: []models.StreetAction{
				{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bb(2.5)},
				{Actor: models.ActorVillain, Action: models.ActionCall},
			}},
			{Street: models.StreetFlop, Actions: []models.StreetAction{
				{Actor: models.ActorVillain, Action: models.ActionCheck},
				{Actor: models.ActorHero, Action: models.ActionBet, AmountBB: bb(4)},
			}},
		},
	}
}

func TestNormalizeCards(t *testing.T) {
	cases := map[string]string{
		"AsKh":     "AsKh",
		"as kh":    "AsKh",
		"AS, KH":   "AsKh",
		"AH KD QS": "AhKdQs",
		"10s":      "Ts", // 用户习惯写 10，规范化成 T
	}
	for input, want := range cases {
		if got := NormalizeCards(input); got != want {
			t.Errorf("NormalizeCards(%q) = %q, 期望 %q", input, got, want)
		}
	}
}

func TestValidateReviewHand_OK(t *testing.T) {
	h := validHand()
	if err := ValidateReviewHand(h); err != nil {
		t.Fatalf("合法手牌不应报错，实际: %v", err)
	}
	// PotType 未填时按对手数量推断
	if h.PotType != models.PotTypeHU {
		t.Errorf("对手数量为 1 时应推断为单挑，实际 %q", h.PotType)
	}
	if h.Result != models.ResultUnknown {
		t.Errorf("结果未填时应默认为 unknown，实际 %q", h.Result)
	}
}

func TestValidateReviewHand_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*models.ReviewHand)
	}{
		{"底牌只有一张", func(h *models.ReviewHand) { h.HeroCards = "As" }},
		{"底牌点数非法", func(h *models.ReviewHand) { h.HeroCards = "XsKh" }},
		{"底牌花色非法", func(h *models.ReviewHand) { h.HeroCards = "AxKh" }},
		{"公共牌 2 张", func(h *models.ReviewHand) { h.Board = "Qs7h" }},
		{"底牌与公共牌重复", func(h *models.ReviewHand) { h.Board = "As7h2d" }},
		{"公共牌自身重复", func(h *models.ReviewHand) { h.Board = "QsQs2d" }},
		{"非法位置", func(h *models.ReviewHand) { h.HeroPosition = "MP1" }},
		// MP 已被 LJ/HJ 取代：它能同时读成两个位置，交给模型就是歧义
		{"MP 已废弃", func(h *models.ReviewHand) { h.HeroPosition = "MP" }},
		{"人数只有 1 人", func(h *models.ReviewHand) { h.TableSize = 1 }},
		{"人数超过 9 人", func(h *models.ReviewHand) { h.TableSize = 10 }},
		{"人数为负", func(h *models.ReviewHand) { h.TableSize = -1 }},
		// 位置必须落在该人数的位置上，否则 UI 选了 6 人桌却存进 LJ 这种自相矛盾的数据
		{"6 人桌用 7 人桌才有的 LJ", func(h *models.ReviewHand) {
			h.TableSize = 6
			h.HeroPosition = models.PositionLJ
		}},
		{"7 人桌用 8 人桌才有的 UTG+1", func(h *models.ReviewHand) {
			h.TableSize = 7
			h.HeroPosition = models.PositionUTG1
		}},
		{"2 人桌用 BTN（应记为 SB）", func(h *models.ReviewHand) {
			h.TableSize = 2
			h.HeroPosition = models.PositionBTN
		}},
		{"对手位置不属于该人数", func(h *models.ReviewHand) {
			h.TableSize = 6
			h.HeroPosition = models.PositionBTN
			h.Villains = []models.VillainInfo{{Position: models.PositionUTG2, IsKey: true}}
		}},
		{"非法结果", func(h *models.ReviewHand) { h.Result = "draw" }},
		{"负筹码", func(h *models.ReviewHand) { h.HeroStackBB = -1 }},
		{"非法街道", func(h *models.ReviewHand) { h.Streets[0].Street = "flopp" }},
		{"非法动作", func(h *models.ReviewHand) { h.Streets[0].Actions[0].Action = "shove" }},
		{"非法行动者", func(h *models.ReviewHand) { h.Streets[0].Actions[0].Actor = "me" }},
		{"下注缺金额", func(h *models.ReviewHand) { h.Streets[1].Actions[1].AmountBB = nil }},
		{"下注金额为零", func(h *models.ReviewHand) { h.Streets[1].Actions[1].AmountBB = bb(0) }},
		{"打了转牌却没录转牌公共牌", func(h *models.ReviewHand) {
			h.Streets = append(h.Streets, models.StreetRecord{
				Street:  models.StreetTurn,
				Actions: []models.StreetAction{{Actor: models.ActorHero, Action: models.ActionCheck}},
			})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := validHand()
			tc.mutate(h)
			if err := ValidateReviewHand(h); err == nil {
				t.Errorf("%s：应该报错但没有", tc.name)
			}
		})
	}
}

func TestValidateReviewHand_AllowsNoBoard(t *testing.T) {
	// 翻前就结束的手牌（比如翻前全下后弃牌）没有公共牌，必须允许
	h := validHand()
	h.Board = ""
	h.Streets = h.Streets[:1]
	if err := ValidateReviewHand(h); err != nil {
		t.Fatalf("无公共牌的手牌应合法，实际: %v", err)
	}
}

func TestValidateReviewHand_RejectsOverlongThought(t *testing.T) {
	h := validHand()
	thought := make([]rune, 1001)
	for i := range thought {
		thought[i] = 'a'
	}
	h.HeroThought = string(thought)
	if err := ValidateReviewHand(h); err == nil {
		t.Error("超过 1000 字的想法应该被拒绝")
	}
}

func TestValidateReviewHand_NormalizesCardsInPlace(t *testing.T) {
	h := validHand()
	h.HeroCards = "as kh"
	h.Board = "qs 7h 2d"
	if err := ValidateReviewHand(h); err != nil {
		t.Fatalf("小写输入应被接受并规范化，实际: %v", err)
	}
	if h.HeroCards != "AsKh" {
		t.Errorf("底牌未规范化: %q", h.HeroCards)
	}
	if h.Board != "Qs7h2d" {
		t.Errorf("公共牌未规范化: %q", h.Board)
	}
}

func TestValidateReviewHand_FillsEmptySlices(t *testing.T) {
	// 空数组必须落成 []，否则 JSON 列写成 null，前端还要多写一层判空
	h := validHand()
	h.Streets = nil
	h.Villains = nil
	h.HeroTags = nil
	h.Board = ""
	if err := ValidateReviewHand(h); err != nil {
		t.Fatalf("应校验通过，实际: %v", err)
	}
	if h.Streets == nil || h.Villains == nil || h.HeroTags == nil {
		t.Error("空切片应被规范化为空数组")
	}
}

func TestComputeHandHash(t *testing.T) {
	h1 := validHand()
	h2 := validHand()
	if ComputeHandHash(h1) != ComputeHandHash(h2) {
		t.Error("内容相同的手牌哈希应该一致")
	}

	// 改标题不该让已有分析失效
	h3 := validHand()
	h3.Title = "换了个名字"
	if ComputeHandHash(h1) != ComputeHandHash(h3) {
		t.Error("标题变化不应影响哈希")
	}

	// 改思路必须换哈希 —— 它直接影响 AI 的分析结论
	h4 := validHand()
	h4.HeroThought = "当时想控池"
	if ComputeHandHash(h1) == ComputeHandHash(h4) {
		t.Error("思路变化必须改变哈希")
	}

	// 改行动序列同理
	h5 := validHand()
	h5.Streets[1].Actions[1].AmountBB = bb(6)
	if ComputeHandHash(h1) == ComputeHandHash(h5) {
		t.Error("下注尺度变化必须改变哈希")
	}

	// 人数会写进提示词，同一个位置在不同人数下含义不同，必须换哈希
	h6 := validHand()
	h6.TableSize = 6
	h6.HeroPosition = models.PositionHJ
	if ComputeHandHash(h1) == ComputeHandHash(h6) {
		t.Error("人数变化必须改变哈希")
	}
}

func TestValidateReviewHand_TableSize(t *testing.T) {
	// 人数留空按满员桌处理，前端默认值也是 9，两边要一致
	h := validHand()
	h.TableSize = 0
	if err := ValidateReviewHand(h); err != nil {
		t.Fatalf("人数留空应校验通过，实际: %v", err)
	}
	if h.TableSize != models.DefaultTableSize {
		t.Errorf("人数留空应归一化为 %d，实际 %d", models.DefaultTableSize, h.TableSize)
	}

	// 6 人桌的合法位置
	for _, pos := range []string{
		models.PositionSB, models.PositionBB, models.PositionUTG,
		models.PositionHJ, models.PositionCO, models.PositionBTN,
	} {
		h := validHand()
		h.TableSize = 6
		h.HeroPosition = pos
		if err := ValidateReviewHand(h); err != nil {
			t.Errorf("6 人桌的 %s 应合法，实际: %v", pos, err)
		}
	}

	// 对手只记数量不记位置是合法状态，不能被位置校验误伤
	h2 := validHand()
	h2.Villains = []models.VillainInfo{{StackBB: bb(100), IsKey: true}}
	if err := ValidateReviewHand(h2); err != nil {
		t.Fatalf("对手位置为空应放行，实际: %v", err)
	}
}

func TestPositionsForTableSize(t *testing.T) {
	// 这张表是前后端共用的契约（前端 src/utils/poker.ts 的 positionsForTableSize），
	// 人数与位置的对应关系写错会直接导致录入页选不出正确位置
	want := map[int][]string{
		9: {models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionUTG1,
			models.PositionUTG2, models.PositionLJ, models.PositionHJ, models.PositionCO,
			models.PositionBTN},
		8: {models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionUTG1,
			models.PositionLJ, models.PositionHJ, models.PositionCO, models.PositionBTN},
		7: {models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionLJ,
			models.PositionHJ, models.PositionCO, models.PositionBTN},
		6: {models.PositionSB, models.PositionBB, models.PositionUTG,
			models.PositionHJ, models.PositionCO, models.PositionBTN},
		5: {models.PositionSB, models.PositionBB, models.PositionUTG,
			models.PositionCO, models.PositionBTN},
		4: {models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionBTN},
		3: {models.PositionSB, models.PositionBB, models.PositionBTN},
		2: {models.PositionSB, models.PositionBB},
	}

	for size := models.MinTableSize; size <= models.MaxTableSize; size++ {
		got := PositionsForTableSize(size)
		expected, ok := want[size]
		if !ok {
			t.Fatalf("%d 人桌缺少期望值", size)
		}
		if len(got) != size {
			t.Errorf("%d 人桌应有 %d 个位置，实际 %d 个：%v", size, size, len(got), got)
		}
		if len(got) != len(expected) {
			t.Errorf("%d 人桌位置数量应为 %d，实际 %d", size, len(expected), len(got))
			continue
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("%d 人桌第 %d 个位置应为 %s，实际 %s（顺序也要一致）", size, i, expected[i], got[i])
			}
		}
	}

	// 越界人数没有合法位置，调用方据此报错
	if PositionsForTableSize(1) != nil || PositionsForTableSize(10) != nil {
		t.Error("越界人数应返回 nil")
	}
}

func TestStreetMinBoardCards(t *testing.T) {
	// 公共牌张数校验依赖这张表，写错会让"打了河牌但只有 3 张公共牌"漏过去
	want := map[string]int{
		models.StreetPreflop: 0,
		models.StreetFlop:    3,
		models.StreetTurn:    4,
		models.StreetRiver:   5,
	}
	for street, n := range want {
		if streetMinBoardCards[street] != n {
			t.Errorf("%s 最少公共牌张数应为 %d，实际 %d", street, n, streetMinBoardCards[street])
		}
	}
}

func TestCheckGameAccessibleIgnoresNil(t *testing.T) {
	// 不关联场次的手牌必须能保存（独立复盘场景）
	h := validHand()
	h.GameID = nil
	if err := ValidateReviewHand(h); err != nil {
		t.Fatalf("不关联场次时应校验通过，实际: %v", err)
	}
	_ = ptrUint(1)
}
