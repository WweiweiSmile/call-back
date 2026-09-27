package models

import (
	"encoding/json"
	"testing"
)

// VillainInfo 的序列化形态是**存量数据的兼容契约**，不能随手改。
//
// villains 是一个 JSON 列，整块读整块写。往里加字段时如果漏了 omitempty，
// 存量手牌重新保存后就会多出一个 "cards":""，ComputeHandHash 便认为内容变了 ——
// 后果是全库已完成的分析被判失效、画像洞察被清、还要白花一次模型额度重算。
//
// 这里直接把序列化结果钉成字面量，而不是"断言某个 key 不存在"：
// 后者只能挡住本次这一个字段，前者在以后任何人改动 tag 时都会红。
func TestVillainInfoJSONShapeUnchangedForLegacy(t *testing.T) {
	// 老数据只有位置。这一行必须与 M7.1 时的输出逐字一致
	legacy := VillainInfo{Position: "CO"}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if got, want := string(b), `{"position":"CO"}`; got != want {
		t.Errorf("老对手的序列化结果必须一字不变\n实际: %s\n期望: %s", got, want)
	}

	// 没记对手底牌时不应出现 cards 键 —— "没看到"与"看到了但为空"要长得一样，
	// 否则录入页多打开一次编辑弹窗再保存，就会把存量手牌全部标脏
	stack := 100.0
	named := VillainInfo{Position: "CO", Name: "老王", StackBB: &stack, IsKey: true}
	b, err = json.Marshal(named)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if got, want := string(b), `{"position":"CO","stackBb":100,"isKey":true,"name":"老王"}`; got != want {
		t.Errorf("没记底牌的对手序列化结果不符\n实际: %s\n期望: %s", got, want)
	}

	// 记了底牌才出现 cards
	withCards := VillainInfo{Position: "CO", Cards: "AsKh"}
	b, err = json.Marshal(withCards)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if got, want := string(b), `{"position":"CO","cards":"AsKh"}`; got != want {
		t.Errorf("记了底牌的对手序列化结果不符\n实际: %s\n期望: %s", got, want)
	}
}
