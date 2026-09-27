package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// OpponentProfile 有四个 JSON 列，漏掉 serializer:json 的后果比 ReviewHand 那边更隐蔽：
// 写入路径用的是 Select + 结构体，GORM 会按字段类型推导 —— 没有 serializer 时
// 切片会被当成"多列的值"展开，运行时报 Error 1241，而编译和 AutoMigrate 都不报错。
//
// 这个测试把标签钉死，避免以后改字段时静默失效。
func TestOpponentProfileJSONFieldsHaveSerializer(t *testing.T) {
	parsed, err := schema.Parse(&OpponentProfile{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 OpponentProfile schema 失败: %v", err)
	}

	jsonFields := []string{"Tendencies", "Exploits", "Unknowns", "WatchNext"}
	for _, name := range jsonFields {
		field := parsed.LookUpField(name)
		if field == nil {
			t.Fatalf("字段 %s 不存在，可能是被重命名了", name)
		}
		if field.Serializer == nil {
			t.Errorf("字段 %s 缺少 serializer:json 标签，写入时会报 Operand should contain 1 column(s)", name)
		}
		if field.DataType != "json" {
			t.Errorf("字段 %s 的列类型应为 json，实际为 %q", name, field.DataType)
		}
	}
}

// 主键与列名被原生 SQL 片段引用，改了要同步改
func TestOpponentProfileColumnNames(t *testing.T) {
	parsed, err := schema.Parse(&OpponentProfile{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 OpponentProfile schema 失败: %v", err)
	}

	expected := map[string]string{
		"OpponentID":        "opponent_id",
		"UserID":            "user_id",
		"Status":            "status",
		"StartedAt":         "started_at",
		"LastGeneratedAt":   "last_generated_at",
		"HandsAtGeneration": "hands_at_generation",
	}
	for fieldName, wantColumn := range expected {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Errorf("字段 %s 不存在", fieldName)
			continue
		}
		if field.DBName != wantColumn {
			t.Errorf("字段 %s 的列名应为 %q，实际 %q", fieldName, wantColumn, field.DBName)
		}
	}
}
