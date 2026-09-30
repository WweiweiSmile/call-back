package services

import (
	"testing"

	"call-go/models"
)

func containsCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}

// 爆炸底池没有翻前行动，翻前三篇技能与「持续下注」都不命中。
//
// 这不是漏了，而是语义正确：没有翻前加注者就谈不上"持续"下注（isCbetSpot 第一步
// isPreflopAggressor 恒为 false），PreflopShapes 也返回空集。
//
// 这条测试守的是"别为了让技能命中而给爆炸底池硬凑一个翻前形态" ——
// 那会让模型拿 3bet 底池的细则去点评一手压根没有翻前的牌
func TestSelectSkillsForBombPotExcludesPreflopAndCbet(t *testing.T) {
	h := handWith([]models.StreetRecord{
		{Street: models.StreetFlop, Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionBet, AmountBB: bbPtr(4)},
		}},
	})
	h.BombPotBB = 5
	h.Villains = []models.VillainInfo{{Position: models.PositionBB, IsKey: true}}

	got := codesOf(SelectSkills(AllSkills(), h))

	// 常驻的两篇必须在：教练的底层立场与赔率速查与玩法无关
	for _, want := range []string{"core-stance", "odds-table"} {
		if !containsCode(got, want) {
			t.Errorf("常驻技能 %s 应命中，实际 %v", want, got)
		}
	}
	for _, unwanted := range []string{
		"preflop-open", "preflop-vs-open", "preflop-3bet-pot", "postflop-cbet",
	} {
		if containsCode(got, unwanted) {
			t.Errorf("爆炸底池不该命中 %s（没有翻前行动、也没有翻前加注者），实际 %v", unwanted, got)
		}
	}
}

// 对照组：同样打到翻牌、同样是我先下注，但翻前我是最后加注者 → cbet 应当命中。
//
// 两条一起看才说明问题：上一条的"不命中"是爆炸底池造成的，而不是触发器坏了。
// 少了这个对照，把 postflop-cbet 的触发器改坏也能让上一条测试通过
func TestSelectSkillsCbetStillFiresWithoutBombPot(t *testing.T) {
	control := handWith([]models.StreetRecord{
		{Street: models.StreetPreflop, Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bbPtr(2.5)},
		}},
		{Street: models.StreetFlop, Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionBet, AmountBB: bbPtr(4)},
		}},
	})
	control.Villains = []models.VillainInfo{{Position: models.PositionBB, IsKey: true}}

	got := codesOf(SelectSkills(AllSkills(), control))
	if !containsCode(got, "postflop-cbet") {
		t.Errorf("翻前是我加注、翻牌我先说话时 cbet 技能应命中，实际 %v", got)
	}
}
