package services

import (
	"call-go/models"
	"call-go/utils"
	"strings"
	"testing"
)

// newTestSkill 造一篇用于匹配测试的合成技能。
// body 不可导出，测试同包才能这样直接构造
func newTestSkill(code string, priority int, t SkillTrigger) Skill {
	return Skill{
		Code:     code,
		Title:    code,
		When:     "测试用",
		Summary:  "测试用",
		Priority: priority,
		Trigger:  t,
		body:     "正文-" + code,
	}
}

func act(actor, action string) models.StreetAction {
	return models.StreetAction{Actor: actor, Action: action}
}

func handWith(streets []models.StreetRecord) *models.ReviewHand {
	return &models.ReviewHand{
		HeroPosition: "BTN",
		PotType:      "hu",
		TableSize:    6,
		Streets:      streets,
	}
}

func street(name string, actors ...string) models.StreetRecord {
	rec := models.StreetRecord{Street: name}
	for _, a := range actors {
		rec.Actions = append(rec.Actions, act(a, models.ActionCall))
	}
	return rec
}

// preflopHand 造一手只有翻前行动的手牌
func preflopHand(heroPos string, actions ...models.StreetAction) *models.ReviewHand {
	h := handWith([]models.StreetRecord{{Street: models.StreetPreflop, Actions: actions}})
	h.HeroPosition = heroPos
	return h
}

func codesOf(skills []Skill) []string {
	out := make([]string, 0, len(skills))
	for _, s := range skills {
		out = append(out, s.Code)
	}
	return out
}

// ---------- 加载 ----------

func TestAllSkillsLoads(t *testing.T) {
	all := AllSkills()
	if len(all) != len(skillDefs) {
		t.Fatalf("加载到 %d 篇，注册表有 %d 篇", len(all), len(skillDefs))
	}

	codes := map[string]bool{}
	for _, s := range all {
		if codes[s.Code] {
			t.Errorf("技能 code 重复：%s", s.Code)
		}
		codes[s.Code] = true

		if strings.TrimSpace(s.body) == "" {
			t.Errorf("技能 %s 的正文是空的", s.Code)
		}
		// 目录层要用 When 与 Summary，缺了会在提示词里出现空字段
		if s.Title == "" || s.When == "" || s.Summary == "" {
			t.Errorf("技能 %s 缺少标题 / 什么时候用 / 一句话结论", s.Code)
		}
		tr := s.Trigger
		if !tr.Always && len(tr.Streets) == 0 && len(tr.Positions) == 0 && len(tr.PotTypes) == 0 &&
			len(tr.TableSizes) == 0 && len(tr.PreflopShapes) == 0 && tr.MinVillains == 0 &&
			!tr.CbetSpot && tr.MinMade == utils.MadeNone && !tr.RequireStrength &&
			!tr.RequireDraw && len(tr.Keywords) == 0 {
			// 全零触发器的技能永远不会命中，等于死代码
			t.Errorf("技能 %s 的触发器全为空且非常驻，永远不会被选中", s.Code)
		}
	}

	// 常驻技能必须存在：它们是「匹配全部落空」时的保底
	if !codes["core-stance"] || !codes["odds-table"] {
		t.Error("core-stance 与 odds-table 必须都在，否则匹配落空时没有任何依据")
	}
}

// 注册表写了不存在的文件时必须 panic：静默降级成空技能库会让所有分析
// 悄悄失去依据，且不报任何错
func TestLoadSkillsPanicsOnMissingFile(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("读取不到正文时应当 panic")
		}
	}()
	loadSkills([]skillDef{{Code: "ghost", Title: "幽灵", file: "skills/__not_here__.md"}})
}

// 拆分过程最容易出的错是某条规则在搬运时掉了。这里对每篇技能抽查一条
// 只可能来自该篇的关键规则，确保正文不是空的、也没有被掉包
func TestSkillContentIsPreserved(t *testing.T) {
	must := map[string]string{
		"core-stance":        "建议过牌必须命中以下之一",
		"odds-table":         "四二法则",
		"preflop-open":       "2.5-3.0 倍",
		"preflop-vs-open":    "压制关系",
		"preflop-3bet-pot":   "套池",
		"opponent-read":      "五格",
		"postflop-cbet":      "持续下注频率必须按单挑情形取值",
		"postflop-multiway":  "多人池铁律",
		"postflop-made-hand": "顶底两对最脆弱",
		"postflop-draw":      "封锁下注",
		"postflop-turn":      "转牌不是耍花招的时候",
		"river-value":        "钟形曲线",
		"river-medium":       "中等牌力下注是最糟糕的错误之一",
		"mindset":            "Bad beat",
	}
	for _, s := range AllSkills() {
		want, ok := must[s.Code]
		if !ok {
			t.Errorf("技能 %s 没有登记内容抽查项，拆分后请补上", s.Code)
			continue
		}
		if !strings.Contains(s.body, want) {
			t.Errorf("技能 %s 的正文里找不到 %q，内容可能在拆分时丢了", s.Code, want)
		}
	}
}

