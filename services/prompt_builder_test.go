package services

import (
	"call-go/models"
	"encoding/json"
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
	prompt := BuildSystemPrompt(nil, nil)
	if !strings.Contains(prompt, "位置的含义取决于桌型") {
		t.Errorf("系统提示词应说明位置含义随桌型变化，实际:\n%s", prompt)
	}
}

// v2.0 的输出契约：Schema 里必须出现两个新字段，且字段名与
// models.AnalysisResult 的 json tag 逐字一致。写错一边模型就会把结果
// 塞到不存在的字段里，而 Go 的 json 反序列化不会报错 —— 只会静默丢数据
func TestSystemPromptDeclaresV2SchemaFields(t *testing.T) {
	prompt := BuildSystemPrompt(nil, nil)
	for _, field := range []string{`"opponentRead"`, `"actionAdvice"`} {
		if !strings.Contains(prompt, field) {
			t.Errorf("v2.0 的 Schema 应包含 %s，实际:\n%s", field, prompt)
		}
	}
	// 五格形象的枚举值也必须在提示词里给出，否则模型会自己造词
	for _, profile := range []string{
		models.ProfileLoosePassive,
		models.ProfileTightPassive,
		models.ProfileLooseAggressive,
		models.ProfileTightAggressive,
		models.ProfileUnknown,
	} {
		if !strings.Contains(prompt, profile) {
			t.Errorf("Schema 应给出形象枚举 %s，实际:\n%s", profile, prompt)
		}
	}
}

// 方法论必须真的被拼进分析和追问两条提示词。
//
// 断言的是"内容被拼进来了"而不是整段比对：方法论会随版本继续增删，
// 逐字比对会让每次改文案都要同步改测试，测试就失去意义了
func TestCoachMethodologyInjectedIntoBothPrompts(t *testing.T) {
	// 「说不出理由的过牌同样是漏损」是方法论的核心主张，两条提示词都要有
	const anchor = "说不出理由的过牌"

	analysis := BuildSystemPrompt(nil, nil)
	if !strings.Contains(analysis, anchor) {
		t.Errorf("复盘分析提示词应注入方法论，实际缺少 %q", anchor)
	}
	if !strings.Contains(analysis, "《德州扑克小绿皮书》") {
		t.Errorf("复盘分析提示词应注明方法论出处，实际:\n%s", analysis)
	}

	chat := BuildChatSystemPrompt()
	if !strings.Contains(chat, anchor) {
		t.Errorf("追问提示词应与分析共用同一套方法论口径，实际缺少 %q", anchor)
	}

	// 追问是 300 字以内的短答案场景，不该把全量方法论带上
	if len(chat) >= len(analysis) {
		t.Errorf("追问提示词应使用精简版方法论，实际长度 %d 不小于分析提示词的 %d",
			len(chat), len(analysis))
	}
}

// extractSchemaBlock 从系统提示词里抠出内嵌的 JSON Schema 示例。
//
// Schema 写在 Go 的 raw string 里，编译器不会校验它是不是合法 JSON。
// 少一个逗号、多一个尾逗号，Go 侧照样编译通过，只有模型会开始返回残废结构 ——
// 而那时错误现场在模型侧，排查成本极高。所以这里把它抠出来真的解析一遍。
func extractSchemaBlock(t *testing.T, prompt string) string {
	t.Helper()

	const marker = "字段如下：\n"
	start := strings.Index(prompt, marker)
	if start < 0 {
		t.Fatalf("系统提示词里找不到 %q，实际:\n%s", marker, prompt)
	}
	body := prompt[start+len(marker):]

	// Schema 的结束以「硬性约束」标题为界
	end := strings.Index(body, "\n\n## 硬性约束")
	if end < 0 {
		t.Fatalf("找不到 Schema 的结束边界（## 硬性约束），实际:\n%s", body)
	}
	return body[:end]
}

// 提示词里内嵌的 JSON Schema 必须是合法 JSON
func TestSystemPromptSchemaIsValidJSON(t *testing.T) {
	schema := extractSchemaBlock(t, BuildSystemPrompt(nil, nil))

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		t.Fatalf("提示词里的 JSON Schema 不是合法 JSON：%v\n原文:\n%s", err, schema)
	}

	// 顶层必须是对象且含全部约定字段。漏字段会让模型不知道该输出什么
	for _, key := range []string{"handSummary", "opponentRead", "actionAdvice", "streetAnalysis"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("Schema 顶层缺少字段 %q，实际:\n%s", key, schema)
		}
	}
}

