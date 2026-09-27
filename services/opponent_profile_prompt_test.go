package services

import (
	"call-go/models"
	"strings"
	"testing"
)

// profileFixture 造一组"与老王交手"的手牌 + 对应统计
func profileFixture() ([]models.ReviewHand, *models.OpponentStats) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		func() models.ReviewHand {
			h := statHand(1, co, "AhAd",
				models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
					statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))},
				models.StreetRecord{Street: models.StreetFlop, Actions: statActions(
					statAct(models.ActorHero, models.ActionCheck), statBet(co, 5),
					statAct(models.ActorHero, models.ActionCall))},
			)
			h.Board = "Qs7h2d"
			return h
		}(),
		statHand(2, co, "",
			models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
				statAct(models.ActorHero, models.ActionRaise), statAct(co, models.ActionCall))},
		),
	}
	return hands, ComputeOpponentStats("老王", hands, testOpponentID)
}

// 关键的一对镜像断言之一：对手底牌**必须**进画像提示词。
//
// 与 TestPromptsNeverLeakVillainCards 正好相反 —— 那一头证明它进不了手牌分析，
// 这一头证明它没有在某个环节被整个丢掉。只测一头的话，
// "干脆谁都不给"和"谁都给"这两种错误各能骗过一半的测试
func TestOpponentProfilePromptIncludesVillainCards(t *testing.T) {
	hands, stats := profileFixture()
	_, user := BuildOpponentProfilePrompt(stats, hands, nil)

	// 牌面用符号写法渲染，与逐手记录里的手牌块一致
	if !strings.Contains(user, "A♥A♦") {
		t.Errorf("画像提示词应包含对手底牌 A♥A♦，实际:\n%s", user)
	}
	// 成牌等级由 Go 算好给模型，不让它自己看牌面
	if !strings.Contains(user, "超对") {
		t.Errorf("摊牌样本应带上算好的成牌等级（AA 在 Qs7h2d 上是超对）:\n%s", user)
	}
}

// 统计数字必须带 n/N。模型看到"入池 5 次"时不知道分母是 5 还是 50，
// 给出的频率判断就没有意义
func TestOpponentProfilePromptCarriesSampleSizes(t *testing.T) {
	hands, stats := profileFixture()
	_, user := BuildOpponentProfilePrompt(stats, hands, nil)

	for _, want := range []string{"行为统计", "翻前（按位置分层）", "面对加注弃牌", "摊牌样本", "下注尺度"} {
		if !strings.Contains(user, want) {
			t.Errorf("画像提示词应包含 %q，实际:\n%s", want, user)
		}
	}
	// 分母必须写出来，而不是只给一个计数
	if !strings.Contains(user, "/2") {
		t.Errorf("统计行应带 n/N 形式的样本量，实际:\n%s", user)
	}
}

// 样本不足要显式警告，并且把门槛数字写进去 ——
// 光说"样本不足"而不说少于几手算不足，模型只能自己猜一条线
func TestOpponentProfilePromptWarnsOnThinSample(t *testing.T) {
	co := models.PositionCO
	hands := []models.ReviewHand{
		statHand(1, co, "", models.StreetRecord{Street: models.StreetPreflop, Actions: statActions(
			statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))}),
	}
	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	if !stats.ThinSample {
		t.Fatal("1 手应判为样本不足")
	}

	_, user := BuildOpponentProfilePrompt(stats, hands, nil)
	if !strings.Contains(user, "样本不足") {
		t.Errorf("样本不足时应给出警告，实际:\n%s", user)
	}

	// 反过来：样本够了就不该再出现这句警告，否则模型会无谓地保守
	enough := make([]models.ReviewHand, 0, 6)
	for i := 1; i <= 6; i++ {
		enough = append(enough, statHand(uint(i), co, "", models.StreetRecord{
			Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))}))
	}
	_, userEnough := BuildOpponentProfilePrompt(
		ComputeOpponentStats("老王", enough, testOpponentID), enough, nil)
	if strings.Contains(userEnough, "样本不足") {
		t.Error("样本充足时不该再警告样本不足")
	}
}

// 逐手记录被截断时必须说明"统计覆盖全部、这里只有最近几手"。
// 不说的话模型会拿最近几手去核对统计里的分母，然后得出"统计错了"的结论
func TestOpponentProfilePromptDisclosesTruncation(t *testing.T) {
	co := models.PositionCO
	hands := make([]models.ReviewHand, 0, OpponentProfileMaxHandsInPrompt+5)
	for i := 1; i <= OpponentProfileMaxHandsInPrompt+5; i++ {
		hands = append(hands, statHand(uint(i), co, "", models.StreetRecord{
			Street: models.StreetPreflop, Actions: statActions(
				statAct(co, models.ActionRaise), statAct(models.ActorHero, models.ActionCall))}))
	}
	stats := ComputeOpponentStats("老王", hands, testOpponentID)
	_, user := BuildOpponentProfilePrompt(stats, hands, nil)

	if !strings.Contains(user, "只列出最近") {
		t.Errorf("截断逐手记录时应说明口径差，实际:\n%s", user)
	}
}