// ---------- 翻前局面形态 ----------

func TestPreflopShapes(t *testing.T) {
	cases := []struct {
		name    string
		heroPos string
		actions []models.StreetAction
		want    []string
	}{
		{
			"hero 开池后全弃",
			"BTN",
			[]models.StreetAction{act("BTN", models.ActionRaise), act("SB", models.ActionFold), act("BB", models.ActionFold)},
			[]string{ShapeUnopened},
		},
		{
			"hero 开池后被 3bet 再弃牌",
			"BTN",
			[]models.StreetAction{act("BTN", models.ActionRaise), act("SB", models.ActionRaise), act("BB", models.ActionFold), act("BTN", models.ActionFold)},
			// 两个决策点：开池时无人加注，面对 3bet 时已有再加注
			[]string{ShapeThreeBet, ShapeUnopened},
		},
		{
			"hero 面对一次开池",
			"BB",
			[]models.StreetAction{act("CO", models.ActionRaise), act("BB", models.ActionFold)},
			[]string{ShapeSingleRaise},
		},
		{
			"hero 面对 3bet",
			"BB",
			[]models.StreetAction{act("CO", models.ActionRaise), act("BTN", models.ActionRaise), act("BB", models.ActionFold)},
			[]string{ShapeThreeBet},
		},
		{
			"hero 翻前没行动（走牌）",
			"BB",
			[]models.StreetAction{act("CO", models.ActionRaise), act("SB", models.ActionFold)},
			[]string{ShapeSingleRaise},
		},
		{
			"全是平跟",
			"BB",
			[]models.StreetAction{act("CO", models.ActionCall), act("BB", models.ActionCheck)},
			[]string{ShapeUnopened},
		},
		{
			"老数据用聚合角色 hero",
			"BTN",
			[]models.StreetAction{act(models.ActorHero, models.ActionRaise), act(models.ActorVillain, models.ActionFold)},
			[]string{ShapeUnopened},
		},
		{
			"全下算加注",
			"BB",
			[]models.StreetAction{act("CO", models.ActionAllin), act("BB", models.ActionFold)},
			[]string{ShapeSingleRaise},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PreflopShapes(preflopHand(c.heroPos, c.actions...))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("得到 %v，期望 %v", got, c.want)
			}
		})
	}
}

func TestPreflopShapesEdgeCases(t *testing.T) {
	// 没有翻前行动
	if got := PreflopShapes(handWith([]models.StreetRecord{street(models.StreetFlop, "BTN")})); got != nil {
		t.Errorf("没有翻前行动应当返回 nil，得到 %v", got)
	}
	// 空行动记录
	if got := PreflopShapes(handWith([]models.StreetRecord{{Street: models.StreetPreflop}})); got != nil {
		t.Errorf("空的翻前记录应当返回 nil，得到 %v", got)
	}
	if got := PreflopShapes(nil); got != nil {
		t.Errorf("nil 手牌应当返回 nil，得到 %v", got)
	}
	// 输出里不该有重复项。翻前的加注次数只会递增，所以同一次决策不会得到
	// 相同形态 —— 去重是防御性的，这里只断言这条不变量成立
	h := preflopHand("BTN",
		act("BTN", models.ActionRaise), act("SB", models.ActionRaise),
		act("BB", models.ActionCall), act("BTN", models.ActionCall))
	got := PreflopShapes(h)
	seen := map[string]bool{}
	for _, shape := range got {
		if seen[shape] {
			t.Errorf("形态重复：%v", got)
		}
		seen[shape] = true
	}
	if len(got) != 2 {
		t.Errorf("hero 有两次决策（开池、面对 3bet），应当得到 2 个形态，实际 %v", got)
	}
}

// ---------- 匹配 ----------

