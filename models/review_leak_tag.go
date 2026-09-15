package models

import "time"

// 标签分类
const (
	TagCategoryPreflop  = "preflop"
	TagCategoryPostflop = "postflop"
	TagCategoryMental   = "mental"
	TagCategoryBankroll = "bankroll"
)

// ReviewLeakTag 漏洞标签字典
//
// 为什么必须有这张表：如果让模型自由生成标签，第二次分析会产出"翻前跟注过宽"和
// "翻前跟注范围太松"两个字符串 —— 语义相同、字符串不同，永远聚不了合，用户画像
// 会退化成一堆同义标签。受控词表是"长期记忆"能成立的前提。
//
// AI 分析时只能从这个表里选 Code，认为需要新标签时走 suggestedTag 待人工审核。
type ReviewLeakTag struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"size:64;uniqueIndex;not null;comment:唯一标识，AI 引用它"`
	Name        string    `json:"name" gorm:"size:64;not null;comment:中文名"`
	Category    string    `json:"category" gorm:"size:20;index;comment:preflop/postflop/mental/bankroll"`
	Description string    `json:"description" gorm:"size:255;comment:判定说明，会写进提示词帮模型选对标签"`
	IsActive    bool      `json:"isActive" gorm:"not null;default:true;comment:是否启用"`
	SortOrder   int       `json:"sortOrder" gorm:"default:0;comment:排序"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (ReviewLeakTag) TableName() string {
	return "review_leak_tags"
}

