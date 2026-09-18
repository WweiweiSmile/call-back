package services

import (
	"call-go/models"
	"strings"
	"testing"
)

func handWithBlinds(smallBlind, bigBlind, ante float64) *models.ReviewHand {
	h := handWithTableSize(9)
	h.TableSize = 9
	h.SmallBlindBB = smallBlind
	h.BigBlindBB = bigBlind
	h.AnteBB = ante
	// 大盲是关键对手，底池推算要靠这个位置把他认出来
	h.Villains = []models.VillainInfo{{Position: models.PositionBB, IsKey: true}}
	return h
}

// 盲注必须写进提示词，而且要点明是死钱。
// 只说底池数字的话，模型看到 "Preflop (pot 1.5bb)" 会以为翻前有人下注过
func TestBuildHandBlockIncludesBlinds(t *testing.T) {
	block := BuildHandBlock(handWithBlinds(0.5, 1, 0))

	if !strings.Contains(block, "小盲 0.5bb") {
		t.Errorf("提示词里应写明小盲额度，实际:\n%s", block)
	}
	if !strings.Contains(block, "大盲 1bb") {
		t.Errorf("提示词里应写明大盲额度，实际:\n%s", block)
	}
	if !strings.Contains(block, "死钱") {
		t.Errorf("提示词里应点明这部分是翻前死钱，否则模型会当成谁下的注，实际:\n%s", block)
	}

	// hero 在 BTN，不是盲注位；死钱 1.5bb，hero 加注到 2.5bb → 4bb
	if !strings.Contains(block, "Preflop (pot 1.5bb)") {
		t.Errorf("翻前起始底池应含盲注，实际:\n%s", block)
	}
	if !strings.Contains(block, "最终底池约 4.0bb（估算，已含盲注与前注）") {
		t.Errorf("最终底池的口径要如实标注，实际:\n%s", block)
	}
}

// 前注要写清"每人一份"和总人数，模型才能自己核对总额
func TestBuildHandBlockIncludesAnte(t *testing.T) {
	block := BuildHandBlock(handWithBlinds(0.5, 1, 0.2))

	if !strings.Contains(block, "前注 0.2bb") {
		t.Errorf("提示词里应写明前注额度，实际:\n%s", block)
	}
	if !strings.Contains(block, "每人一份") {
		t.Errorf("前注必须说明是每人一份，否则模型会把 0.2bb 当成全桌总额，实际:\n%s", block)
	}
	// 0.2 × 9 = 1.8bb；死钱 1.5 + 1.8 = 3.3bb，hero 加注到 2.5bb → 5.8bb
	if !strings.Contains(block, "1.8bb") {
		t.Errorf("前注总额应折算成 1.8bb，实际:\n%s", block)
	}
	if !strings.Contains(block, "Preflop (pot 3.3bb)") {
		t.Errorf("翻前起始底池应为 3.3bb，实际:\n%s", block)
	}
}

// 没记盲注的手牌（含加这个功能之前落库的老数据）必须走老口径，
// 否则提示词会凭空多出盲注说明，模型会照着一个不存在的盲注结构去分析
func TestBuildHandBlockWithoutBlindsKeepsOldWording(t *testing.T) {
	h := handWithTableSize(9)
	block := BuildHandBlock(h)

	// 只认盲注那一行本身：末尾的口径说明「未记录盲注与前注」里也含"盲注""前注"字样，
	// 用裸的 Contains 会被它误判
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "盲注: ") {
			t.Errorf("未记录盲注时不该出现盲注说明行，实际:\n%s", block)
		}
	}
	if strings.Contains(block, "已含盲注") {
		t.Errorf("未记录盲注时不该声称底池含盲注，实际:\n%s", block)
	}
	if !strings.Contains(block, "未记录盲注与前注") {
		t.Errorf("未记录盲注时仍要如实标注口径，实际:\n%s", block)
	}
	// 老口径：翻前起始底池 0，加注到 2.5bb → 2.5bb
	if !strings.Contains(block, "最终底池约 2.5bb") {
		t.Errorf("未记录盲注时底池应与加功能前一致（2.5bb），实际:\n%s", block)
	}
}

// 认得出大盲是谁时，他跟注只补差额；这条链路要一路通到提示词里的底池数字
func TestBuildHandBlockCreditsBigBlindCall(t *testing.T) {
	h := handWithBlinds(0.5, 1, 0)
	h.Streets = []models.StreetRecord{{
		Street: models.StreetPreflop,
		Actions: []models.StreetAction{
			{Actor: models.ActorHero, Action: models.ActionRaise, AmountBB: bbPtr(3)},
			{Actor: models.ActorVillain, Action: models.ActionCall},
		},
	}}

	block := BuildHandBlock(h)

	// 真实底池 0.5 + 1 + 3 + 2 = 6.5
	if !strings.Contains(block, "最终底池约 6.5bb") {
		t.Errorf("大盲跟注后底池应为 6.5bb（只补 2bb），实际:\n%s", block)
	}
}