func TestSelectSkillsAlwaysIsAlwaysSelected(t *testing.T) {
	all := []Skill{newTestSkill("always", 1, SkillTrigger{Always: true})}

	for _, hand := range []*models.ReviewHand{nil, {}, handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})} {
		got := SelectSkills(all, hand)
		if len(got) != 1 || got[0].Code != "always" {
			t.Errorf("常驻技能没被选中：%v", codesOf(got))
		}
	}
}

// 全零触发器不该默认命中：没写适用范围的技能不能霸占预算
func TestSelectSkillsZeroTriggerIsNotMatched(t *testing.T) {
	all := []Skill{newTestSkill("wildcard", 1, SkillTrigger{})}
	if got := SelectSkills(all, handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})); len(got) != 0 {
		t.Errorf("全零触发器不该命中，却选中了 %v", codesOf(got))
	}
}

// 非常驻技能在 nil 手牌上不该命中 —— 没有场景就没有可匹配的东西
func TestSelectSkillsNonAlwaysOnNilHand(t *testing.T) {
	all := []Skill{newTestSkill("scoped", 1, SkillTrigger{Streets: []string{models.StreetFlop}})}
	if got := SelectSkills(all, nil); len(got) != 0 {
		t.Errorf("nil 手牌上非常驻技能不该命中，得到 %v", codesOf(got))
	}
}

func TestSelectSkillsByDimension(t *testing.T) {
	flopHand := handWith([]models.StreetRecord{
		street(models.StreetPreflop, "BTN", "CO"),
		street(models.StreetFlop, "BTN", "CO"),
	})

	cases := []struct {
		name    string
		trigger SkillTrigger
		hand    *models.ReviewHand
		want    bool
	}{
		{"街道命中", SkillTrigger{Streets: []string{models.StreetFlop}}, flopHand, true},
		{"街道不命中", SkillTrigger{Streets: []string{models.StreetTurn}}, flopHand, false},
		{"只算有行动的街", SkillTrigger{Streets: []string{models.StreetTurn}}, handWith([]models.StreetRecord{
			street(models.StreetTurn), // 空行动记录不算「打到转牌」
		}), false},
		{"位置命中", SkillTrigger{Positions: []string{"BTN"}}, flopHand, true},
		{"位置不命中", SkillTrigger{Positions: []string{"BB"}}, flopHand, false},
		{"底池类型命中", SkillTrigger{PotTypes: []string{"hu"}}, flopHand, true},
		{"底池类型不命中", SkillTrigger{PotTypes: []string{"multi"}}, flopHand, false},
		{"人数命中", SkillTrigger{TableSizes: []int{6, 9}}, flopHand, true},
		{"人数不命中", SkillTrigger{TableSizes: []int{2}}, flopHand, false},
		{"多维度取交集·全中", SkillTrigger{Streets: []string{models.StreetFlop}, Positions: []string{"BTN"}}, flopHand, true},
		{"多维度取交集·一失即否", SkillTrigger{Streets: []string{models.StreetFlop}, Positions: []string{"BB"}}, flopHand, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SelectSkills([]Skill{newTestSkill("x", 1, c.trigger)}, c.hand)
			if (len(got) == 1) != c.want {
				t.Errorf("命中=%v，期望 %v", len(got) == 1, c.want)
			}
		})
	}
}

// PreflopShapes 取代了最初设计的 MinActions：数行动条数分不清
// 「hero 开池后全弃」与「hero 开池后被 3bet 再弃牌」—— 两者都是 3 个行动
func TestSelectSkillsByPreflopShape(t *testing.T) {
	openAllFold := preflopHand("BTN",
		act("BTN", models.ActionRaise), act("SB", models.ActionFold), act("BB", models.ActionFold))
	openThenFacing3Bet := preflopHand("BTN",
		act("BTN", models.ActionRaise), act("SB", models.ActionRaise), act("BB", models.ActionFold), act("BTN", models.ActionFold))

	if n := len(openAllFold.Streets[0].Actions); n != 3 {
		t.Fatalf("这手牌应当有 3 个翻前行动，实际 %d", n)
	}
	if n := len(openThenFacing3Bet.Streets[0].Actions); n != 4 {
		t.Fatalf("这手牌应当有 4 个翻前行动，实际 %d", n)
	}

	open := newTestSkill("preflop-open", 1, SkillTrigger{PreflopShapes: []string{ShapeUnopened}})
	threeBet := newTestSkill("preflop-3bet-pot", 1, SkillTrigger{PreflopShapes: []string{ShapeThreeBet}})

	// 开池后全弃：只该展开开池技能
	got := codesOf(SelectSkills([]Skill{open, threeBet}, openAllFold))
	if strings.Join(got, ",") != "preflop-open" {
		t.Errorf("开池后全弃：得到 %v，期望只展开 preflop-open", got)
	}

	// 开池后被 3bet：两个决策点都发生了，两篇都该展开。
	// 顺序按 priority：注册表里 3bet 那篇（66）排在开池（65）之前
	got = codesOf(SelectSkills([]Skill{open, threeBet}, openThenFacing3Bet))
	if strings.Join(got, ",") != "preflop-3bet-pot,preflop-open" {
		t.Errorf("开池后被 3bet：得到 %v，期望两篇都展开", got)
	}
}

