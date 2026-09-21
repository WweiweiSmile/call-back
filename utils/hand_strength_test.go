package utils

import (
	"strings"
	"testing"
)

// ---------- 牌面结构 ----------

func TestAnalyzeBoard(t *testing.T) {
	cases := []struct {
		name  string
		board string
		want  BoardTexture
	}{
		{"干燥面", "9d8c2h", BoardTexture{HighRank: 9}},
		{"公对", "9s9d2c", BoardTexture{Paired: true, HighRank: 9}},
		{"公牌三条", "9s9d9c", BoardTexture{Paired: true, TripsOnBoard: true, HighRank: 9}},
		{"两张同花不算同花面", "Qs9s2d", BoardTexture{HighRank: 12}},
		{"三张同花", "Qs9s2s", BoardTexture{ThreeFlush: true, HighRank: 12}},
		{"四张同花", "Qs9s2s4s", BoardTexture{ThreeFlush: true, FourFlush: true, HighRank: 12}},
		{"A高", "Ad8c2h", BoardTexture{AceHigh: true, HighRank: 14}},
		{"连通面", "9d8c7h", BoardTexture{Connected: true, HighRank: 9}},
		{"两间隔也算连通", "Td8c6h", BoardTexture{Connected: true, HighRank: 10}},
		{"跨度太大不算连通", "Kd8c2h", BoardTexture{HighRank: 13}},
		{"A低顺面", "Ad2c3h", BoardTexture{AceHigh: true, Connected: true, HighRank: 14}},
		{"QKA 连通", "QdKcAh", BoardTexture{AceHigh: true, Connected: true, HighRank: 14}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AnalyzeBoard(splitString(NormalizeCards(c.board)))
			if got.Paired != c.want.Paired || got.TripsOnBoard != c.want.TripsOnBoard ||
				got.ThreeFlush != c.want.ThreeFlush || got.FourFlush != c.want.FourFlush ||
				got.Connected != c.want.Connected || got.AceHigh != c.want.AceHigh ||
				got.HighRank != c.want.HighRank {
				t.Errorf("牌面 %s：\n得到 %+v\n期望 %+v", c.board, got, c.want)
			}
		})
	}
}

// ---------- 绝对成牌等级 ----------

func TestEvaluateHandMadeHand(t *testing.T) {
	cases := []struct {
		name  string
		hero  string
		board string
		want  MadeHand
	}{
		{"无成牌", "AsKh", "9d8c2h", MadeNone},
		{"底对", "Ad8h", "Qd8c2h", MadeWeakPair},
		{"中对", "Td8h", "Qd8c2h", MadeWeakPair},
		{"顶对", "AdQh", "Qd8c2h", MadeTopPair},
		{"超对", "KhKs", "Qd8c2h", MadeOverpair},
		{"口袋对但小于公牌最大是弱对", "8h8s", "Qd8c2h", MadeSet},
		{"两对", "AdQh", "Qd8c8h", MadeTwoPair},
		{"三条·口袋对中牌", "8d8h", "Qd8c2h", MadeSet},
		{"明三条·公对配手牌", "AdQh", "QdQc2h", MadeTrips},
		{"顺子", "JdTd", "9s8s9c7s", MadeStraight},
		{"A低顺", "Ad2h", "3s4c5d", MadeStraight},
		{"同花", "AsKs", "Qs9s2s", MadeFlush},
		{"葫芦", "9d9h", "9sQdQc", MadeFullHouse},
		{"四条", "9d9h", "9s9cQd", MadeQuads},
		{"同花顺", "AsKs", "QsJsTs", MadeStraightFlush},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EvaluateHand(c.hero, c.board)
			if err != nil {
				t.Fatal(err)
			}
			if got.Made != c.want {
				t.Errorf("hero %s 在 %s 上：得到 %s，期望 %s", c.hero, c.board, got.Made, c.want)
			}
		})
	}
}

