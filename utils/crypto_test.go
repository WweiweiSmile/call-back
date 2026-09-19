package utils

import (
	"encoding/base64"
	"strings"
	"testing"
)

// testKey 32 字节的原始串。用它同时覆盖"原始 32 字节"这条解析分支
const testKey = "0123456789abcdef0123456789abcdef"

func withKey(t *testing.T, raw string) {
	t.Helper()
	original := preferenceEncryptionKey
	SetPreferenceEncryptionKey(raw)
	t.Cleanup(func() { preferenceEncryptionKey = original })
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	withKey(t, testKey)

	cases := []string{
		"sk-1234567890abcdef",
		"一个含中文的串",
		"a",
		strings.Repeat("x", 500),
	}

	for _, plain := range cases {
		encrypted, err := EncryptSecret(plain)
		if err != nil {
			t.Fatalf("加密 %q 失败: %v", plain, err)
		}
		if strings.Contains(encrypted, plain) && plain != "a" {
			t.Errorf("密文里不该出现明文片段: %s", encrypted)
		}

		decrypted, err := DecryptSecret(encrypted)
		if err != nil {
			t.Fatalf("解密失败: %v", err)
		}
		if decrypted != plain {
			t.Errorf("往返不一致\n got: %q\nwant: %q", decrypted, plain)
		}
	}
}

// TestEncryptSecretNonceRandom 同一把 Key 存两次必须得到不同密文。
// 否则从库里一眼就能看出两个用户用了同一把 Key
func TestEncryptSecretNonceRandom(t *testing.T) {
	withKey(t, testKey)

	const plain = "sk-1234567890abcdef"
	first, err := EncryptSecret(plain)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncryptSecret(plain)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Error("两次加密结果相同，nonce 没有随机")
	}
}

// TestDecryptSecretTampered GCM 自带认证，密文被改一个字节就必须失败
func TestDecryptSecretTampered(t *testing.T) {
	withKey(t, testKey)

	encrypted, err := EncryptSecret("sk-1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}

	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff

	if _, err := DecryptSecret(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Error("密文被篡改后仍能解密")
	}
}

// TestDecryptSecretWithRotatedKey 换主密钥后必须明确失败。
//
// 这是"主密钥丢失"时用户会撞上的路径：密文还在，但解不开了
func TestDecryptSecretWithRotatedKey(t *testing.T) {
	withKey(t, testKey)

	encrypted, err := EncryptSecret("sk-1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}

	withKey(t, "fedcba9876543210fedcba9876543210")

	if _, err := DecryptSecret(encrypted); err == nil {
		t.Error("换了主密钥仍能解密")
	}
}

// TestEncryptionUnavailable 主密钥缺失时必须报错，绝不能静默降级成明文，
// 也绝不能临时生成一把（那会让每次重启把已存的 Key 全变成垃圾）
func TestEncryptionUnavailable(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"未配置", ""},
		{"只有空白", "   "},
		{"长度不对", "too-short"},
		{"base64 解出来不是 32 字节", base64.StdEncoding.EncodeToString([]byte("only-16-bytes!!!"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withKey(t, tc.raw)

			if EncryptionReady() {
				t.Fatal("密钥不合法时 EncryptionReady 必须为 false")
			}
			if _, err := EncryptSecret("sk-1234567890"); err != ErrEncryptionUnavailable {
				t.Errorf("期望返回 ErrEncryptionUnavailable，实际: %v", err)
			}
			if _, err := DecryptSecret("whatever"); err == nil {
				t.Error("密钥不合法时解密应当报错")
			}
		})
	}
}

// TestSetPreferenceEncryptionKeyAcceptsBase64 覆盖 base64 那条解析分支
func TestSetPreferenceEncryptionKeyAcceptsBase64(t *testing.T) {
	withKey(t, base64.StdEncoding.EncodeToString([]byte(testKey)))

	if !EncryptionReady() {
		t.Fatal("base64 编码的 32 字节密钥应当被接受")
	}

	encrypted, err := EncryptSecret("sk-1234567890")
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "sk-1234567890" {
		t.Errorf("往返不一致: %q", decrypted)
	}
}

func TestSecretHint(t *testing.T) {
	cases := []struct {
		plain string
		want  string
	}{
		{"sk-1234567890", "7890"},
		{"12345678", "5678"},
		// 太短的串一律不给掩码：露末 4 位等于把大部分内容送出去
		{"1234567", ""},
		{"abc", ""},
		{"", ""},
	}

	for _, tc := range cases {
		if got := SecretHint(tc.plain); got != tc.want {
			t.Errorf("SecretHint(%q) = %q, 期望 %q", tc.plain, got, tc.want)
		}
	}
}