// 对手读牌类技能要能被挡在没有对手信息的手牌之外
func TestSelectSkillsMinVillains(t *testing.T) {
	noVillain := handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})

	withPosition := handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})
	withPosition.Villains = []models.VillainInfo{{Position: "CO"}}

	withNameOnly := handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})
	withNameOnly.Villains = []models.VillainInfo{{Name: "老王"}}

	// 只记了筹码的老数据不算：那种信息不足以做任何形象判断
	stackOnly := handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})
	stack := 100.0
	stackOnly.Villains = []models.VillainInfo{{StackBB: &stack}}

	trigger := SkillTrigger{MinVillains: 1}

	cases := []struct {
		name string
		hand *models.ReviewHand
		want bool
	}{
		{"没有对手", noVillain, false},
		{"有位置的对手", withPosition, true},
		{"有名字的对手", withNameOnly, true},
		{"只有筹码的老数据", stackOnly, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SelectSkills([]Skill{newTestSkill("opponent-read", 1, trigger)}, c.hand)
			if (len(got) == 1) != c.want {
				t.Errorf("命中=%v，期望 %v", len(got) == 1, c.want)
			}
		})
	}
}

// 关键词是唯一会读用户输入的触发维度
func TestSelectSkillsKeywords(t *testing.T) {
	hand := handWith(nil)
	hand.HeroThought = "这把我有点上头了，想追回来"

	if got := SelectSkills([]Skill{newTestSkill("mindset", 1, SkillTrigger{Keywords: []string{"上头"}})}, hand); len(got) != 1 {
		t.Error("想法里出现关键词应当命中")
	}
	if got := SelectSkills([]Skill{newTestSkill("mindset", 1, SkillTrigger{Keywords: []string{"抽水"}})}, hand); len(got) != 0 {
		t.Error("关键词没出现不该命中")
	}

	hand.HeroThought = ""
	hand.Title = "深筹码对决"
	if got := SelectSkills([]Skill{newTestSkill("deep", 1, SkillTrigger{Keywords: []string{"深筹码"}})}, hand); len(got) != 1 {
		t.Error("标题里的关键词应当命中")
	}
}

// ---------- 排序与裁剪 ----------

// 排序必须完全确定：同一手牌两次分析的提示词不一致会让 input_snapshot 失去可复现性。
// 三级顺序为 priority 降序 → 命中维度数降序 → code 字典序
func TestSelectSkillsOrdering(t *testing.T) {
	preflopFlop := []string{models.StreetPreflop, models.StreetFlop}
	all := []Skill{
		newTestSkill("bbb", 10, SkillTrigger{Streets: preflopFlop, Positions: []string{"BTN"}, PotTypes: []string{"hu"}}), // 3 维
		newTestSkill("aaa", 10, SkillTrigger{Streets: []string{models.StreetPreflop}, Positions: []string{"BTN"}}),        // 2 维
		newTestSkill("ccc", 20, SkillTrigger{Streets: []string{models.StreetPreflop}}),                                    // priority 最高
		newTestSkill("eee", 10, SkillTrigger{Positions: []string{"BTN"}}),                                                 // 1 维，code 较大
		newTestSkill("ddd", 10, SkillTrigger{Streets: []string{models.StreetFlop}}),                                       // 1 维，code 较小
	}
	hand := handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN"), street(models.StreetFlop, "BTN")})

	// ccc 靠 priority 居首；bbb/aaa 靠命中维度数领先；ddd/eee 同分同维度，按 code 排
	want := []string{"ccc", "bbb", "aaa", "ddd", "eee"}
	for i := 0; i < 20; i++ {
		got := codesOf(SelectSkills(all, hand))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("第 %d 次排序为 %v，期望 %v", i, got, want)
		}
	}
}

