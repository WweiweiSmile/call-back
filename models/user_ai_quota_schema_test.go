package models

import (
	"call-go/config"
	"strconv"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestUserAIQuotaTableName 表名是指定的，别让 GORM 的命名策略推出来一个别的
func TestUserAIQuotaTableName(t *testing.T) {
	if got := (UserAIQuota{}).TableName(); got != "user_ai_quotas" {
		t.Errorf("表名应为 user_ai_quotas，实际 %q", got)
	}
}

// TestUserAIQuotaPointsDefaultMatchesConfig points 的 default 标签是**载荷**：
// AutoMigrate 建列时用的就是它。它必须与 config.DefaultAIPointLimit 一致 ——
// 否则两个来源会给出不同的初始点数（库里是 default 值，代码兜底是常量）
func TestUserAIQuotaPointsDefaultMatchesConfig(t *testing.T) {
	parsed, err := schema.Parse(&UserAIQuota{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 UserAIQuota schema 失败: %v", err)
	}

	field := parsed.LookUpField("Points")
	if field == nil {
		t.Fatal("找不到 Points 字段")
	}

	want := strconv.Itoa(config.DefaultAIPointLimit)
	if field.DefaultValue != want {
		t.Errorf("Points 的 default 标签是 %q，应与 config.DefaultAIPointLimit (%s) 一致",
			field.DefaultValue, want)
	}
	if config.DefaultAIPointLimit != 9 {
		t.Errorf("DefaultAIPointLimit = %d，口径是「9 点 = 3 次复盘」，改它要同步改提示文案",
			config.DefaultAIPointLimit)
	}
}

// TestUserAIQuotaUserIDUnique 每个用户只该有一行 —— 余额是单值，
// 有第二行就意味着两个来源各扣各的
func TestUserAIQuotaUserIDUnique(t *testing.T) {
	parsed, err := schema.Parse(&UserAIQuota{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("解析 UserAIQuota schema 失败: %v", err)
	}

	userID := parsed.LookUpField("UserID")
	if userID == nil {
		t.Fatal("找不到 UserID 字段")
	}

	// `uniqueIndex` 标签不落在 Field.Unique 上，而是进 ParseIndexes()。
	// 余额是单值，多一行就意味着两个来源各扣各的，所以这条必须钉死
	for _, idx := range parsed.ParseIndexes() {
		if idx.Class != "UNIQUE" {
			continue
		}
		for _, opt := range idx.Fields {
			if opt.Field == userID {
				return
			}
		}
	}
	t.Error("UserID 必须有唯一索引（每个用户一条余额行）")
}