// 「公牌三条 + hero 两张杂牌」曾经被误判成葫芦 —— 因为「三条那个点数」自己
// 也算「出现两次以上」。那是最糟的一类错误：会把人人都有的一副牌说成坚果
func TestBoardTripsIsNotFullHouse(t *testing.T) {
	got, err := EvaluateHand("AdKh", "9s9d9c")
	if err != nil {
		t.Fatal(err)
	}
	if got.Made != MadeTrips {
		t.Fatalf("得到 %s，期望明三条（绝不能是葫芦）", got.Made)
	}
	// 公牌自己有三条，hero 的三条并不比别人的强多少，必须降级
	if got.Tier != TierStrong {
		t.Errorf("公牌三条面上持明三条：得到 %s，期望被降一档到强", got.Tier)
	}
	if len(got.Notes) == 0 {
		t.Error("降级必须留下原因")
	}
}

// ---------- 相对牌力：用户口径的两个例子 ----------

// 同花面 + 公对面上的顺子 ——「相对牌力一般」
func TestStraightOnFlushAndPairedBoardIsMedium(t *testing.T) {
	// 牌面 9♠8♠9♣7♠：三张同花 + 公对；hero J♦T♦ 接成 J 高顺
	got, err := EvaluateHand("JdTd", "9s8s9c7s")
	if err != nil {
		t.Fatal(err)
	}
	if got.Made != MadeStraight {
		t.Fatalf("成牌等级应当是顺子，得到 %s", got.Made)
	}
	if got.Tier != TierMedium {
		t.Errorf("得到 %s，期望中等（对手范围里合理存在同花与葫芦）", got.Tier)
	}
	// 两条降级原因都要留痕，方便回溯「为什么这个顺子只算中等」
	if len(got.Notes) != 2 {
		t.Errorf("期望留下 2 条降级原因，实际 %d 条：%v", len(got.Notes), got.Notes)
	}
}

// 无同花面、无公对面的顺子 ——「牌力较好」
func TestStraightOnDryBoardIsVeryStrong(t *testing.T) {
	// 牌面 9♦8♣7♥2♥：既无同花面也无公对；hero J♠T♠ 接成 J 高顺
	got, err := EvaluateHand("JsTs", "9d8c7h2h")
	if err != nil {
		t.Fatal(err)
	}
	if got.Made != MadeStraight {
		t.Fatalf("成牌等级应当是顺子，得到 %s", got.Made)
	}
	if got.Tier != TierVeryStrong {
		t.Errorf("得到 %s，期望很强（只有更大的顺子能赢它）", got.Tier)
	}
	if len(got.Notes) != 0 {
		t.Errorf("干燥面上不该有降级，实际：%v", got.Notes)
	}
}

// 同一个顺子，牌面不同 → 档位必须不同。这是本文件存在的全部理由
func TestSameMadeHandDifferentBoardDifferentTier(t *testing.T) {
	dry, _ := EvaluateHand("JsTs", "9d8c7h2h")
	wet, _ := EvaluateHand("JdTd", "9s8s9c7s")

	if dry.Made != MadeStraight || wet.Made != MadeStraight {
		t.Fatal("两手牌的绝对成牌等级都应当是顺子")
	}
	if !(dry.Tier > wet.Tier) {
		t.Errorf("干燥面 %s 应当高于潮湿面 %s", dry.Tier, wet.Tier)
	}
	t.Logf("干燥面 %s（%d）> 潮湿面 %s（%d）", dry.Tier, dry.Tier, wet.Tier, wet.Tier)
}

// ---------- 公牌自己成牌 ----------