// 常驻技能不占篇数上限：它们不是「某场景的知识」，是所有场景的共同前提
func TestSelectSkillsExpandedCap(t *testing.T) {
	all := []Skill{newTestSkill("resident", 1000, SkillTrigger{Always: true})}
	for _, code := range []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"} {
		all = append(all, newTestSkill(code, 1, SkillTrigger{Streets: []string{models.StreetPreflop}}))
	}
	hand := handWith([]models.StreetRecord{street(models.StreetPreflop, "BTN")})

	got := SelectSkills(all, hand)
	if len(got) != skillMaxExpanded+1 {
		t.Fatalf("选中 %d 篇，期望 1 篇常驻 + %d 篇展开", len(got), skillMaxExpanded)
	}
	if got[0].Code != "resident" {
		t.Errorf("常驻技能应当排在首位，实际 %v", codesOf(got))
	}
}

// ---------- 目录层 ----------

// catalogLine 取出目录里某一篇技能所在的那一行，顺便断言它确实存在
func catalogLine(t *testing.T, cat, code string) string {
	t.Helper()
	for _, line := range strings.Split(cat, "\n") {
		if strings.Contains(line, " "+code) || strings.Contains(line, "★ "+code) {
			return line
		}
	}
	t.Fatalf("目录里找不到 %s：\n%s", code, cat)
	return ""
}

func TestRenderSkillCatalog(t *testing.T) {
	all := AllSkills()
	selected := SelectSkills(all, nil) // nil 手牌 → 只有常驻技能
	cat := RenderSkillCatalog(all, selected)

	// 每一篇都要出现在目录里，否则模型不知道有这个方面可考虑
	expanded := map[string]bool{}
	for _, s := range selected {
		expanded[s.Code] = true
	}

	for _, s := range all {
		line := catalogLine(t, cat, s.Code)

		// 已展开的打 ★，且只有标题 —— 它的一句话结论在下面的全文里本来就有，
		// 再列一遍是纯冗余（15 篇技能时这一项能省掉近一半目录字数）
		if expanded[s.Code] {
			if !strings.HasPrefix(line, "★ ") {
				t.Errorf("%s 已展开却没打 ★：%q", s.Code, line)
			}
			if strings.Contains(line, "｜结论：") {
				t.Errorf("%s 已展开却又列了一遍一句话结论：%q", s.Code, line)
			}
			if !strings.Contains(line, s.Title) {
				t.Errorf("%s 的行里没有标题：%q", s.Code, line)
			}
			continue
		}

		// 未展开的必须带一句话结论：这是「未展开 ≠ 完全不知道」的全部依据
		if strings.HasPrefix(line, "★ ") {
			t.Errorf("%s 没展开却打了 ★：%q", s.Code, line)
		}
		if !strings.Contains(line, s.Summary) {
			t.Errorf("未展开的 %s 缺少一句话结论：%q", s.Code, line)
		}
	}

	// 出处与「结论要能指回规则」这条总要求原本写在方法论正文开头，
	// 拆成多篇后必须由目录承载 —— 掉了这条，模型就失去了引用义务
	if !strings.Contains(cat, "小绿皮书") {
		t.Error("目录里缺少方法论出处")
	}
	if !strings.Contains(cat, "指回") {
		t.Error("目录里缺少「每条结论都要能指回某条规则」的要求")
	}
}

func TestRenderSkillCatalogEmpty(t *testing.T) {
	if got := RenderSkillCatalog(nil, nil); got != "" {
		t.Errorf("没有技能时应当是空串，得到 %q", got)
	}
}

// ---------- 正文层 ----------

func TestRenderSkillBlock(t *testing.T) {
	selected := []Skill{
		newTestSkill("a", 1, SkillTrigger{Always: true}),
		newTestSkill("b", 1, SkillTrigger{Always: true}),
	}
	got := RenderSkillBlock(selected)
	want := "## a\n正文-a\n\n## b\n正文-b\n"
	if got != want {
		t.Errorf("拼装结果不对：\n得到 %q\n期望 %q", got, want)
	}
}

// 正文不以换行结尾时，两篇不能被黏在一起。
// 这不是假想情况：.md 文件末尾少一个空行就会这样
func TestRenderSkillBlockNormalizesMissingTrailingNewline(t *testing.T) {
	selected := []Skill{
		{Code: "a", Title: "甲", body: "没有结尾换行"},  // 故意不带 \n
		{Code: "b", Title: "乙", body: "带结尾换行\n"}, // 正常情况
	}
	got := RenderSkillBlock(selected)
	want := "## 甲\n没有结尾换行\n\n## 乙\n带结尾换行\n"
	if got != want {
		t.Errorf("拼装结果不对：\n得到 %q\n期望 %q", got, want)
	}
}

