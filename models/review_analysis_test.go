package models

import (
	"encoding/json"
	"testing"
)

func TestStrengthItemUnmarshal(t *testing.T) {
	var s StrengthItem
	if err := json.Unmarshal([]byte(`{"text":"翻前用 99 跟注 BTN 开池是合理的"}`), &s); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if s.Text != "翻前用 99 跟注 BTN 开池是合理的" {
		t.Errorf("text 未正确解析: %q", s.Text)
	}
}

// 空对象不能报错，也不能产生脏数据
func TestStrengthItemUnmarshalEmpty(t *testing.T) {
	var s StrengthItem
	if err := json.Unmarshal([]byte(`{}`), &s); err != nil {
		t.Fatalf("空对象不应报错: %v", err)
	}
	if s.Text != "" {
		t.Errorf("空对象应得到空文本，实际: %q", s.Text)
	}
}

// 序列化必须只输出 text，不能把内部结构漏进 API
func TestStrengthItemMarshalOnlyText(t *testing.T) {
	b, err := json.Marshal(StrengthItem{Text: "打得好"})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(b) != `{"text":"打得好"}` {
		t.Errorf("序列化结果应为 {\"text\":\"打得好\"}，实际 %s", b)
	}
}

// 优点不带标签是有意为之：标签字典是漏洞字典，
// 让模型拿漏洞标签描述优点会自相矛盾。这个测试把契约钉死
func TestStrengthItemHasNoTagFields(t *testing.T) {
	b, _ := json.Marshal(StrengthItem{Text: "x"})

	var fields map[string]interface{}
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(fields) != 1 {
		t.Errorf("优点应只有 text 一个字段，实际: %v", fields)
	}
	for _, forbidden := range []string{"tagCode", "severity", "evidence"} {
		if _, exists := fields[forbidden]; exists {
			t.Errorf("优点不应包含字段 %s", forbidden)
		}
	}
}