// Schema 里声明的字段名必须能真的反序列化进 models.AnalysisResult。
//
// 这条守的是「改名必须两边一起改」那个约定：Go 的 json 反序列化遇到不认识的
// 字段不报错，只是静默丢掉。真出问题时的症状是"AI 分析结果里新字段永远是空的"，
// 而模型侧完全正常 —— 很难查。
func TestAnalysisResultDeserializesSchemaShapedPayload(t *testing.T) {
	// 形状完全按提示词里的 Schema 手写，模拟模型的真实返回
	const payload = `{
	  "handSummary": "翻前加注后翻牌过牌，把主动权交了出去",
	  "opponentRead": {
	    "profile": "loosePassive",
	    "profileReason": "他三次跟注都没加注，符合跟注站特征",
	    "streets": [
	      {"street": "preflop", "action": "CO 位跟注 3BB", "rangeKept": "中小对子、同花连牌", "rangeDropped": "AA/KK/AK 等强牌"},
	      {"street": "flop", "action": "过牌", "rangeKept": "全部保留", "rangeDropped": ""}
	    ],
	    "conclusion": "成牌约五成、听牌三成、纯诈唬两成"
	  },
	  "actionAdvice": [
	    {"street": "flop", "action": "bet", "sizing": "1/2池(12BB)", "reason": "我是翻前加注者且对手过牌示弱", "targetProfile": "松弱，价值为主"}
	  ],
	  "streetAnalysis": [
	    {"street": "preflop", "verdict": "ok", "comment": "开池尺度合理"}
	  ],
	  "alternatives": [],
	  "leaks": [],
	  "strengths": [],
	  "drills": ["练习在翻牌圈识别对手示弱后的持续下注时机"]
	}`

	var result models.AnalysisResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatalf("Schema 形状的返回体反序列化失败：%v", err)
	}

	if result.OpponentRead == nil {
		t.Fatal("opponentRead 没能反序列化进 AnalysisResult.OpponentRead")
	}
	if result.OpponentRead.Profile != models.ProfileLoosePassive {
		t.Errorf("profile 应为 %q，实际 %q", models.ProfileLoosePassive, result.OpponentRead.Profile)
	}
	if len(result.OpponentRead.Streets) != 2 {
		t.Fatalf("streets 应有 2 条，实际 %d", len(result.OpponentRead.Streets))
	}
	if got := result.OpponentRead.Streets[0].RangeKept; got == "" {
		t.Error("streets[].rangeKept 没能反序列化")
	}
	if result.OpponentRead.Conclusion == "" {
		t.Error("opponentRead.conclusion 没能反序列化")
	}

	if len(result.ActionAdvice) != 1 {
		t.Fatalf("actionAdvice 应有 1 条，实际 %d", len(result.ActionAdvice))
	}
	if result.ActionAdvice[0].Sizing == "" {
		t.Error("actionAdvice[].sizing 没能反序列化")
	}
	if result.ActionAdvice[0].TargetProfile == "" {
		t.Error("actionAdvice[].targetProfile 没能反序列化")
	}
}

// v2.0 之前的存量分析没有新字段，追问时 formatAnalysisForChat 不能炸。
// 这条守的是向后兼容 —— dev 库里已经有 v1.2 的历史分析了
func TestFormatAnalysisForChatToleratesPreV2Result(t *testing.T) {
	old := &models.AnalysisResult{
		HandSummary: "翻前加注被跟，翻牌没有持续下注",
		StreetAnalysis: []models.StreetAnalysisItem{
			{Street: models.StreetFlop, Verdict: "mistake", Comment: "该下注没下"},
		},
		// OpponentRead / ActionAdvice 留空，模拟 v1.2 的产出
	}

	out := formatAnalysisForChat(old)
	if !strings.Contains(out, "翻前加注被跟") {
		t.Errorf("老结果的基本内容应正常输出，实际:\n%s", out)
	}
	if strings.Contains(out, "对手形象") {
		t.Errorf("老结果没有 opponentRead，不该凭空输出这一块，实际:\n%s", out)
	}
}

// 有 opponentRead 时，两块新内容都要出现在追问上下文里 ——
// 否则教练追问时会看不到自己刚给过的范围判断，容易自相矛盾
func TestFormatAnalysisForChatIncludesV2Blocks(t *testing.T) {
	result := &models.AnalysisResult{
		HandSummary: "翻前加注后翻牌过牌",
		OpponentRead: &models.OpponentRead{
			Profile:       models.ProfileLoosePassive,
			ProfileReason: "三次跟注都没加注",
			Streets: []models.RangeStreetItem{
				{Street: models.StreetPreflop, Action: "跟注 3BB", RangeKept: "中小对子", RangeDropped: "AA/KK"},
			},
			Conclusion: "成牌约五成",
		},
		ActionAdvice: []models.ActionAdviceItem{
			{Street: models.StreetFlop, Action: "bet", Sizing: "1/2池(12BB)", Reason: "对手过牌示弱", TargetProfile: "松弱"},
		},
	}

	out := formatAnalysisForChat(result)

	for _, want := range []string{"松弱（跟注站）", "中小对子", "成牌约五成", "下注", "1/2池(12BB)", "松弱"} {
		if !strings.Contains(out, want) {
			t.Errorf("追问上下文应包含 %q，实际:\n%s", want, out)
		}
	}
}