// 每一篇的正文都要**原样**出现在系统提示词里，且与下一节之间恰好隔一个空行。
// 正文被转义、被截断，或者接缝处多/少一个换行，都会让 input_snapshot 的 diff
// 出现噪音，从而掩盖真正的提示词变更
func TestSkillBodiesAppearVerbatim(t *testing.T) {
	hand := preflopHand("BTN",
		act("BTN", models.ActionRaise), act("SB", models.ActionRaise), act("BB", models.ActionFold), act("BTN", models.ActionFold))
	hand.Villains = []models.VillainInfo{{Position: "SB"}}
	hand.HeroThought = "有点上头"

	prompt := BuildSystemPrompt(hand, nil)
	selected := SelectSkills(AllSkills(), hand)

	if len(selected) < 4 {
		t.Fatalf("这手牌应当展开多篇技能，实际只有 %v", codesOf(selected))
	}
	for _, s := range selected {
		section := "## " + s.Title + "\n" + s.body
		if !strings.Contains(prompt, section) {
			t.Errorf("技能 %s 的正文没有原样出现在提示词里", s.Code)
		}
	}
}

// ---------- 与提示词的整体接缝 ----------

func TestPromptHasCatalogAndConstraints(t *testing.T) {
	prompt := BuildSystemPrompt(nil, nil)

	if !strings.Contains(prompt, "## 教练技能目录") {
		t.Error("提示词里没有技能目录层")
	}
	for _, n := range []string{"13. ", "14. ", "15. ", "16. "} {
		if !strings.Contains(prompt, n) {
			t.Errorf("提示词里缺少硬性约束 %s", n)
		}
	}
	// 目录必须在正文之前：先让模型知道有哪些方面，再给展开的细节
	if strings.Index(prompt, "## 教练技能目录") > strings.Index(prompt, "## 底层立场与决策流程") {
		t.Error("技能目录应当排在技能正文之前")
	}
	// 目录与正文都必须在安全声明之前
	if strings.Index(prompt, "## 安全声明") < strings.Index(prompt, "## 底层立场与决策流程") {
		t.Error("技能层应当排在安全声明之前")
	}
}

// ---------- 牌力与局面触发（S3） ----------

// toRiver 造一手打到河牌的手牌
func toRiver(heroCards, board string) *models.ReviewHand {
	h := handWith([]models.StreetRecord{
		street(models.StreetPreflop, "BTN", "CO"),
		street(models.StreetFlop, "BTN", "CO"),
		street(models.StreetTurn, "BTN", "CO"),
		street(models.StreetRiver, "BTN", "CO"),
	})
	h.HeroCards = heroCards
	h.Board = board
	return h
}

func TestSelectSkillsByMadeHand(t *testing.T) {
	// 牌面 9♠8♠2♦，hero 两对（9 与 8 各配一张）
	twoPair := toRiver("9d8h", "9s8s2d")
	// 同一牌面上只有顶对
	topPair := toRiver("9dAc", "9s8s2d")

	trigger := SkillTrigger{MinMade: utils.MadeTwoPair, Streets: postflopStreets}

	if got := SelectSkills([]Skill{newTestSkill("made", 1, trigger)}, twoPair); len(got) != 1 {
		t.Error("两对应当展开成牌打法")
	}
	if got := SelectSkills([]Skill{newTestSkill("made", 1, trigger)}, topPair); len(got) != 0 {
		t.Error("顶对不该展开成牌打法（那一篇从两对开始）")
	}
}