// DefaultLeakTags 冷启动的标签字典。
// 服务启动时按 Code upsert，所以往这里加标签，重启即生效，不需要手工改库。
var DefaultLeakTags = []ReviewLeakTag{
	// ---------- 翻前 ----------
	{Code: "preflop_too_loose", Name: "翻前跟注过宽", Category: TagCategoryPreflop, SortOrder: 10,
		Description: "在不利位置或面对加注时，用本该弃牌的边缘牌跟注入池"},
	{Code: "preflop_too_tight", Name: "翻前弃牌过紧", Category: TagCategoryPreflop, SortOrder: 20,
		Description: "在有位置或赔率合适时，弃掉了本可以继续的牌"},
	{Code: "preflop_limp_too_much", Name: "翻前过度平跟", Category: TagCategoryPreflop, SortOrder: 30,
		Description: "该加注开池却选择平跟，放弃主动权和弃牌率"},
	{Code: "preflop_open_too_wide", Name: "翻前开池过宽", Category: TagCategoryPreflop, SortOrder: 40,
		Description: "在前位或面对紧手时，开池范围超出合理区间"},
	{Code: "preflop_3bet_missing", Name: "该 3bet 时只跟注", Category: TagCategoryPreflop, SortOrder: 50,
		Description: "拿到强牌或有位置优势时错失 3bet，把主动权让出去"},
	{Code: "preflop_3bet_too_loose", Name: "3bet 过宽", Category: TagCategoryPreflop, SortOrder: 60,
		Description: "用不足以承受 4bet 的牌做 3bet 诈唬，且缺少阻断牌"},
	{Code: "preflop_bb_defend_loose", Name: "大盲防守过松", Category: TagCategoryPreflop, SortOrder: 70,
		Description: "大盲面对加注时防守范围过宽，忽视了位置劣势和实现权益能力"},
	{Code: "preflop_facing_3bet_loose", Name: "面对 3bet 跟注过宽", Category: TagCategoryPreflop, SortOrder: 80,
		Description: "面对 3bet 用被压制的牌跟注，未考虑对手范围收紧"},
	{Code: "preflop_short_stack_wrong", Name: "短筹码策略错误", Category: TagCategoryPreflop, SortOrder: 90,
		Description: "筹码少于 40BB 时仍用深筹码思路打牌，未切换到推/弃策略"},

	// ---------- 翻后 ----------
	{Code: "cbet_missing", Name: "翻牌持续下注不足", Category: TagCategoryPostflop, SortOrder: 100,
		Description: "作为翻前加注者在有利牌面该持续下注却选择过牌，错失弃牌率"},
	{Code: "cbet_too_much", Name: "翻牌持续下注过频", Category: TagCategoryPostflop, SortOrder: 110,
		Description: "在与自己范围无关的牌面仍高频持续下注，缺乏范围优势"},
	{Code: "multiway_cbet_wrong", Name: "多人池持续下注不当", Category: TagCategoryPostflop, SortOrder: 120,
		Description: "多人底池仍套用单挑的持续下注频率，忽视被反超概率"},
	{Code: "no_flop_plan", Name: "翻牌缺少后续街计划", Category: TagCategoryPostflop, SortOrder: 130,
		Description: "翻牌行动时没有想好转牌/河牌的计划，导致后续被动"},
	{Code: "size_not_purpose", Name: "下注尺度与目的不符", Category: TagCategoryPostflop, SortOrder: 140,
		Description: "价值下注用太小尺度、诈唬用太大尺度，尺度与意图不匹配"},
	{Code: "draw_odds_wrong", Name: "听牌赔率计算错误", Category: TagCategoryPostflop, SortOrder: 150,
		Description: "跟注听牌时未正确计算底池赔率、隐含赔率或补牌数"},
	{Code: "turn_over_fold", Name: "转牌过度弃牌", Category: TagCategoryPostflop, SortOrder: 160,
		Description: "转牌面对一条街的下注就弃掉仍有胜率的牌，弃牌率过高"},
	{Code: "river_over_fold", Name: "河牌过度弃牌", Category: TagCategoryPostflop, SortOrder: 170,
		Description: "河牌面对诈唬频率合理的对手弃牌过多，被剥削"},
	{Code: "call_down_too_light", Name: "抓诈唬过松", Category: TagCategoryPostflop, SortOrder: 180,
		Description: "用只能赢诈唬的牌连续跟注到底，忽视对手价值范围"},
	{Code: "river_thin_value_missing", Name: "河牌薄价值缺失", Category: TagCategoryPostflop, SortOrder: 190,
		Description: "河牌有中等强度成牌却选择过牌，错失被更差牌跟注的价值"},
	{Code: "miss_value_bet", Name: "成牌未做价值下注", Category: TagCategoryPostflop, SortOrder: 200,
		Description: "拿到强牌却主动过牌或跟注，未把底池做大"},
	{Code: "river_bluff_too_much", Name: "河牌诈唬过频", Category: TagCategoryPostflop, SortOrder: 210,
		Description: "河牌诈唬缺少阻断牌或故事不连贯，频率超出合理范围"},
	{Code: "overplay_top_pair", Name: "顶对过度游戏", Category: TagCategoryPostflop, SortOrder: 220,
		Description: "把顶对当坚果打，在遭遇反抗时仍持续做大底池"},
	{Code: "pot_control_wrong", Name: "过度控池", Category: TagCategoryPostflop, SortOrder: 230,
		Description: "该建立底池时反复过牌控池，长期损失价值"},
	{Code: "slowplay_wrong", Name: "慢打过当", Category: TagCategoryPostflop, SortOrder: 240,
		Description: "在湿润牌面或多人池慢打强牌，给了对手免费反超的机会"},

	// ---------- 心态 ----------
	{Code: "result_oriented", Name: "结果导向思维", Category: TagCategoryMental, SortOrder: 300,
		Description: "以单手牌输赢而非决策质量来评价打法"},
	{Code: "tilt_call", Name: "情绪化跟注", Category: TagCategoryMental, SortOrder: 310,
		Description: "因上风/下风情绪影响，做出无逻辑的跟注或加注"},
	{Code: "fear_of_loss", Name: "怕输导致动作变形", Category: TagCategoryMental, SortOrder: 320,
		Description: "因害怕输掉已有筹码而放弃本该执行的进攻线路"},
	{Code: "scared_money", Name: "恐惧金钱", Category: TagCategoryMental, SortOrder: 330,
		Description: "因筹码对应的金额超出心理承受范围而打不出正确决策"},

	// ---------- 资金管理 ----------
	{Code: "bankroll_too_high", Name: "级别超出资金承受能力", Category: TagCategoryBankroll, SortOrder: 400,
		Description: "参与的盲注级别相对总资金过高，影响决策质量"},
	{Code: "no_stop_loss", Name: "缺乏止损纪律", Category: TagCategoryBankroll, SortOrder: 410,
		Description: "下风期未按预设止损线离场，导致亏损扩大"},
}
