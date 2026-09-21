package services

import (
	"call-go/config"
	"call-go/models"
	"call-go/utils"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 真机验证：设计文档 §9.1 列的三条验证点，改技能库之后都该重跑一遍。
//
// 默认跳过 —— 它会用某个用户的真实 Key 调真实模型，每次两三分钟。
// 跑法：
//
//	VERIFY_REAL_MODEL=1 VERIFY_USER_ID=11 go test ./services/ -run TestVerify -v -timeout 30m
//
// 不设 VERIFY_USER_ID 时用下面这个默认值（本地 dev 库里配了 Key 的用户）

func verifySetup(t *testing.T) (*AICallSettings, *AIClient, []models.ReviewLeakTag) {
	t.Helper()
	userID := uint(11)
	if v := os.Getenv("VERIFY_USER_ID"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("VERIFY_USER_ID 不是数字：%v", err)
		}
		userID = uint(n)
	}

	if err := config.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	utils.SetJWTSecret(config.AppConfig.JWTSecret)
	utils.SetPreferenceEncryptionKey(config.AppConfig.PrefEncryptionKey)
	if err := config.InitDB(); err != nil {
		t.Fatal(err)
	}

	// 测试的 cwd 是包目录，找不到仓库根的 .env —— 要先 source .env 再跑
	settings, err := (&AISettingService{}).ResolveCallSettings(userID)
	if err != nil {
		t.Fatalf("解析用户 %d 的 AI 设置失败（.env 里的 DB_* 带上了吗？）：%v", userID, err)
	}
	tags, err := NewReviewService().GetLeakTagModels()
	if err != nil {
		t.Fatal(err)
	}
	return settings, NewAIClient(), tags
}

// airOnRiver 河牌持空气的手牌。
//
// 这是「渐进性披露会不会让模型推卸责任」最真实的场景：
// hero 在河牌做了一个决策，但 river-value / river-medium / postflop-made-hand /
// postflop-draw 全都**不会展开**（空气既不是成牌也没有听牌），
// 模型只能靠常驻技能 + 转牌技能去点评那条街。
func airOnRiver() *models.ReviewHand {
	stack := 100.0
	h := &models.ReviewHand{
		HeroPosition: "BB",
		HeroCards:    "Ad3c",
		Board:        "Kd8h2s5c9h",
		HeroStackBB:  100,
		TableSize:    6,
		PotType:      "hu",
		Stakes:       "1/2",
		Villains:     []models.VillainInfo{{Name: "老王", Position: "BTN", StackBB: &stack}},
		HeroThought:  "他在河牌下这么大，我觉得他是在诈唬，但我只有 A 高",
		Streets: []models.StreetRecord{
			{Street: models.StreetPreflop, Actions: []models.StreetAction{
				{Actor: "BTN", Action: models.ActionRaise, AmountBB: ptr(3)},
				{Actor: "BB", Action: models.ActionCall},
			}},
			{Street: models.StreetFlop, Actions: []models.StreetAction{
				{Actor: "BB", Action: models.ActionCheck},
				{Actor: "BTN", Action: models.ActionBet, AmountBB: ptr(4)},
				{Actor: "BB", Action: models.ActionCall},
			}},
			{Street: models.StreetTurn, Actions: []models.StreetAction{
				{Actor: "BB", Action: models.ActionCheck},
				{Actor: "BTN", Action: models.ActionCheck},
			}},
			{Street: models.StreetRiver, Actions: []models.StreetAction{
				{Actor: "BB", Action: models.ActionCheck},
				{Actor: "BTN", Action: models.ActionBet, AmountBB: ptr(18)},
				{Actor: "BB", Action: models.ActionFold},
			}},
		},
	}
	return h
}

func ptr(v float64) *float64 { return &v }

// oldStyleSystemPrompt 重构出「改造前」的系统提示词：
// 技能层换成全部技能正文、去掉目录层、去掉新增的硬性约束 13-16。
//
// 目的不是复刻历史，而是**同一手牌、同一份代码，只让技能层不同**，
// 这样 tokens_in 的差就纯粹来自渐进性披露本身
func oldStyleSystemPrompt(t *testing.T, hand *models.ReviewHand, tags []models.ReviewLeakTag) string {
	t.Helper()
	sys := BuildSystemPrompt(hand, tags)

	all := AllSkills()
	sel := SelectSkills(all, hand)
	block := RenderSkillCatalog(all, sel) + "\n" + RenderSkillBlock(sel)
	if !strings.Contains(sys, block) {
		t.Fatal("拼不出技能层，无法重构旧提示词")
	}
	sys = strings.Replace(sys, block, RenderSkillBlock(all), 1)

	// 去掉新增的约束 13-16
	i := strings.Index(sys, "\n13. 系统提示里的「教练技能」")
	j := strings.Index(sys, "\n## 分析视角")
	if i < 0 || j < 0 || j < i {
		t.Fatal("定位不到新增约束，无法重构旧提示词")
	}
	return sys[:i] + sys[j:]
}

