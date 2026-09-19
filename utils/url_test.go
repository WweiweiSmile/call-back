package utils

import (
	"net/netip"
	"strings"
	"testing"
)

// TestValidateAIBaseURL 是本次改动的安全边界测试，内网地址那一段要穷举。
//
// 这一层拦不住"域名解析到内网"（那由 services/ai_client.go 的 DialContext 守卫负责），
// 但凡是能在字符串上认出来的内网目标都必须在这里被拒
func TestValidateAIBaseURL(t *testing.T) {
	valid := []struct {
		name string
		raw  string
		want string
	}{
		{"标准地址", "https://api.deepseek.com", "https://api.deepseek.com"},
		{"去掉尾部斜杠", "https://api.deepseek.com/", "https://api.deepseek.com"},
		{"带路径", "https://api.deepseek.com/v1", "https://api.deepseek.com/v1"},
		{"路径带尾部斜杠", "https://api.deepseek.com/v1/", "https://api.deepseek.com/v1"},
		{"大写 scheme 与主机名归一化", "HTTPS://API.DeepSeek.COM/v1", "https://api.deepseek.com/v1"},
		{"带端口", "http://example.com:8080/v1", "http://example.com:8080/v1"},
		{"智谱的 v4 路径", "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4"},
		{"首尾空白被去掉", "  https://api.deepseek.com/v1  ", "https://api.deepseek.com/v1"},
		{"FQDN 的尾点保留", "https://api.deepseek.com./v1", "https://api.deepseek.com./v1"},
	}

	for _, tc := range valid {
		t.Run("合法/"+tc.name, func(t *testing.T) {
			got, err := ValidateAIBaseURL(tc.raw)
			if err != nil {
				t.Fatalf("期望通过，却被拒: %v", err)
			}
			if got != tc.want {
				t.Errorf("归一化结果不对\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{"空串", ""},
		{"只有空白", "   "},

		// scheme
		{"无 scheme", "api.deepseek.com"},
		{"ftp", "ftp://api.deepseek.com"},
		{"file", "file:///etc/passwd"},
		{"javascript", "javascript:alert(1)"},

		// 形状
		{"带用户名密码", "http://user:pass@api.deepseek.com"},
		{"带查询参数", "https://api.deepseek.com/v1?key=x"},
		{"带锚点", "https://api.deepseek.com/v1#x"},
		{"已包含完整 endpoint", "https://api.deepseek.com/v1/chat/completions"},
		{"已包含完整 endpoint（尾部斜杠）", "https://api.deepseek.com/v1/chat/completions/"},
		{"缺少主机名", "https:///v1"},
		{"只有端口没有主机名", "http://:8080"},
		{"下划线主机名", "https://my_gateway.example.com/v1"},
		{"非 ASCII 主机名", "https://模型.example.com/v1"},

		// 内网：IPv4 字面量
		{"loopback", "http://127.0.0.1:8000"},
		{"loopback 其它地址", "http://127.1.2.3"},
		{"A 类私网", "http://10.0.0.5"},
		{"B 类私网", "http://172.16.1.1"},
		{"B 类私网上边界", "http://172.31.255.255"},
		{"C 类私网", "http://192.168.1.1"},
		{"云元数据地址", "http://169.254.169.254"},
		{"未指定地址", "http://0.0.0.0"},
		{"组播地址", "http://224.0.0.1"},
		{"CGNAT", "http://100.64.0.1"},
		{"基准测试段", "http://198.18.0.1"},
		{"保留段", "http://240.0.0.1"},

		// 内网：IPv6 字面量
		{"IPv6 loopback", "http://[::1]"},
		{"IPv6 链路本地", "http://[fe80::1]"},
		{"IPv6 唯一本地", "http://[fd00::1]"},
		{"IPv6 未指定（带方括号）", "http://[::]"},
		{"不带方括号的 IPv6", "http://::1"},
		{"IPv4-mapped loopback（不 Unmap 就会绕过）", "http://[::ffff:127.0.0.1]"},
		{"IPv4-mapped 私网", "http://[::ffff:10.0.0.1]"},
		{"NAT64 包裹的内网", "http://[64:ff9b::7f00:1]"},

		// 内网：保留域名后缀
		{"localhost", "http://localhost:11434"},
		{"localhost 大写", "http://LOCALHOST"},
		{"localhost 子域", "http://api.localhost"},
		{"mDNS 后缀", "http://mybox.local"},
		{"internal 后缀", "http://model.internal"},
	}

	for _, tc := range invalid {
		t.Run("拒绝/"+tc.name, func(t *testing.T) {
			got, err := ValidateAIBaseURL(tc.raw)
			if err == nil {
				t.Fatalf("期望被拒，却通过了（归一化结果 %q）", got)
			}
		})
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},

		{"127.0.0.1", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"192.168.0.1", false},
		{"169.254.1.1", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		{"100.64.0.1", false},
		{"192.0.0.1", false},
		{"198.18.0.1", false},
		{"198.19.255.255", false},
		{"240.0.0.1", false},
		{"::1", false},
		{"fe80::1", false},
		{"fd00::1", false},
		{"::", false},
		{"ff02::1", false},
		{"64:ff9b::7f00:1", false},
		// 4-in-6 必须还原成 v4 再判，否则这里会漏
		{"::ffff:127.0.0.1", false},
		{"::ffff:192.168.1.1", false},
	}

	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			addr, err := netip.ParseAddr(tc.addr)
			if err != nil {
				t.Fatalf("测试用例本身写错了: %v", err)
			}
			if got := IsPublicIP(addr); got != tc.want {
				t.Errorf("IsPublicIP(%s) = %v, 期望 %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestValidateAIModel(t *testing.T) {
	valid := []string{"deepseek-chat", "gpt-4o-mini", "moonshot-v1-8k", "glm-4-flash", "a"}
	for _, model := range valid {
		if err := ValidateAIModel(model); err != nil {
			t.Errorf("ValidateAIModel(%q) 期望通过，却报 %v", model, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"bad\nmodel",
		"bad\x00model",
	}
	for _, model := range invalid {
		if err := ValidateAIModel(model); err == nil {
			t.Errorf("ValidateAIModel(%q) 期望被拒，却通过了", model)
		}
	}

	// 长度上限：正好 100 通过，101 被拒（按 rune 数算，不是字节数）
	if err := ValidateAIModel(strings.Repeat("深", AIModelMaxLen)); err != nil {
		t.Errorf("正好 %d 个字符应当通过: %v", AIModelMaxLen, err)
	}
	if err := ValidateAIModel(strings.Repeat("深", AIModelMaxLen+1)); err == nil {
		t.Errorf("超过 %d 个字符应当被拒", AIModelMaxLen)
	}
}

func TestValidateAPIKey(t *testing.T) {
	valid := []string{
		"sk-1234567890",
		"sk-proj-abcdefghijklmnop",
		// 首尾空白由这里 trim 掉：粘贴带空白是最常见的"Key 明明对却鉴权失败"
		"  sk-1234567890  ",
	}
	for _, key := range valid {
		if err := ValidateAPIKey(key); err != nil {
			t.Errorf("ValidateAPIKey(%q) 期望通过，却报 %v", key, err)
		}
	}

	invalid := map[string]string{
		"":              "空串",
		"   ":           "只有空白",
		"sk-1234":       "太短",
		"sk-1234 5678":  "中间有空格",
		"sk-1234\n5678": "含换行",
		"sk-" + string(make([]byte, APIKeyMaxLen)): "超长",
	}
	for key, why := range invalid {
		if err := ValidateAPIKey(key); err == nil {
			t.Errorf("ValidateAPIKey(%s) 期望被拒，却通过了", why)
		}
	}
}
