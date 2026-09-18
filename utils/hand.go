package utils

import (
	"call-go/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// 合法点数与花色
const (
	validRanks = "23456789TJQKA"
	validSuits = "shdc"
)

// positionsByTableSize 各人数下的合法位置，按翻前行动顺序（SB 先说话，BTN 最后）。
//
// 这份表和前端 src/utils/poker.ts 的 positionsForTableSize 是同一份契约，改动要两边同步。
// 规律：9 人桌去掉 UTG+2 就是 8 人，再去掉 UTG+1 就是 7 人，以此类推；
// 2 人桌的 SB 同时兼任 BTN（单挑时按钮位下小盲）。
var positionsByTableSize = map[int][]string{
	9: {
		models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionUTG1,
		models.PositionUTG2, models.PositionLJ, models.PositionHJ, models.PositionCO,
		models.PositionBTN,
	},
	8: {
		models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionUTG1,
		models.PositionLJ, models.PositionHJ, models.PositionCO, models.PositionBTN,
	},
	7: {
		models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionLJ,
		models.PositionHJ, models.PositionCO, models.PositionBTN,
	},
	6: {
		models.PositionSB, models.PositionBB, models.PositionUTG,
		models.PositionHJ, models.PositionCO, models.PositionBTN,
	},
	5: {
		models.PositionSB, models.PositionBB, models.PositionUTG,
		models.PositionCO, models.PositionBTN,
	},
	4: {
		models.PositionSB, models.PositionBB, models.PositionUTG, models.PositionBTN,
	},
	3: {models.PositionSB, models.PositionBB, models.PositionBTN},
	2: {models.PositionSB, models.PositionBB},
}

// PositionsForTableSize 返回该人数下的合法位置；人数越界时返回 nil
func PositionsForTableSize(tableSize int) []string {
	return positionsByTableSize[tableSize]
}

// isValidPosition 位置是否属于该人数下的合法集合
func isValidPosition(position string, tableSize int) bool {
	for _, p := range positionsByTableSize[tableSize] {
		if p == position {
			return true
		}
	}
	return false
}

var validActions = map[string]bool{
	models.ActionFold:  true,
	models.ActionCheck: true,
	models.ActionCall:  true,
	models.ActionBet:   true,
	models.ActionRaise: true,
	models.ActionAllin: true,
}

// sanitizeOpponentName 清洗对手名：去首尾空白、把连续空白折成一个空格、去掉控制字符。
// 名字会进提示词，换行与控制字符既能搅乱提示词排版，也是注入的载体
func sanitizeOpponentName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	return strings.Join(strings.Fields(cleaned), " ")
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

// ValidateBlinds 校验盲注与前注额度（BB）。
//
// 三个值全为 0 表示"这手牌没记录盲注"，是合法状态 —— 老数据与不想记盲注的用户
// 都走这条路径，底池估算会退回不含盲注的老口径。
//
// 手牌录入与用户默认设置共用这一份校验，免得两处出现不同的口径。
func ValidateBlinds(smallBlind, bigBlind, ante float64) error {
	if smallBlind < 0 || bigBlind < 0 || ante < 0 {
		return fmt.Errorf("盲注与前注不能为负数")
	}
	// 大盲是折算的基准，单独填小盲或前注没有意义
	if bigBlind == 0 && (smallBlind > 0 || ante > 0) {
		return fmt.Errorf("填了小盲或前注，就必须填大盲")
	}
	if bigBlind > 0 && smallBlind > bigBlind {
		return fmt.Errorf("小盲不能大于大盲")
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

	// --- 人数 / 位置 ---
	// 人数先归一化：0 表示没填，按满员桌处理（前端默认值也是 9）。
	// 位置的含义完全取决于人数——同样是 UTG，6 人桌和 9 人桌面对的范围
	// 差别很大——所以两者必须一起校验，不能各管各的。
	if h.TableSize == 0 {
		h.TableSize = models.DefaultTableSize
	}
	if h.TableSize < models.MinTableSize || h.TableSize > models.MaxTableSize {
		return fmt.Errorf("人数只能是 %d~%d 人，当前为 %d", models.MinTableSize, models.MaxTableSize, h.TableSize)
	}

	h.HeroPosition = strings.ToUpper(strings.TrimSpace(h.HeroPosition))
	if !isValidPosition(h.HeroPosition, h.TableSize) {
		return fmt.Errorf("无效的位置：%s（%d 人桌可选 %s）",
			h.HeroPosition, h.TableSize, strings.Join(PositionsForTableSize(h.TableSize), "/"))
	}

	// --- 对手 ---
	// 位置既要合法，也不能两个人坐同一个位置、更不能跟我撞位。
	// 空位置放行：M7.1 之前的老手牌允许只记对手数量不记位置
	seenPositions := make(map[string]bool, len(h.Villains))
	keyVillainCount := 0
	for i := range h.Villains {
		v := &h.Villains[i]
		v.Position = strings.ToUpper(strings.TrimSpace(v.Position))
		v.Name = sanitizeOpponentName(v.Name)
		// 对手表 id 由服务层按名字解析后写入，客户端传什么都不采信：
		// 采信就等于允许引用别人的对手
		v.OpponentID = 0

		if len([]rune(v.Name)) > models.OpponentNameMaxRunes {
			return fmt.Errorf("对手名字不能超过 %d 个字", models.OpponentNameMaxRunes)
		}
		// 名字必须坐在一个位置上，否则提示词里指不到人 —— "老王 加注"说不清是哪个老王。
		// 反过来不强制：老数据的对手没有名字，强制会让老手牌一编辑就报错
		if v.Name != "" && v.Position == "" {
			return fmt.Errorf("对手「%s」没有位置", v.Name)
		}
		if v.Position == "" {
			continue
		}
		if !isValidPosition(v.Position, h.TableSize) {
			return fmt.Errorf("对手位置无效：%s（%d 人桌可选 %s）",
				v.Position, h.TableSize, strings.Join(PositionsForTableSize(h.TableSize), "/"))
		}
		if v.Position == h.HeroPosition {
			return fmt.Errorf("对手位置不能和我相同：%s", v.Position)
		}
		if seenPositions[v.Position] {
			return fmt.Errorf("对手位置重复：%s", v.Position)
		}
		seenPositions[v.Position] = true

		if v.IsKey {
			keyVillainCount++
		}
	}
	// 关键对手是给提示词与画像用的"主要对手"，多过一个就失去意义了
	if keyVillainCount > 1 {
		return fmt.Errorf("关键对手只能有一个")
	}
	// 没填对手数量时按对手列表补上（脚本或第三方调用建的请求）。
	// 填了就不动：老手牌的"对手数量"是当时手填的，与列表长度不是一回事
	if h.VillainCount == 0 && len(h.Villains) > 0 {
		h.VillainCount = len(h.Villains)
	}
	// 我占一个位置，对手最多坐到剩下的位置
	if len(h.Villains) > h.TableSize-1 {
		return fmt.Errorf("%d 人桌最多记录 %d 个对手", h.TableSize, h.TableSize-1)
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

	// 盲注要放在人数归一化之后校验：前注的总额按人数折算，
	// 人数为 0 时折算出来是 0，会把"没记录"和"前注为 0"混成一回事
	if err := ValidateBlinds(h.SmallBlindBB, h.BigBlindBB, h.AnteBB); err != nil {
		return err
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
			// 行动者要么是老口径的聚合角色，要么是本手牌真的记录过的对手位置。
			// 拿一个本手牌里不存在的位置（比如 6 人桌写了 UTG+2、或者对手已被删掉）
			// 在这里就拦下，不至于存下一份指不到人的行动序列
			action.Actor = h.NormalizeActor(action.Actor)
			if !h.IsKnownActor(action.Actor) {
				return fmt.Errorf("无效的行动者：%s（本手牌没有这个位置）", action.Actor)
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
// 只放**真正会进提示词**的字段。刻意排除的每一项都有理由：
//   - Title / AnalyzeStatus：改标题不该让已有的 AI 分析失效，否则用户重命名
//     一手牌就得重新花一次模型调用
//   - HeroTags：标签只是用户自己的翻查线索，不进提示词，模型看不到它。
//     把它算进指纹的后果是"给手牌加个标签"就重置分析状态、清掉画像洞察，
//     还得白花一次额度重新分析
//
// 反过来，HeroThought、Streets 这些会直接影响结论的字段一个都不能漏。
func ComputeHandHash(h *models.ReviewHand) string {
	payload := struct {
		TableSize    int                   `json:"tableSize"`
		HeroPosition string                `json:"heroPosition"`
		HeroCards    string                `json:"heroCards"`
		HeroStackBB  float64               `json:"heroStackBb"`
		Stakes       string                `json:"stakes"`
		SmallBlindBB float64               `json:"smallBlindBb"`
		BigBlindBB   float64               `json:"bigBlindBb"`
		AnteBB       float64               `json:"anteBb"`
		Board        string                `json:"board"`
		VillainCount int                   `json:"villainCount"`
		Villains     []models.VillainInfo  `json:"villains"`
		PotType      string                `json:"potType"`
		Streets      []models.StreetRecord `json:"streets"`
		HeroThought  string                `json:"heroThought"`
		Result       string                `json:"result"`
		ResultAmount *float64              `json:"resultAmount"`
	}{
		TableSize:    h.TableSize,
		HeroPosition: h.HeroPosition,
		HeroCards:    h.HeroCards,
		HeroStackBB:  h.HeroStackBB,
		Stakes:       h.Stakes,
		SmallBlindBB: h.SmallBlindBB,
		BigBlindBB:   h.BigBlindBB,
		AnteBB:       h.AnteBB,
		Board:        h.Board,
		VillainCount: h.VillainCount,
		Villains:     h.Villains,
		PotType:      h.PotType,
		Streets:      h.Streets,
		HeroThought:  h.HeroThought,
		Result:       h.Result,
		ResultAmount: h.ResultAmount,
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
