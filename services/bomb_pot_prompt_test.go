package services

import (
	"strings"
	"testing"

	"call-go/models"
)

// bombPotHand 6 人桌爆炸底池：没有翻前行动，每人先投 5bb 直接看翻牌。
// 翻牌上对手过牌、我下注 4bb（跟注不记金额，跟到当前下注额即可）
func bombPotHand() *models.ReviewHand {
	h := handWithTableSize(6)
	h.BombPotBB = 5
	h.Board = "Qs7h2d"
	h.Villains = []models.VillainInfo{{Position: models.PositionBB, IsKey: true}}
	h.Streets = []models.StreetRecord{
		{Street: models.StreetFlop, Actions: []models.StreetAction{
			{Actor: models.PositionBB, Action: models.ActionCheck},
			{Actor: models.ActorHero, Action: models.ActionBet, AmountBB: bbPtr(4)},
		}},
	}
	return h
}

// 爆炸底池必须自带一行说明。
//
// 它的三个盲注列是 0（确实没发盲注），少了这行，模型只会看到"翻前一片空白"
// 然后按常规牌局的框架批评"翻前为什么没有任何动作" —— 那是拿不存在的局面挑错
func TestBuildHandBlockExplainsBombPot(t *testing.T) {
	block := BuildHandBlock(bombPotHand())

	if !strings.Contains(block, "爆炸底池") {
		t.Errorf("提示词里应写明这手是爆炸底池，实际:\n%s", block)
	}
	if !strings.Contains(block, "没有翻前行动") {
		t.Errorf("必须点明没有翻前行动，否则模型会以为用户漏记了翻前，实际:\n%s", block)
	}
	if !strings.Contains(block, "每人先投 5bb") {
		t.Errorf("必须写清每人先投多少，实际:\n%s", block)
	}
	// 5 × 6 = 30bb：起始底池要写出来，赔率推理的分母全靠它
	if !strings.Contains(block, "= 30bb") {
		t.Errorf("爆炸底池的起始底池应写明 30bb，实际:\n%s", block)
	}
}

// 爆炸底池下不能出现盲注那一行，也不能凭空多出一条翻前街
func TestBuildHandBlockBombPotHasNoBlindLineAndNoPreflop(t *testing.T) {
	block := BuildHandBlock(bombPotHand())

	if strings.Contains(block, "盲注: 小盲") {
		t.Errorf("爆炸底池没发盲注，不该有盲注那一行，实际:\n%s", block)
	}
	if strings.Contains(block, "Preflop") {
		t.Errorf("没有翻前行动就不该渲染翻前街，实际:\n%s", block)
	}
}

// 起始底池要出现在翻牌那一行上：30bb 死钱 + 我下注的 4bb
func TestBuildHandBlockBombPotPotProgression(t *testing.T) {
	block := BuildHandBlock(bombPotHand())

	if !strings.Contains(block, "(pot 30.0bb)") {
		t.Errorf("翻牌起始底池应为爆炸底池的 30bb，实际:\n%s", block)
	}
	// 30 + 4（我的下注，对手过牌不投入）
	if !strings.Contains(block, "最终底池约 34.0bb（估算，已含爆炸底池每人先投的死钱）") {
		t.Errorf("最终底池的口径要如实标注是爆炸底池的死钱，实际:\n%s", block)
	}
}

// 提示词版本按**手牌**报：爆炸底池是 v3.1-skills，常规手牌仍是 v3.0-skills。
//
// 写成全局常量一并升版的话，库里所有历史分析都会因为"版本不一致"被判 stale，
// 前端随即跳出「这手牌在分析之后被修改过」—— 对那些压根没改过的手牌是假警报，
// 而压掉警报的唯一办法是白花一次模型额度重跑一遍逐字相同的提示词
func TestPromptVersionForBombPot(t *testing.T) {
	if models.CurrentPromptVersion == models.PromptVersionBombPot {
		t.Fatal("两个版本号相同，这条测试等于没测")
	}

	plain := handWithTableSize(9)
	if got := models.PromptVersionFor(plain); got != models.CurrentPromptVersion {
		t.Errorf("常规手牌应报 %s，实际 %s", models.CurrentPromptVersion, got)
	}

	bomb := handWithTableSize(9)
	bomb.BombPotBB = 5
	if got := models.PromptVersionFor(bomb); got != models.PromptVersionBombPot {
		t.Errorf("爆炸底池手牌应报 %s，实际 %s", models.PromptVersionBombPot, got)
	}

	// nil 安全：调用方在加载失败等路径上可能还没有手牌
	if got := models.PromptVersionFor(nil); got != models.CurrentPromptVersion {
		t.Errorf("nil 手牌应报常规版本，实际 %s", got)
	}
}

// 回归：常规手牌的提示词里不该出现"爆炸底池"字样，输出与加这个玩法之前逐字一致
func TestBuildHandBlockWithoutBombPotKeepsOldWording(t *testing.T) {
	block := BuildHandBlock(handWithBlinds(0.5, 1, 0))

	if strings.Contains(block, "爆炸底池") {
		t.Errorf("常规手牌不该出现爆炸底池的说明，实际:\n%s", block)
	}
	// 盲注那一行照旧
	if !strings.Contains(block, "盲注: 小盲 0.5bb") {
		t.Errorf("常规手牌的盲注行不该被这次改动影响，实际:\n%s", block)
	}
}
