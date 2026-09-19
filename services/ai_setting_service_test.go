package services

import (
	"call-go/config"
	"call-go/utils"
	"strings"
	"testing"
)

func TestResolveAIValues(t *testing.T) {
	preset := config.DefaultAIPreset()

	cases := []struct {
		name          string
		storedBaseURL string
		storedModel   string
		wantBaseURL   string
		wantModel     string
	}{
		{
			name:        "都没配过 → 全用预设",
			wantBaseURL: preset.BaseURL,
			wantModel:   preset.Model,
		},
		{
			name:          "只配了地址 → 模型用预设",
			storedBaseURL: "https://my-gateway.example.com/v1",
			wantBaseURL:   "https://my-gateway.example.com/v1",
			wantModel:     preset.Model,
		},
		{
			name:        "只配了模型 → 地址用预设",
			storedModel: "my-model",
			wantBaseURL: preset.BaseURL,
			wantModel:   "my-model",
		},
		{
			name:          "都配了 → 各用各的",
			storedBaseURL: "https://my-gateway.example.com/v1",
			storedModel:   "my-model",
			wantBaseURL:   "https://my-gateway.example.com/v1",
			wantModel:     "my-model",
		},
		{
			name:          "带首尾空白 → 去掉",
			storedBaseURL: "  https://my-gateway.example.com/v1  ",
			storedModel:   "  my-model  ",
			wantBaseURL:   "https://my-gateway.example.com/v1",
			wantModel:     "my-model",
		},
		{
			name:          "只有空白视同没配 → 用预设",
			storedBaseURL: "   ",
			storedModel:   "   ",
			wantBaseURL:   preset.BaseURL,
			wantModel:     preset.Model,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, model := resolveAIValues(tc.storedBaseURL, tc.storedModel)
			if baseURL != tc.wantBaseURL {
				t.Errorf("baseURL = %q, 期望 %q", baseURL, tc.wantBaseURL)
			}
			if model != tc.wantModel {
				t.Errorf("model = %q, 期望 %q", model, tc.wantModel)
			}
		})
	}
}

// TestDecideAPIKey 三态语义。
//
// "只改模型名不该碰 Key"就靠 nil 这一支，是本文件最关键的一条
func TestDecideAPIKey(t *testing.T) {
	withEncryptionKey(t)

	const storedCipher = "stored-cipher-value"
	const storedHint = "abcd"

	t.Run("没传 APIKey → 原样保留", func(t *testing.T) {
		cipher, hint, err := decideAPIKey(storedCipher, storedHint, nil)
		if err != nil {
			t.Fatal(err)
		}
		if cipher != storedCipher || hint != storedHint {
			t.Errorf("期望原样保留 %q/%q，实际 %q/%q", storedCipher, storedHint, cipher, hint)
		}
	})

	t.Run("传空串 → 清除", func(t *testing.T) {
		empty := ""
		cipher, hint, err := decideAPIKey(storedCipher, storedHint, &empty)
		if err != nil {
			t.Fatal(err)
		}
		if cipher != "" || hint != "" {
			t.Errorf("期望清除成空，实际 %q/%q", cipher, hint)
		}
	})

	t.Run("传纯空白 → 视同清除", func(t *testing.T) {
		blank := "   "
		cipher, hint, err := decideAPIKey(storedCipher, storedHint, &blank)
		if err != nil {
			t.Fatal(err)
		}
		if cipher != "" || hint != "" {
			t.Errorf("期望清除成空，实际 %q/%q", cipher, hint)
		}
	})

	t.Run("传新 Key → 加密替换且可取回", func(t *testing.T) {
		incoming := "  sk-1234567890abcdef  "
		cipher, hint, err := decideAPIKey(storedCipher, storedHint, &incoming)
		if err != nil {
			t.Fatal(err)
		}
		if cipher == "" || cipher == storedCipher {
			t.Error("密文没有被替换")
		}
		if strings.Contains(cipher, "sk-") {
			t.Errorf("密文里出现了明文片段: %s", cipher)
		}
		// 尾号取的是 trim 之后的末 4 位
		if hint != "cdef" {
			t.Errorf("hint = %q, 期望 %q", hint, "cdef")
		}

		decrypted, err := utils.DecryptSecret(cipher)
		if err != nil {
			t.Fatal(err)
		}
		if decrypted != "sk-1234567890abcdef" {
			t.Errorf("解密结果 = %q，期望 trim 之后的原串", decrypted)
		}
	})

	t.Run("传非法 Key → 报错且不动原值", func(t *testing.T) {
		invalid := "short"
		cipher, hint, err := decideAPIKey(storedCipher, storedHint, &invalid)
		if err == nil {
			t.Fatal("非法 Key 应当被拒")
		}
		if cipher != "" || hint != "" {
			t.Errorf("出错时不该返回任何可写入的值，实际 %q/%q", cipher, hint)
		}
	})

	t.Run("主密钥缺失 → 报错而不是把 Key 存成空", func(t *testing.T) {
		// 这里刻意不设置密钥。加密失败必须原样上报，
		// 绝不能降级成"返回空串"——那会把用户原有的 Key 一并清掉
		utils.SetPreferenceEncryptionKey("")
		t.Cleanup(func() { withEncryptionKey(t) })

		incoming := "sk-1234567890abcdef"
		if _, _, err := decideAPIKey(storedCipher, storedHint, &incoming); err == nil {
			t.Fatal("主密钥缺失时应当报错")
		}
	})
}

// withEncryptionKey 给测试装一把固定的加密主密钥
func withEncryptionKey(t *testing.T) {
	t.Helper()
	utils.SetPreferenceEncryptionKey("0123456789abcdef0123456789abcdef")
	t.Cleanup(func() { utils.SetPreferenceEncryptionKey("") })
}
