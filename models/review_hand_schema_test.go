package models

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// GORM 的 serializer:json 标签写错不会编译报错，也不会在 AutoMigrate 时报错 ——
// 只会在真正写库时抛 unsupported type，或者更糟：把 Go 结构体原样塞进去。
// 这个测试把标签是否被正确识别钉死，避免以后改动标签时静默失效。
func TestReviewHandJSONFieldsHaveSerializer(t *testing.T) {
	parsed, err := schema.Parse(&ReviewHand{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 ReviewHand schema 失败: %v", err)
	}

	jsonFields := []string{"Streets", "Villains", "HeroTags"}
	for _, name := range jsonFields {
		field := parsed.LookUpField(name)
		if field == nil {
			t.Fatalf("字段 %s 不存在，可能是被重命名了", name)
		}
		if field.Serializer == nil {
			t.Errorf("字段 %s 缺少 serializer:json 标签，写库时会失败", name)
		}
		if field.DataType != "json" {
			t.Errorf("字段 %s 的列类型应为 json，实际为 %q", name, field.DataType)
		}
	}
}

// 检查索引与关键列名。列名一旦变化，JSON_CONTAINS 之类的原生 SQL 会静默查不到数据
func TestReviewHandColumnNames(t *testing.T) {
	parsed, err := schema.Parse(&ReviewHand{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 ReviewHand schema 失败: %v", err)
	}

	// 这些列名被 services 里的原生 SQL 片段引用，改了要同步改
	expectedColumns := map[string]string{
		"UserID":        "user_id",
		"GameID":        "game_id",
		"HeroPosition":  "hero_position",
		"HeroTags":      "hero_tags",
		"AnalyzeStatus": "analyze_status",
		"ContentHash":   "content_hash",
		"CreatedAt":     "created_at",
	}
	for fieldName, wantColumn := range expectedColumns {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Errorf("字段 %s 不存在", fieldName)
			continue
		}
		if field.DBName != wantColumn {
			t.Errorf("字段 %s 的列名应为 %q，实际为 %q", fieldName, wantColumn, field.DBName)
		}
	}
}

func TestReviewHandTableName(t *testing.T) {
	if got := (ReviewHand{}).TableName(); got != "review_hands" {
		t.Errorf("表名应为 review_hands，实际 %q", got)
	}
	if got := (ReviewLeakTag{}).TableName(); got != "review_leak_tags" {
		t.Errorf("表名应为 review_leak_tags，实际 %q", got)
	}
}

// 真正调用一次 GORM 的序列化器，确认 []StreetRecord / []string 能被转成 JSON 字符串。
//
// 只断言"标签被识别"还不够：如果序列化器在写库那一刻才失败（unsupported type），
// 编译和 AutoMigrate 都不会报错，只有用户保存手牌时才会炸。
func TestReviewHandJSONSerializerRoundTrip(t *testing.T) {
	parsed, err := schema.Parse(&ReviewHand{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 schema 失败: %v", err)
	}

	amount := 4.0
	streets := []StreetRecord{{
		Street: StreetFlop,
		Actions: []StreetAction{
			{Actor: ActorVillain, Action: ActionCheck},
			{Actor: ActorHero, Action: ActionBet, AmountBB: &amount},
		},
	}}
	villains := []VillainInfo{{Position: PositionBB, IsKey: true}}
	tags := []string{"3bet底池"}

	cases := []struct {
		fieldName string
		value     interface{}
		wantSub   string
	}{
		{"Streets", streets, `"street":"flop"`},
		{"Villains", villains, `"position":"BB"`},
		{"HeroTags", tags, `3bet底池`},
	}

	for _, tc := range cases {
		field := parsed.LookUpField(tc.fieldName)
		if field == nil || field.Serializer == nil {
			t.Fatalf("字段 %s 没有序列化器", tc.fieldName)
		}

		got, err := field.Serializer.Value(context.Background(), field, reflect.Value{}, tc.value)
		if err != nil {
			t.Errorf("%s 序列化失败: %v", tc.fieldName, err)
			continue
		}

		str, ok := got.(string)
		if !ok {
			t.Errorf("%s 序列化结果应是 string，实际 %T", tc.fieldName, got)
			continue
		}
		if !strings.Contains(str, tc.wantSub) {
			t.Errorf("%s 序列化结果应包含 %s，实际 %s", tc.fieldName, tc.wantSub, str)
		}
	}
}

// 空数组必须序列化成 []，不能是 null。
//
// hero_tags 的筛选走 JSON_CONTAINS，MySQL 对 JSON null 返回 NULL 而不是 false，
// 会让"按标签筛选"对这类记录静默失效。ValidateReviewHand 会兜底把 nil 换成空切片，
// 这里确认空切片确实产出 []。
func TestReviewHandEmptySlicesSerializeToEmptyArray(t *testing.T) {
	parsed, err := schema.Parse(&ReviewHand{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 schema 失败: %v", err)
	}

	field := parsed.LookUpField("HeroTags")
	if field == nil || field.Serializer == nil {
		t.Fatal("HeroTags 没有序列化器")
	}

	got, err := field.Serializer.Value(context.Background(), field, reflect.Value{}, []string{})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if got != "[]" {
		t.Errorf("空切片应序列化为 []，实际 %v（会导致 JSON_CONTAINS 筛选失效）", got)
	}
}

// 标签字典是"记忆"能聚合的前提，冷启动数据必须完整且不重复
func TestDefaultLeakTagsIntegrity(t *testing.T) {
	if len(DefaultLeakTags) < 25 {
		t.Errorf("冷启动标签过少（%d 个），不足以覆盖常见漏洞", len(DefaultLeakTags))
	}

	seenCode := make(map[string]bool, len(DefaultLeakTags))
	seenName := make(map[string]bool, len(DefaultLeakTags))
	validCategories := map[string]bool{
		TagCategoryPreflop:  true,
		TagCategoryPostflop: true,
		TagCategoryMental:   true,
		TagCategoryBankroll: true,
	}

	for _, tag := range DefaultLeakTags {
		if tag.Code == "" {
			t.Error("存在空的 code")
			continue
		}
		// code 是主键式的唯一标识，重复会导致 seedLeakTags 里先插入后更新，白跑一趟
		if seenCode[tag.Code] {
			t.Errorf("code 重复: %s", tag.Code)
		}
		seenCode[tag.Code] = true

		// name 重复意味着两个标签在用户画像里长得一样，聚合出来很难看
		if seenName[tag.Name] {
			t.Errorf("name 重复: %s", tag.Name)
		}
		seenName[tag.Name] = true

		if !validCategories[tag.Category] {
			t.Errorf("%s 的分类非法: %q", tag.Code, tag.Category)
		}
		if tag.Description == "" {
			t.Errorf("%s 缺少判定说明，模型选标签时会失去依据", tag.Code)
		}
		// code 会被写进提示词让模型引用，非 ASCII 容易在模型输出里被改写
		for _, r := range tag.Code {
			if r > 127 {
				t.Errorf("%s 的 code 含非 ASCII 字符，模型可能改写它", tag.Code)
				break
			}
		}
	}
}

// 四个分类都要有足够的标签。
//
// 刻意不去约束 code 的命名前缀：category 是独立列，前缀纯属冗余，
// 没有任何查询或聚合依赖它，强制统一只会让标签改名而无收益。
// 真正会出问题的是某个分类空空如也 —— 画像页会出现一片空白的分组，
// 用户看到"心态"分类下什么都没有，会以为数据丢了。
func TestDefaultLeakTagsCoverAllCategories(t *testing.T) {
	countByCategory := make(map[string]int, 4)

	for _, tag := range DefaultLeakTags {
		countByCategory[tag.Category]++
	}

	// 各类别的最低数量按经验设定：翻前/翻后是高频问题区，心态和资金管理相对少
	minByCategory := map[string]int{
		TagCategoryPreflop:  6,
		TagCategoryPostflop: 8,
		TagCategoryMental:   3,
		TagCategoryBankroll: 2,
	}

	for category, minCount := range minByCategory {
		got := countByCategory[category]
		if got < minCount {
			t.Errorf("分类 %s 只有 %d 个标签，至少需要 %d 个", category, got, minCount)
		}
	}

	if len(countByCategory) != len(minByCategory) {
		t.Errorf("出现了预期之外的分类，实际分类数 %d，预期 %d",
			len(countByCategory), len(minByCategory))
	}
}