// 方法论必须注入，且与手牌分析同源 —— 否则会出现两条路径给出相反形象判断的情况
func TestOpponentProfilePromptInjectsMethodology(t *testing.T) {
	hands, stats := profileFixture()
	system, _ := BuildOpponentProfilePrompt(stats, hands, nil)

	// 五格分类表来自 skills/opponent-read.md
	for _, want := range []string{"对手读牌", "跟注站", "岩石", "松凶"} {
		if !strings.Contains(system, want) {
			t.Errorf("系统提示应注入对手读牌技能（缺少 %q）", want)
		}
	}
	// 身份映射必须钉死，否则模型会滑回"评价学员打得对不对"
	for _, want := range []string{"身份映射", "摊牌样本", "不许用结果倒推", "按位置分层"} {
		if !strings.Contains(system, want) {
			t.Errorf("系统提示应包含约束 %q", want)
		}
	}
}

// ---------- 解析与强制降级 ----------

// 样本不足时**在代码里**强制降级。
//
// 提示词里写了这条，但它是本功能里最容易被绕过、后果又最严重的一条：
// 2 手牌得出的"他是松凶"会被用户当成事实去执行
func TestParseOpponentProfileForcesDowngradeOnThinSample(t *testing.T) {
	content := `{
      "profile": "looseAggressive",
      "confidence": "high",
      "profileReason": "他两把都加注了",
      "tendencies": [{"aspect":"翻前开池","observation":"很松","sampleSize":"2/2","evidence":"翻前加注"}],
      "exploits": [{"against":"翻前开池","adjustment":"收紧跟注范围","sizing":"3bet 到 9bb","risk":"他 4bet"}],
      "unknowns": [], "watchNext": ["多记几手"], "summary": "他是个疯子"
    }`

	thin := &models.OpponentStats{Hands: 2, ThinSample: true}
	result, err := parseOpponentProfileResult(content, thin)
	if err != nil {
		t.Fatalf("解析不该失败: %v", err)
	}

	if result.Profile != models.ProfileUnknown {
		t.Errorf("样本不足时形象应被强制为 unknown，实际 %q", result.Profile)
	}
	if result.Confidence != models.ConfidenceLow {
		t.Errorf("样本不足时置信度应被强制为 low，实际 %q", result.Confidence)
	}
	if len(result.Exploits) != 0 {
		t.Errorf("样本不足时不该给出剥削方案，实际 %d 条", len(result.Exploits))
	}
	// 降级后四个数组必须都是非 nil 的空数组：nil 落库会变成 JSON 的 null，
	// 前端拿 .length 直接用会白屏（第一次端到端跑就是这么炸的）
	if result.Exploits == nil {
		t.Error("exploits 降级后必须是空数组而不是 nil")
	}
	if result.Tendencies == nil || result.Unknowns == nil || result.WatchNext == nil {
		t.Error("四个数组都必须是空数组而不是 nil")
	}
	// 总结要保留：它是唯一还能给用户看的东西（"该继续观察什么"）
	if result.Summary != "他是个疯子" {
		t.Errorf("总结不该被清掉，实际 %q", result.Summary)
	}

	// 反向确认：样本充足时同样的内容不该被降级 ——
	// 没有这一条的话，一个"永远返回 unknown"的实现也能通过上面全部断言
	enough := &models.OpponentStats{Hands: 12, ThinSample: false}
	kept, err := parseOpponentProfileResult(content, enough)
	if err != nil {
		t.Fatalf("解析不该失败: %v", err)
	}
	if kept.Profile != models.ProfileLooseAggressive || len(kept.Exploits) != 1 {
		t.Errorf("样本充足时不该降级，实际 %+v", kept)
	}
}

// 没见过的形象枚举要归到 unknown，而不是原样存下来 ——
// 前端按枚举渲染，一个没见过的值会让整块画像显示不出来
func TestParseOpponentProfileSanitizesEnums(t *testing.T) {
	content := `{"profile":"maniac","confidence":"very-high","summary":"他很凶"}`

	result, err := parseOpponentProfileResult(content, &models.OpponentStats{Hands: 20})
	if err != nil {
		t.Fatalf("解析不该失败: %v", err)
	}
	if result.Profile != models.ProfileUnknown {
		t.Errorf("未知形象应归为 unknown，实际 %q", result.Profile)
	}
	if result.Confidence != models.ConfidenceLow {
		t.Errorf("未知置信度应归为 low，实际 %q", result.Confidence)
	}
	// 模型完全可以整个省略这几个数组。省略 → json 解出 nil → 必须被归一成空数组
	if result.Tendencies == nil || result.Exploits == nil ||
		result.Unknowns == nil || result.WatchNext == nil {
		t.Errorf("省略的数组应被归一成空数组，实际 %+v", result)
	}
}

// 模型偶尔会把 JSON 包在 ``` 里，得能剥掉
func TestParseOpponentProfileStripsCodeFence(t *testing.T) {
	content := "```json\n{\"profile\":\"tightPassive\",\"confidence\":\"medium\",\"summary\":\"他很紧\"}\n```"

	result, err := parseOpponentProfileResult(content, &models.OpponentStats{Hands: 20})
	if err != nil {
		t.Fatalf("解析不该失败: %v", err)
	}
	if result.Profile != models.ProfileTightPassive {
		t.Errorf("形象应为 tightPassive，实际 %q", result.Profile)
	}
}

// 没有总结就算失败：画像是给人看的，只有几个枚举字段等于什么都没说
func TestParseOpponentProfileRejectsEmptySummary(t *testing.T) {
	for _, content := range []string{
		`{"profile":"unknown","confidence":"low","summary":"   "}`,
		`不是 JSON`,
		``,
	} {
		if _, err := parseOpponentProfileResult(content, &models.OpponentStats{Hands: 20}); err == nil {
			t.Errorf("内容 %q 应被拒收", content)
		}
	}
}
