package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// ScoreRequest.ChipCounts 是 json 列，漏掉 serializer:json 的后果和 ReviewHand 那边一样：
// 编译与 AutoMigrate 都不报错，写申请时才炸。这个测试把标签钉死。
func TestScoreRequestChipCountsHasJSONSerializer(t *testing.T) {
	parsed, err := schema.Parse(&ScoreRequest{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 ScoreRequest schema 失败: %v", err)
	}

	field := parsed.LookUpField("ChipCounts")
	if field == nil {
		t.Fatal("字段 ChipCounts 不存在，可能是被重命名了")
	}
	if field.Serializer == nil {
		t.Error("字段 ChipCounts 缺少 serializer:json 标签，写入时会报 Operand should contain 1 column(s)")
	}
	if field.DataType != "json" {
		t.Errorf("字段 ChipCounts 的列类型应为 json，实际为 %q", field.DataType)
	}
	if got := field.DBName; got != "chip_counts" {
		t.Errorf("字段 ChipCounts 的列名应为 chip_counts，实际为 %q", got)
	}
}
