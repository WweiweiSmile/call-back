package services

import (
	"call-go/models"
	"strings"
	"testing"
)

func handWithTableSize(size int) *models.ReviewHand {
	return &models.ReviewHand{
		TableSize:    size,
		HeroPosition: models.PositionBTN,
		HeroCards:    "AsKh",
		HeroStackBB:  100,
		VillainCount: 1,
		Streets: []models.StreetRecord{
			{Street: models.StreetPreflop, Actions: []models.StreetAction{
				{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bbPtr(2.5)},
			}},
		},
	}
}

func bbPtr(v float64) *float64 { return &v }

// 人数必须真的写进提示词：位置的含义完全取决于它，模型看不到人数就会默认按
// 满员桌理解 UTG/LJ，6-max 的牌会被按 9 人桌的标准点评
func TestBuildHandBlockIncludesTableSize(t *testing.T) {
	cases := []struct {
		size     int
		wantText string
	}{
		{9, "9人桌"},
		{8, "8人桌"},
		{6, "6人桌"},
		{3, "3人桌"},
	}
	for _, tc := range cases {
		block := BuildHandBlock(handWithTableSize(tc.size))
		if !strings.Contains(block, tc.wantText) {
			t.Errorf("%d 人桌的手牌块应包含 %q，实际:\n%s", tc.size, tc.wantText, block)
		}
		// 桌型要排在 Hero 行之前：先让模型建立桌型，再读位置
		if strings.Index(block, tc.wantText) > strings.Index(block, "Hero (") {
			t.Errorf("%d 人桌的桌型应排在 Hero 行之前，实际:\n%s", tc.size, block)
		}
	}
}

// 2 人桌的 SB 同时是 BTN。不点明的话模型会把单挑当成有常规盲注位的对抗，
// 而单挑的按钮位是翻前先行动、翻后后行动，正好反过来
func TestBuildHandBlockHeadsUp(t *testing.T) {
	block := BuildHandBlock(handWithTableSize(2))
	if !strings.Contains(block, "2人桌") {
		t.Errorf("2 人桌的手牌块应包含桌型，实际:\n%s", block)
	}
	if !strings.Contains(block, "SB 同时是 BTN") {
		t.Errorf("2 人桌应点明 SB 兼任 BTN，实际:\n%s", block)
	}
}

// 人数为 0 时兜底成满员桌而不是输出「0人桌」
func TestBuildHandBlockTableSizeZeroFallsBack(t *testing.T) {
	block := BuildHandBlock(handWithTableSize(0))
	if strings.Contains(block, "0人桌") {
		t.Errorf("人数为 0 时不应输出 0人桌，实际:\n%s", block)
	}
	if !strings.Contains(block, "9人桌") {
		t.Errorf("人数为 0 时应兜底成 9 人桌，实际:\n%s", block)
	}
}

// M7.1：对手要有名字。同一个人在各条街的行动必须写成同一个称呼，
// 否则模型会把"翻前 CO 加注"和"河牌 CO 又下注"当成两个人
func TestBuildHandBlockNamesVillains(t *testing.T) {
	hand := handWithTableSize(9)
	hand.Villains = []models.VillainInfo{
		{Position: models.PositionCO, Name: "老王", IsKey: true},
		{Position: models.PositionSB, Name: "小李"},
	}
	hand.Streets = []models.StreetRecord{
		{Street: models.StreetPreflop, Actions: []models.StreetAction{
			{Actor: models.PositionCO, Action: models.ActionRaise, AmountBB: bbPtr(2.5)},
			{Actor: models.PositionSB, Action: models.ActionFold},
			{Actor: models.ActorHero, Action: models.ActionCall},
		}},
	}

	block := BuildHandBlock(hand)
	if !strings.Contains(block, "老王 (CO)  [关键对手]") {
		t.Errorf("对手块应带名字与位置，实际:\n%s", block)
	}
	if !strings.Contains(block, "小李 (SB)") {
		t.Errorf("非关键对手也要列出名字，实际:\n%s", block)
	}
	if !strings.Contains(block, "老王 (CO) raises to 2.5bb") {
		t.Errorf("行动序列里的对手应写成「名字 (位置)」，实际:\n%s", block)
	}
	if !strings.Contains(block, "小李 (SB) folds") {
		t.Errorf("弃牌也要指名道姓，实际:\n%s", block)
	}
}

// 老手牌（对手没有名字）的输出必须与 M7.1 之前逐字一致：
// 老结论是在那套写法下得出的，换了写法就没有可比性
func TestBuildHandBlockLegacyVillainOutputUnchanged(t *testing.T) {
	hand := handWithTableSize(9)
	hand.Villains = []models.VillainInfo{{Position: models.PositionBB, StackBB: bbPtr(100), IsKey: true}}
	hand.Streets = []models.StreetRecord{
		{Street: models.StreetPreflop, Actions: []models.StreetAction{
			{Actor: models.ActorVillain, Action: models.ActionCall},
		}},
	}

	block := BuildHandBlock(hand)
	if !strings.Contains(block, "Villain (BB) 100bb  [关键对手]") {
		t.Errorf("老数据的对手行应与 M7.1 之前一致，实际:\n%s", block)
	}
	if !strings.Contains(block, "Villain calls") {
		t.Errorf("老数据的行动仍应写成聚合角色 Villain，实际:\n%s", block)
	}
}

// 系统提示词要告诉模型位置随人数变化，否则它拿到「6人桌 + UTG」还是会按满员桌判
func TestSystemPromptMentionsTableSize(t *testing.T) {
	prompt := BuildSystemPrompt(nil)
	if !strings.Contains(prompt, "位置的含义取决于桌型") {
		t.Errorf("系统提示词应说明位置含义随桌型变化，实际:\n%s", prompt)
	}
}
