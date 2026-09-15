package utils

import (
	"call-go/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// 合法点数与花色
const (
	validRanks = "23456789TJQKA"
	validSuits = "shdc"
)

var validPositions = map[string]bool{
	models.PositionUTG: true,
	models.PositionMP:  true,
	models.PositionCO:  true,
	models.PositionBTN: true,
	models.PositionSB:  true,
	models.PositionBB:  true,
}

var validActions = map[string]bool{
	models.ActionFold:  true,
	models.ActionCheck: true,
	models.ActionCall:  true,
	models.ActionBet:   true,
	models.ActionRaise: true,
	models.ActionAllin: true,
}

var validActors = map[string]bool{
	models.ActorHero:    true,
	models.ActorVillain: true,
	models.ActorOther:   true,
}

var validResults = map[string]bool{
	models.ResultWin:     true,
	models.ResultLose:    true,
	models.ResultFold:    true,
	models.ResultUnknown: true,
}

var validPotTypes = map[string]bool{
	models.PotTypeHU:    true,
	models.PotTypeMulti: true,
}

// 每条街对应的最少公共牌张数
var streetMinBoardCards = map[string]int{
	models.StreetPreflop: 0,
	models.StreetFlop:    3,
	models.StreetTurn:    4,
	models.StreetRiver:   5,
}

// NormalizeCards 规范化牌面字符串。
// 输入容忍空格、逗号、大小写混写（如 "as kh"、"AS, KH"），
// 统一输出为"点数大写 + 花色小写"的拼接形式，如 "AsKh"。
func NormalizeCards(raw string) string {
	var sb strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= '2' && c <= '9':
			sb.WriteByte(c)
		case c == '1':
			// 不少人按习惯把十写成 "10" 而不是 "T"。单独一个 '1' 不是合法点数，
			// 所以 "10" 没有歧义；只吃 '1' 不吃 '0' 的话 "10s" 会退化成 "s"，
			// 长度校验虽然拦得住，但报错信息会让人摸不着头脑
			if i+1 < len(raw) && raw[i+1] == '0' {
				sb.WriteByte('T')
				i++
			}
		case c == 't' || c == 'T':
			sb.WriteByte('T')
		case c == 'j' || c == 'J':
			sb.WriteByte('J')
		case c == 'q' || c == 'Q':
			sb.WriteByte('Q')
		case c == 'k' || c == 'K':
			sb.WriteByte('K')
		case c == 'a' || c == 'A':
			sb.WriteByte('A')
		case c == 's' || c == 'S':
			sb.WriteByte('s')
		case c == 'h' || c == 'H':
			sb.WriteByte('h')
		case c == 'd' || c == 'D':
			sb.WriteByte('d')
		case c == 'c' || c == 'C':
			sb.WriteByte('c')
		default:
			// 空格、逗号等分隔符直接丢弃
		}
	}
	return sb.String()
}

// ValidateCards 校验牌面字符串：长度必须是 2 的倍数，且每张牌合法
func ValidateCards(cards string) error {
	if len(cards)%2 != 0 {
		return fmt.Errorf("牌面长度不合法：%s", cards)
	}
	for i := 0; i < len(cards); i += 2 {
		rank, suit := cards[i], cards[i+1]
		if !strings.ContainsRune(validRanks, rune(rank)) {
			return fmt.Errorf("非法点数：%c", rank)
		}
		if !strings.ContainsRune(validSuits, rune(suit)) {
			return fmt.Errorf("非法花色：%c", suit)
		}
	}
	return nil
}