func TestBoardPlays(t *testing.T) {
	cases := []struct {
		name  string
		hero  string
		board string
		want  bool
	}{
		{"公牌成顺·hero 没帮忙", "2d3c", "5s6d7h8c9s", true},
		{"公牌成花·hero 没帮忙", "2d3c", "AsKsQsJs9s", true},
		{"hero 用两张接成更大的顺", "TdJc", "5s6d7h8c9s", false},
		{"hero 有超对", "KhKs", "Qd8c2h", false},
		{"hero 中三条", "8d8h", "Qd8c2h", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EvaluateHand(c.hero, c.board)
			if err != nil {
				t.Fatal(err)
			}
			if got.BoardPlays != c.want {
				t.Errorf("hero %s 在 %s 上：BoardPlays 得到 %v，期望 %v",
					c.hero, c.board, got.BoardPlays, c.want)
			}
		})
	}
}

// 「公牌成顺、hero 用更大的牌接成更大的顺」曾经被判成「公牌成牌」——
// 因为旧实现只比成牌**等级**，两个都是「顺子」就认为 hero 没帮忙
func TestHeroExtendingBoardStraightIsNotBoardPlays(t *testing.T) {
	got, err := EvaluateHand("TdJc", "5s6d7h8c9s")
	if err != nil {
		t.Fatal(err)
	}
	if got.Made != MadeStraight {
		t.Fatalf("成牌等级应当是顺子，得到 %s", got.Made)
	}
	if got.BoardPlays {
		t.Fatal("hero 的 J 高顺比公牌的 9 高顺更好，不该判成「公牌成牌」")
	}
	if got.Tier == TierWeak {
		t.Errorf("档位不该被压到弱，实际 %s", got.Tier)
	}
}

// ---------- 听牌 ----------

func TestDraws(t *testing.T) {
	cases := []struct {
		name             string
		hero             string
		board            string
		wantFlushDraw    bool
		wantStraightDraw bool
	}{
		{"四张同花", "AsKs", "Qs9s2d", true, false},
		{"公牌四张同花不算 hero 的听牌", "AdKh", "Qs9s2s4s", false, false},
		{"两头顺", "JdTc", "9s8d2h", false, true},
		{"卡顺", "Jd9c", "8s7d2h", false, true},
		{"已成就不是听牌", "AsKs", "QsJsTs", false, false},
		{"什么都没有", "AdKh", "9d8c2h", false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EvaluateHand(c.hero, c.board)
			if err != nil {
				t.Fatal(err)
			}
			if got.FlushDraw != c.wantFlushDraw {
				t.Errorf("FlushDraw 得到 %v，期望 %v", got.FlushDraw, c.wantFlushDraw)
			}
			if got.StraightDraw != c.wantStraightDraw {
				t.Errorf("StraightDraw 得到 %v，期望 %v", got.StraightDraw, c.wantStraightDraw)
			}
		})
	}
}

// 河牌上没有听牌可言 —— 没有下一张牌了。
// 这是真机验证时发现的：河牌持 A 高（Ad3c 在 Kd8h2s5c9h 上）被算出了
// A2345 的卡顺听牌，导致 postflop-draw 那篇技能在河牌上展开
func TestNoDrawOnRiver(t *testing.T) {
	// 河牌五张，A 高但 A2345 差一张 4
	got, err := EvaluateHand("Ad3c", "Kd8h2s5c9h")
	if err != nil {
		t.Fatal(err)
	}
	if got.StraightDraw || got.FlushDraw {
		t.Errorf("河牌不该报听牌，得到 顺子听牌=%v 同花听牌=%v", got.StraightDraw, got.FlushDraw)
	}

	// 同一手牌在转牌上（四张公共牌）应当还是听牌
	turn, err := EvaluateHand("Ad3c", "Kd8h2s5c")
	if err != nil {
		t.Fatal(err)
	}
	if !turn.StraightDraw {
		t.Error("转牌上 A2345 差一张 4，应当算顺子听牌")
	}
}

