package config

import "testing"

// withAppConfig 临时替换全局配置，测试结束还原
func withAppConfig(t *testing.T, cfg *Config) {
	t.Helper()
	original := AppConfig
	AppConfig = cfg
	t.Cleanup(func() { AppConfig = original })
}

// TestAIConfigNilSafe AppConfig 尚未初始化时必须安全返回默认值。
//
// 这是 routes_smoke_test 能在不 LoadConfig 的进程里跑通的前提：
// 那里会构造控制器，而控制器链路上会用到 AIConfig
func TestAIConfigNilSafe(t *testing.T) {
	withAppConfig(t, nil)

	got := AIConfig()
	if got.TimeoutSec != defaultAITimeoutSec {
		t.Errorf("TimeoutSec = %d, 期望 %d", got.TimeoutSec, defaultAITimeoutSec)
	}
	if got.DailyLimit != defaultAIDailyLimit {
		t.Errorf("DailyLimit = %d, 期望 %d", got.DailyLimit, defaultAIDailyLimit)
	}
}

// TestAIConfigIgnoresNonPositive 非法（<=0）的值要退回默认，
// 免得一个手滑的 0 让每次调用都立刻超时
func TestAIConfigIgnoresNonPositive(t *testing.T) {
	withAppConfig(t, &Config{AITimeoutSec: 0, AIDailyLimit: -1})

	got := AIConfig()
	if got.TimeoutSec != defaultAITimeoutSec {
		t.Errorf("TimeoutSec = %d, 期望退回默认 %d", got.TimeoutSec, defaultAITimeoutSec)
	}
	if got.DailyLimit != defaultAIDailyLimit {
		t.Errorf("DailyLimit = %d, 期望退回默认 %d", got.DailyLimit, defaultAIDailyLimit)
	}
}

func TestAIConfigReadsAppConfig(t *testing.T) {
	withAppConfig(t, &Config{AITimeoutSec: 60, AIDailyLimit: 5})

	got := AIConfig()
	if got.TimeoutSec != 60 || got.DailyLimit != 5 {
		t.Errorf("AIConfig() = %+v, 期望 {60 5}", got)
	}
}

func TestDefaultAIPresetNilSafe(t *testing.T) {
	withAppConfig(t, nil)

	preset := DefaultAIPreset()
	if preset.BaseURL != DefaultAIBaseURL {
		t.Errorf("BaseURL = %q, 期望 %q", preset.BaseURL, DefaultAIBaseURL)
	}
	if preset.Model != DefaultAIModel {
		t.Errorf("Model = %q, 期望 %q", preset.Model, DefaultAIModel)
	}
	if preset.Key != AIPresetDeepSeek {
		t.Errorf("Key = %q, 期望 %q", preset.Key, AIPresetDeepSeek)
	}
}

// TestDefaultAIPresetFollowsEnv 部署方改 DEEPSEEK_BASE_URL / DEEPSEEK_MODEL
// 就能改「没配过的用户打开模型设置时的预填值」，不必改代码
func TestDefaultAIPresetFollowsEnv(t *testing.T) {
	withAppConfig(t, &Config{
		DeepSeekBaseURL: "https://my-gateway.example.com/v1",
		DeepSeekModel:   "my-model",
	})

	preset := DefaultAIPreset()
	if preset.BaseURL != "https://my-gateway.example.com/v1" {
		t.Errorf("BaseURL = %q, 配置没有生效", preset.BaseURL)
	}
	if preset.Model != "my-model" {
		t.Errorf("Model = %q, 配置没有生效", preset.Model)
	}
}

func TestAIPresets(t *testing.T) {
	withAppConfig(t, nil)

	presets := AIPresets()
	if len(presets) != 5 {
		t.Fatalf("预设数量 = %d, 期望 5（4 家供应商 + 自定义）", len(presets))
	}

	seen := make(map[string]bool, len(presets))
	for _, preset := range presets {
		if preset.Key == "" {
			t.Error("预设的 Key 不能为空，前端用它做 chip 的选中态")
		}
		if preset.Name == "" {
			t.Errorf("预设 %s 缺少展示名", preset.Key)
		}
		if seen[preset.Key] {
			t.Errorf("预设 Key 重复: %s", preset.Key)
		}
		seen[preset.Key] = true
	}

	// 自定义必须排在最后，且两项留空：前端选中它时不得把空值灌进输入框
	custom := presets[len(presets)-1]
	if custom.Key != AIPresetCustom {
		t.Errorf("最后一项应当是自定义，实际是 %s", custom.Key)
	}
	if custom.BaseURL != "" || custom.Model != "" {
		t.Errorf("自定义预设的 BaseURL / Model 必须留空，实际 %q / %q", custom.BaseURL, custom.Model)
	}

	// 其余每家都必须给得出可用的预填值，否则"一键填充"是假的
	for _, preset := range presets[:len(presets)-1] {
		if preset.BaseURL == "" || preset.Model == "" {
			t.Errorf("预设 %s 缺少 BaseURL 或 Model", preset.Key)
		}
	}
}