// 相对牌力必须真的影响路由 —— 这是整套牌力分类存在的意义。
// 同一个顺子，牌面不同 → 该不该为它打大池完全不同 → 展开的技能也必须不同
func TestSelectSkillsStrengthRoutingDependsOnBoard(t *testing.T) {
	// 干燥面：无同花面、无公对 → 顺子是准坚果
	dry := toRiver("JsTs", "9d8c7h2h3d")
	// 同花面 + 公对 → 同一个顺子只是中等牌
	wet := toRiver("JdTd", "9s8s9c7s2h")

	for _, c := range []struct {
		name string
		hand *models.ReviewHand
	}{
		{"干燥面", dry},
		{"同花面+公对", wet},
	} {
		st, ok := handStrength(c.hand)
		if !ok {
			t.Fatalf("%s：算不出牌力", c.name)
		}
		if st.Made != utils.MadeStraight {
			t.Fatalf("%s：成牌等级应当是顺子，得到 %s", c.name, st.Made)
		}
		t.Logf("%s：%s / %s", c.name, st.Made, st.Tier)
	}

	value := newTestSkill("river-value", 1, SkillTrigger{
		Streets: []string{models.StreetRiver}, RequireStrength: true,
		MinStrength: utils.TierStrong, MaxStrength: utils.TierNuts,
	})
	medium := newTestSkill("river-medium", 1, SkillTrigger{
		Streets: []string{models.StreetRiver}, RequireStrength: true,
		MinStrength: utils.TierWeak, MaxStrength: utils.TierMedium,
	})

	// 干燥面上的顺子 → 价值下注
	if got := codesOf(SelectSkills([]Skill{value, medium}, dry)); strings.Join(got, ",") != "river-value" {
		t.Errorf("干燥面上的顺子应当走价值下注，得到 %v", got)
	}
	// 同花面 + 公对上的同一个顺子 → 中等牌力
	if got := codesOf(SelectSkills([]Skill{value, medium}, wet)); strings.Join(got, ",") != "river-medium" {
		t.Errorf("潮湿面上的顺子应当走中等牌力，得到 %v", got)
	}
}

func TestSelectSkillsByDraw(t *testing.T) {
	// 四张同花
	flushDraw := toRiver("AsKs", "Qs9s2d7h")
	// 已成同花
	madeFlush := toRiver("AsKs", "Qs9s2s7s")

	trigger := SkillTrigger{RequireDraw: true, Streets: postflopStreets}

	if got := SelectSkills([]Skill{newTestSkill("draw", 1, trigger)}, flushDraw); len(got) != 1 {
		t.Error("四张同花应当展开听牌技能")
	}
	if got := SelectSkills([]Skill{newTestSkill("draw", 1, trigger)}, madeFlush); len(got) != 0 {
		t.Error("已经成了同花就不该再按听牌展开")
	}
}

// 牌力算不出来时（手牌或牌面缺失）必须当作「这一维不命中」，
// 而不是当成空气 —— 后者会让「河牌中等牌」在手牌记不全时错误展开
func TestSelectSkillsStrengthUnavailable(t *testing.T) {
	incomplete := toRiver("", "") // 有河牌的行动，但没有牌面

	stimed := newTestSkill("river-medium", 1, SkillTrigger{
		Streets: []string{models.StreetRiver}, RequireStrength: true,
		MinStrength: utils.TierWeak, MaxStrength: utils.TierMedium,
	})
	if got := SelectSkills([]Skill{stimed}, incomplete); len(got) != 0 {
		t.Error("算不出牌力时不该命中")
	}

	// 只看街道的技能不受影响
	byStreet := newTestSkill("postflop-turn", 1, SkillTrigger{Streets: []string{models.StreetTurn}})
	if got := SelectSkills([]Skill{byStreet}, incomplete); len(got) != 1 {
		t.Error("只依赖街道的技能不该被牌力缺失影响")
	}
}