// ValidateReviewHand 校验并就地规范化手牌。
// 校验不通过时返回中文错误，可直接透给前端展示。
func ValidateReviewHand(h *models.ReviewHand) error {
	// --- 底牌 ---
	h.HeroCards = NormalizeCards(h.HeroCards)
	if len(h.HeroCards) != 4 {
		return fmt.Errorf("底牌必须是 2 张，当前为 %d 张", len(h.HeroCards)/2)
	}
	if err := ValidateCards(h.HeroCards); err != nil {
		return fmt.Errorf("底牌不合法：%w", err)
	}

	// --- 公共牌 ---
	h.Board = NormalizeCards(h.Board)
	switch len(h.Board) / 2 {
	case 0, 3, 4, 5:
		// 合法张数
	default:
		return fmt.Errorf("公共牌只能是 0、3、4 或 5 张，当前为 %d 张", len(h.Board)/2)
	}
	if err := ValidateCards(h.Board); err != nil {
		return fmt.Errorf("公共牌不合法：%w", err)
	}

	// --- 重复牌 ---
	// 一副牌里同一张牌不可能出现两次，重复说明录入有误，且会让 AI 的分析完全失真
	seen := make(map[string]bool, 7)
	for i := 0; i < len(h.HeroCards); i += 2 {
		card := h.HeroCards[i : i+2]
		if seen[card] {
			return fmt.Errorf("重复的牌：%s", card)
		}
		seen[card] = true
	}
	for i := 0; i < len(h.Board); i += 2 {
		card := h.Board[i : i+2]
		if seen[card] {
			return fmt.Errorf("重复的牌：%s", card)
		}
		seen[card] = true
	}

	// --- 位置 / 枚举字段 ---
	h.HeroPosition = strings.ToUpper(strings.TrimSpace(h.HeroPosition))
	if !validPositions[h.HeroPosition] {
		return fmt.Errorf("无效的位置：%s", h.HeroPosition)
	}

	if h.Result == "" {
		h.Result = models.ResultUnknown
	}
	if !validResults[h.Result] {
		return fmt.Errorf("无效的结果：%s", h.Result)
	}

	if h.PotType == "" {
		// 没填时按对手数量推断，减少前端必填项
		if h.VillainCount > 1 {
			h.PotType = models.PotTypeMulti
		} else {
			h.PotType = models.PotTypeHU
		}
	}
	if !validPotTypes[h.PotType] {
		return fmt.Errorf("无效的底池类型：%s", h.PotType)
	}

	if h.HeroStackBB < 0 {
		return fmt.Errorf("有效筹码不能为负数")
	}
	if h.VillainCount < 0 {
		return fmt.Errorf("对手数量不能为负数")
	}

	// --- 行动序列 ---
	// 记录出现了哪些街，用于下面校验公共牌张数是否匹配
	maxBoardNeeded := 0
	for i := range h.Streets {
		street := &h.Streets[i]
		street.Street = strings.ToLower(strings.TrimSpace(street.Street))

		need, ok := streetMinBoardCards[street.Street]
		if !ok {
			return fmt.Errorf("无效的街道：%s", street.Street)
		}
		if len(street.Actions) > 0 && need > maxBoardNeeded {
			maxBoardNeeded = need
		}

		for j := range street.Actions {
			action := &street.Actions[j]
			if !validActors[action.Actor] {
				return fmt.Errorf("无效的行动者：%s", action.Actor)
			}
			if !validActions[action.Action] {
				return fmt.Errorf("无效的行动：%s", action.Action)
			}
			// 有金额的动作金额必须为正；跟注/下注/加注/全下都要有金额
			if action.Action == models.ActionBet ||
				action.Action == models.ActionRaise ||
				action.Action == models.ActionAllin {
				if action.AmountBB == nil || *action.AmountBB <= 0 {
					return fmt.Errorf("%s 必须填写大于 0 的金额", action.Action)
				}
			}
			if action.AmountBB != nil && *action.AmountBB < 0 {
				return fmt.Errorf("金额不能为负数")
			}
		}
	}

	// 打了转牌却没录转牌的公共牌，说明录入漏了 —— 这种数据交给 AI 会得到错误分析，
	// 不如在入口拦下来
	if maxBoardNeeded > len(h.Board)/2 {
		return fmt.Errorf("已记录到 %s 街的行动，但公共牌只有 %d 张，至少需要 %d 张",
			streetNameForCount(maxBoardNeeded), len(h.Board)/2, maxBoardNeeded)
	}

	// --- 文本长度 ---
	// 用户自由文本会进提示词，限长既是成本控制也是提示词注入的第一道防线
	if len([]rune(h.HeroThought)) > 1000 {
		return fmt.Errorf("「我当时怎么想的」不能超过 1000 字")
	}
	if len([]rune(h.Title)) > 100 {
		return fmt.Errorf("标题不能超过 100 字")
	}

	// 空的手牌列表要落成空数组而不是 nil，否则 JSON 列会写成 null，前端还得判空
	if h.Streets == nil {
		h.Streets = []models.StreetRecord{}
	}
	if h.Villains == nil {
		h.Villains = []models.VillainInfo{}
	}
	if h.HeroTags == nil {
		h.HeroTags = []string{}
	}

	return nil
}

func streetNameForCount(cards int) string {
	switch cards {
	case 3:
		return "翻牌"
	case 4:
		return "转牌"
	case 5:
		return "河牌"
	default:
		return "翻前"
	}
}

// ComputeHandHash 计算手牌内容指纹。
//
// 刻意排除 Title 和 AnalyzeStatus：改标题不应该让已有的 AI 分析失效，
// 否则用户重命名一手牌就得重新花一次模型调用。
func ComputeHandHash(h *models.ReviewHand) string {
	payload := struct {
		HeroPosition string                `json:"heroPosition"`
		HeroCards    string                `json:"heroCards"`
		HeroStackBB  float64               `json:"heroStackBb"`
		Stakes       string                `json:"stakes"`
		Board        string                `json:"board"`
		VillainCount int                   `json:"villainCount"`
		Villains     []models.VillainInfo  `json:"villains"`
		PotType      string                `json:"potType"`
		Streets      []models.StreetRecord `json:"streets"`
		HeroThought  string                `json:"heroThought"`
		Result       string                `json:"result"`
		ResultAmount *float64              `json:"resultAmount"`
		HeroTags     []string              `json:"heroTags"`
	}{
		HeroPosition: h.HeroPosition,
		HeroCards:    h.HeroCards,
		HeroStackBB:  h.HeroStackBB,
		Stakes:       h.Stakes,
		Board:        h.Board,
		VillainCount: h.VillainCount,
		Villains:     h.Villains,
		PotType:      h.PotType,
		Streets:      h.Streets,
		HeroThought:  h.HeroThought,
		Result:       h.Result,
		ResultAmount: h.ResultAmount,
		HeroTags:     h.HeroTags,
	}

	// 结构体字段顺序固定，encoding/json 的输出是确定的，可以直接做哈希
	b, err := json.Marshal(payload)
	if err != nil {
		// 这里的类型都是可序列化的，理论上不会失败；真失败了给个空哈希，
		// 后果只是"内容没变也会重新分析一次"，不影响正确性
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