func runOnce(t *testing.T, client *AIClient, settings *AICallSettings, label,
	system, user string) *CompletionResult {
	t.Helper()
	ctx := context.Background()
	start := time.Now()
	res, err := client.CompleteJSON(ctx, *settings, system, user)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("%s 调用失败：%v", label, err)
	}
	t.Logf("%s：耗时 %s，tokens %d/%d，system %d 字符，user %d 字符",
		label, dur.Round(time.Second), res.TokensIn, res.TokensOut, len([]rune(system)), len([]rune(user)))
	return res
}

// TestZZVerifyShirkOrNot 验证点 1（成败判据）：模型会不会在专项技能未展开时推卸责任
func TestVerifyShirkOrNot(t *testing.T) {
	if os.Getenv("VERIFY_REAL_MODEL") != "1" {
		t.Skip("设置 VERIFY_REAL_MODEL=1 才跑")
	}
	settings, client, tags := verifySetup(t)

	hand := airOnRiver()
	sel := SelectSkills(AllSkills(), hand)
	t.Logf("本次展开 %d 篇：%v", len(sel), codesOf(sel))

	system, user := BuildAnalysisPrompt(hand, tags, nil)
	res := runOnce(t, client, settings, "河牌空气·新提示词", system, user)

	os.WriteFile("/tmp/verify_shirk.txt", []byte(res.Content), 0o644)
	t.Logf("原始返回已写入 /tmp/verify_shirk.txt")
}

// TestZZVerifyTokens 验证点 3：同一手牌，新旧提示词的 tokens_in 直接对比
func TestVerifyTokens(t *testing.T) {
	if os.Getenv("VERIFY_REAL_MODEL") != "1" {
		t.Skip("设置 VERIFY_REAL_MODEL=1 才跑")
	}
	settings, client, tags := verifySetup(t)

	stack := 100.0
	bigHand := &models.ReviewHand{
		HeroPosition: "BTN",
		HeroCards:    "9d9h",
		Board:        "9s8s2d7c3h",
		HeroStackBB:  100,
		TableSize:    6,
		PotType:      "multi",
		Stakes:       "1/2",
		Villains: []models.VillainInfo{
			{Name: "老王", Position: "CO", StackBB: &stack},
			{Position: "BB", StackBB: &stack},
		},
		HeroThought: "翻牌中了三条，一路价值下注",
		Streets: []models.StreetRecord{
			{Street: models.StreetPreflop, Actions: []models.StreetAction{
				{Actor: "CO", Action: models.ActionCall},
				{Actor: "BTN", Action: models.ActionRaise, AmountBB: ptr(4)},
				{Actor: "BB", Action: models.ActionCall},
				{Actor: "CO", Action: models.ActionCall},
			}},
			{Street: models.StreetFlop, Actions: []models.StreetAction{
				{Actor: "BB", Action: models.ActionCheck},
				{Actor: "CO", Action: models.ActionCheck},
				{Actor: "BTN", Action: models.ActionBet, AmountBB: ptr(9)},
				{Actor: "BB", Action: models.ActionCall},
				{Actor: "CO", Action: models.ActionFold},
			}},
			{Street: models.StreetTurn, Actions: []models.StreetAction{
				{Actor: "BB", Action: models.ActionCheck},
				{Actor: "BTN", Action: models.ActionBet, AmountBB: ptr(22)},
				{Actor: "BB", Action: models.ActionCall},
			}},
			{Street: models.StreetRiver, Actions: []models.StreetAction{
				{Actor: "BB", Action: models.ActionCheck},
				{Actor: "BTN", Action: models.ActionBet, AmountBB: ptr(50)},
				{Actor: "BB", Action: models.ActionCall},
			}},
		},
	}

	sel := SelectSkills(AllSkills(), bigHand)
	t.Logf("本次展开 %d 篇：%v", len(sel), codesOf(sel))

	newSys, user := BuildAnalysisPrompt(bigHand, tags, nil)
	oldSys := oldStyleSystemPrompt(t, bigHand, tags)

	t.Logf("新 system %d 字符 / 旧 system %d 字符",
		len([]rune(newSys)), len([]rune(oldSys)))

	rNew := runOnce(t, client, settings, "新提示词", newSys, user)
	rOld := runOnce(t, client, settings, "旧提示词（全量）", oldSys, user)

	delta := rNew.TokensIn - rOld.TokensIn
	t.Logf("tokens_in：新 %d，旧 %d，差 %+d（%.1f%%）",
		rNew.TokensIn, rOld.TokensIn, delta, float64(delta)*100/float64(rOld.TokensIn))

	os.WriteFile("/tmp/verify_new.txt", []byte(rNew.Content), 0o644)
	os.WriteFile("/tmp/verify_old.txt", []byte(rOld.Content), 0o644)
	os.WriteFile("/tmp/verify_old_system.txt", []byte(oldSys), 0o644)
	t.Logf("原始返回已写入 /tmp/verify_new.txt 与 /tmp/verify_old.txt")
}