func TestSelectSkillsByCbetSpot(t *testing.T) {
	cases := []struct {
		name string
		hand *models.ReviewHand
		want bool
	}{
		{
			"翻前加注者翻牌先下注",
			handWith([]models.StreetRecord{
				{Street: models.StreetPreflop, Actions: []models.StreetAction{
					act("BTN", models.ActionRaise), act("CO", models.ActionCall)}},
				{Street: models.StreetFlop, Actions: []models.StreetAction{act("BTN", models.ActionBet)}},
			}),
			true,
		},
		{
			"翻前加注者翻牌面对别人的下注",
			handWith([]models.StreetRecord{
				{Street: models.StreetPreflop, Actions: []models.StreetAction{
					act("BTN", models.ActionRaise), act("CO", models.ActionCall)}},
				{Street: models.StreetFlop, Actions: []models.StreetAction{
					act("CO", models.ActionBet), act("BTN", models.ActionCall)}},
			}),
			// 那是「要不要跟注或加注」，不是持续下注 —— 拿持续下注的频率表
			// 去点评这种局面会给出方向相反的建议
			false,
		},
		{
			"全是平跟，没有加注者",
			handWith([]models.StreetRecord{
				{Street: models.StreetPreflop, Actions: []models.StreetAction{
					act("BTN", models.ActionCall), act("CO", models.ActionCheck)}},
				{Street: models.StreetFlop, Actions: []models.StreetAction{act("BTN", models.ActionBet)}},
			}),
			false,
		},
		{
			"翻前加注的是对手",
			handWith([]models.StreetRecord{
				{Street: models.StreetPreflop, Actions: []models.StreetAction{
					act("CO", models.ActionRaise), act("BTN", models.ActionCall)}},
				{Street: models.StreetFlop, Actions: []models.StreetAction{act("CO", models.ActionBet)}},
			}),
			false,
		},
		{
			"翻前加注者翻牌过牌后被对手下注",
			handWith([]models.StreetRecord{
				{Street: models.StreetPreflop, Actions: []models.StreetAction{
					act("BTN", models.ActionRaise), act("CO", models.ActionCall)}},
				{Street: models.StreetFlop, Actions: []models.StreetAction{
					act("BTN", models.ActionCheck), act("CO", models.ActionBet)}},
			}),
			// hero 的第一个翻牌动作是过牌，那一刻还没人下注 —— 那正是持续下注决策点
			true,
		},
	}

	trigger := SkillTrigger{CbetSpot: true}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SelectSkills([]Skill{newTestSkill("cbet", 1, trigger)}, c.hand)
			if (len(got) == 1) != c.want {
				t.Errorf("命中=%v，期望 %v", len(got) == 1, c.want)
			}
		})
	}
}

// 15 篇技能下，一手「什么都有」的手牌会命中很多篇；篇数上限必须真的在起作用，
// 而且裁掉的应当是最不相关的那几篇（翻前细则），不是河牌细则
func TestExpandedCapDropsLeastRelevant(t *testing.T) {
	// 三条：干燥面上是强牌 → 河牌价值下注；多人池 + 用户自己写了情绪
	h := toRiver("9d9h", "9s8s2d7c3h")
	h.PotType = "multi"
	h.Villains = []models.VillainInfo{
		{Position: "CO"}, {Position: "BB"}, {Position: "SB"},
	}
	h.HeroThought = "有点上头"

	got := codesOf(SelectSkills(AllSkills(), h))
	t.Logf("展开 %d 篇：%v", len(got), got)

	expanded := 0
	for _, s := range SelectSkills(AllSkills(), h) {
		if !s.Trigger.Always {
			expanded++
		}
	}
	if expanded > skillMaxExpanded {
		t.Errorf("非常驻技能展开 %d 篇，超过上限 %d", expanded, skillMaxExpanded)
	}

	bodies := strings.Join(got, ",")
	// 与这手牌直接相关的必须活下来：河牌、成牌、多人池，以及用户自己写了"上头"
	for _, must := range []string{"river-value", "postflop-made-hand", "postflop-multiway", "mindset", "opponent-read"} {
		if !strings.Contains(bodies, must) {
			t.Errorf("被裁掉的应当是翻前细则，不该是 %s；实际展开 %v", must, got)
		}
	}
	// 被裁掉的应当是与「河牌」最不相关的翻前细则
	if strings.Contains(bodies, "preflop-open") {
		t.Errorf("篇数超限时应当先裁翻前细则，实际展开 %v", got)
	}
}

// 体量日志只观测：返回值是两层的真实字数，且**传入的技能一个字都不能少**。
// 这里没有阈值可断言 —— 数值上限是刻意移除的（见 LogSkillLayerSizes 的注释）
func TestLogSkillLayerSizesObservesWithoutTrimming(t *testing.T) {
	sel := []Skill{
		{Code: "r", Trigger: SkillTrigger{Always: true}, body: strings.Repeat("字", 7)},
		{Code: "a", body: strings.Repeat("字", 5)},
	}
	before := []string{sel[0].body, sel[1].body}

	cat, body := LogSkillLayerSizes(strings.Repeat("目", 11), sel)
	if cat != 11 {
		t.Errorf("目录层字数 = %d，期望 11", cat)
	}
	if body != 12 {
		t.Errorf("正文层字数 = %d，期望 12（7+5）", body)
	}
	if len(sel) != 2 {
		t.Errorf("技能被裁掉了：剩 %d 篇，期望 2", len(sel))
	}
	for i := range sel {
		if sel[i].body != before[i] {
			t.Errorf("第 %d 篇正文被改动：%q → %q", i, before[i], sel[i].body)
		}
	}
}