// 已经成了顺子就不该再报「顺子听牌」，那是噪音
func TestNoDrawReportWhenAlreadyMade(t *testing.T) {
	got, err := EvaluateHand("JsTs", "9d8c7h")
	if err != nil {
		t.Fatal(err)
	}
	if got.Made != MadeStraight {
		t.Fatalf("成牌等级应当是顺子，得到 %s", got.Made)
	}
	if got.StraightDraw {
		t.Error("已经成顺子还报顺子听牌")
	}
}

// ---------- 输入校验 ----------

func TestEvaluateHandRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		hero  string
		board string
	}{
		{"hero 只有一张", "As", "9d8c2h"},
		{"hero 有三张", "AsKhQd", "9d8c2h"},
		{"公牌只有两张", "AsKh", "9d8c"},
		{"公牌有六张", "AsKh", "9d8c2h3s4d5c"},
		{"公牌为空", "AsKh", ""},
		{"非法点数", "XsKh", "9d8c2h"},
		{"非法花色", "AxKh", "9d8c2h"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := EvaluateHand(c.hero, c.board); err == nil {
				t.Error("应当报错")
			}
		})
	}
}

// 输入容忍度沿用 NormalizeCards：大小写混写、带分隔符都要能正常解析
func TestEvaluateHandNormalizesInput(t *testing.T) {
	want, err := EvaluateHand("KhKs", "Qd8c2h")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"kh ks", "KH,KS", "kH kS"} {
		got, err := EvaluateHand(raw, "Qd8c2h")
		if err != nil {
			t.Fatalf("%q 解析失败：%v", raw, err)
		}
		if got.Made != want.Made || got.Tier != want.Tier {
			t.Errorf("%q 得到 %s/%s，期望 %s/%s", raw, got.Made, got.Tier, want.Made, want.Tier)
		}
	}
}

// ---------- 分值比较 ----------

func TestHandScoreOrdering(t *testing.T) {
	// 同一等级内要比得出来：J 高顺 > 9 高顺
	heroBetter := handScore(splitRanks("TdJc5s6d7h8c9s"), nil)
	boardOnly := handScore(splitRanks("5s6d7h8c9s"), nil)
	if cmpScore(heroBetter, boardOnly) <= 0 {
		t.Error("J 高顺应当大于 9 高顺")
	}

	// A 低顺只算 5 高（wheel），不能按 14 算
	wheel := handScore(splitRanks("Ad2h3s4c5d"), nil)
	six := handScore(splitRanks("2h3s4c5d6h"), nil)
	if cmpScore(wheel, six) >= 0 {
		t.Error("A2345 是 5 高顺，应当小于 23456 的 6 高顺")
	}
}

func splitRanks(cards string) []int {
	ranks, _ := splitCards(NormalizeCards(cards))
	return ranks
}

func TestRankValueAndHelpers(t *testing.T) {
	if rankValue('T') != 10 || rankValue('A') != 14 || rankValue('2') != 2 || rankValue('X') != 0 {
		t.Error("点数换算不对")
	}
	if st := bestStraight([]int{14, 2, 3, 4, 5}); st == nil || st[0] != 5 {
		t.Errorf("A2345 应当成顺且高张是 5，得到 %v", st)
	}
	if st := bestStraight([]int{14, 13, 12, 11, 10, 9}); st == nil || st[0] != 14 {
		t.Errorf("应当取最大的顺子（A 高），得到 %v", st)
	}
}

// ---------- 档位与等级的名字 ----------

func TestTierAndMadeHandStrings(t *testing.T) {
	if !strings.Contains(MadeStraight.String(), "顺子") {
		t.Errorf("顺子的名字不对：%s", MadeStraight)
	}
	if TierMedium.String() != "中等" {
		t.Errorf("中等档位的名字不对：%s", TierMedium)
	}
	// 未定义的取值不能静默变成空串，否则日志里会出现看不懂的空字段
	if got := Tier(99).String(); !strings.Contains(got, "99") {
		t.Errorf("未定义档位应当带上原值，得到 %q", got)
	}
}
