package services

import (
	"call-go/models"
	"testing"
)

// TestNewPreferenceDefaults 钉住这一行的形状。
//
// 这是本次改动里最值得测的一条：PreferenceService.Get 在"没有行"时返回默认值，
// 但**有行**时是直接从行里读的。于是建行时必须带上盲注默认值 ——
// 一旦 AISettingService 建出一行 0/0/0，"只配了模型、没碰过盲注"的用户
// 拿到的默认盲注就从 0.5/1/0 变成 0/0/0，录入页预填会静默失效
func TestNewPreferenceDefaults(t *testing.T) {
	const userID = 7
	got := newPreferenceDefaults(userID)

	if got.UserID != userID {
		t.Errorf("UserID = %d, 期望 %d", got.UserID, userID)
	}

	// 三个盲注默认值必须来自 models 包的常量，不能是别的什么数
	if got.SmallBlindBB != models.DefaultSmallBlindBB {
		t.Errorf("SmallBlindBB = %v, 期望 %v", got.SmallBlindBB, models.DefaultSmallBlindBB)
	}
	if got.BigBlindBB != models.DefaultBigBlindBB {
		t.Errorf("BigBlindBB = %v, 期望 %v", got.BigBlindBB, models.DefaultBigBlindBB)
	}
	if got.AnteBB != models.DefaultAnteBB {
		t.Errorf("AnteBB = %v, 期望 %v", got.AnteBB, models.DefaultAnteBB)
	}

	// AI 四列必须为空：这一行代表"用户从没配过任何东西"
	if got.AIBaseURL != "" {
		t.Errorf("AIBaseURL = %q, 期望空串", got.AIBaseURL)
	}
	if got.AIModel != "" {
		t.Errorf("AIModel = %q, 期望空串", got.AIModel)
	}
	if got.AIAPIKeyEncrypted != "" {
		t.Errorf("AIAPIKeyEncrypted = %q, 期望空串", got.AIAPIKeyEncrypted)
	}
	if got.AIAPIKeyHint != "" {
		t.Errorf("AIAPIKeyHint = %q, 期望空串", got.AIAPIKeyHint)
	}

	// 未落库：ID 必须是零值，否则调用方会走 Save 而不是 Create
	if got.ID != 0 {
		t.Errorf("ID = %d, 期望 0（尚未落库）", got.ID)
	}
}

// TestNewPreferenceDefaultsNotShared 每次都要给新对象。
// 共享同一个指针会让一次改动污染后续所有调用
func TestNewPreferenceDefaultsNotShared(t *testing.T) {
	first := newPreferenceDefaults(1)
	second := newPreferenceDefaults(2)

	if first == second {
		t.Fatal("两次调用返回了同一个指针")
	}
	if first.UserID == second.UserID {
		t.Error("两次调用返回了同一份 UserID，说明对象被复用了")
	}
}
