package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestTelemetryEventTableName 表名两个服务共用，是跨仓约定，别让命名策略推
func TestTelemetryEventTableName(t *testing.T) {
	if got := (TelemetryEvent{}).TableName(); got != "telemetry_events" {
		t.Errorf("表名应为 telemetry_events，实际 %q", got)
	}
}

// TestTelemetryEventExtraHasJSONSerializer
//
// serializer:json 写错不会编译报错，也不会在 AutoMigrate 时报错 —— 只会在真正
// 写库时抛 unsupported type，或者更糟：把 map 当成"多列的值"展开
func TestTelemetryEventExtraHasJSONSerializer(t *testing.T) {
	parsed, err := schema.Parse(&TelemetryEvent{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 TelemetryEvent schema 失败: %v", err)
	}

	field := parsed.LookUpField("Extra")
	if field == nil {
		t.Fatal("找不到 Extra 字段")
	}
	if field.Serializer == nil {
		t.Error("Extra 缺少 serializer:json 标签，写库时会失败")
	}
	if field.DataType != "json" {
		t.Errorf("Extra 的列类型应为 json，实际为 %q", field.DataType)
	}
}

// TestTelemetryEventColumnNames 列名被排查用的原生 SQL（JSON_EXTRACT 等）引用，
// 也出现在 analytics 脚本里，改了要同步改
func TestTelemetryEventColumnNames(t *testing.T) {
	parsed, err := schema.Parse(&TelemetryEvent{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 TelemetryEvent schema 失败: %v", err)
	}

	expected := map[string]string{
		"Event":     "event",
		"UserID":    "user_id",
		"IP":        "ip",
		"RemoteIP":  "remote_ip",
		"UserAgent": "user_agent",
		"Extra":     "extra",
		"CreatedAt": "created_at",
	}
	for fieldName, wantColumn := range expected {
		field := parsed.LookUpField(fieldName)
		if field == nil {
			t.Errorf("字段 %s 不存在，可能是被重命名了", fieldName)
			continue
		}
		if field.DBName != wantColumn {
			t.Errorf("字段 %s 的列名应为 %q，实际 %q", fieldName, wantColumn, field.DBName)
		}
	}
}
