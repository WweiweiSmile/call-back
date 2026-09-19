package utils

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode"
)

const (
	// AIModelMaxLen 模型名长度上限，与 models.UserPreference.AIModel 的 size 对齐
	AIModelMaxLen = 100
	// APIKeyMaxLen API Key 长度上限。OpenAI 的 sk-proj- 长 Key 已超 150 字符，
	// 留足余量；前端输入框的 maxlength 必须与它一致，否则长 Key 会被静默截断
	APIKeyMaxLen = 512

	// chatCompletionsPath 客户端会自己拼上去的路径。用户若把它一起填进 BaseURL，
	// 拼起来就是 /chat/completions/chat/completions，报 404 而提示里看不出原因
	chatCompletionsPath = "/chat/completions"
)

// nonPublicPrefixes 标准判定之外还要拒的段。
//
// 都是「路由不到公网、但服务器可能真的能连上」的地址：CGNAT、
// 基准测试段（198.18/15 常用于透明代理，可能被指向内网）、保留段、
// 以及 NAT64 的 64:ff9b::/96（可用来把 IPv4 内网地址藏在 IPv6 里）
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
}

// IsPublicIP 这个地址是否属于「公网可达」。
//
// 只用于判断**字面量**与 Dial 时解析出的结果，不做 DNS 预解析 ——
// 域名在保存时的解析结果与真正拨号时的结果可以不同（DNS rebinding），
// 真正的边界在 services/ai_client.go 的 DialContext 守卫上
func IsPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	// IPv4-mapped IPv6（::ffff:127.0.0.1）必须先还原成 IPv4，
	// 否则 IsLoopback 之类的判定在 v6 形态上不生效，整条内网拦截会被绕过
	if ip.Is4In6() {
		ip = ip.Unmap()
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}

	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// ValidateAIBaseURL 校验并规范化用户填的模型接口地址。
//
// 返回规范化后的地址（scheme 与主机名小写、去掉尾部斜杠）。
//
// 拒绝内网地址是**安全边界**，不是为了防手滑：服务端会带着用户给的地址去发请求，
// 而 ai_client.go 会把对方响应体的前 200 字拼进错误信息、经分析记录的 error_msg
// 展示给用户 —— 不拦就是一条人人可用的内网探测通道。
//
// 代价是自建的本地模型（Ollama / LM Studio 之类）用不了，这是明确接受的取舍
func ValidateAIBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("BaseURL 不能为空")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("BaseURL 不是合法的地址")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("BaseURL 只支持 http/https 地址")
	}
	if parsed.Host == "" {
		return "", errors.New("BaseURL 缺少主机名")
	}
	// 凭据混在 URL 里会进日志与错误信息，直接拒
	if parsed.User != nil {
		return "", errors.New("BaseURL 里不要带用户名密码")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("BaseURL 不要带查询参数")
	}

	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(path), chatCompletionsPath) {
		return "", errors.New("请填到 /v1 这一层，不要带 /chat/completions")
	}

	host := strings.ToLower(parsed.Host)

	// 用标准库的 Hostname() 取主机名（去端口、去方括号）。
	// 它的返回值在畸形输入上不可靠 —— 见 checkHostname 里的说明，
	// 真正的把关在那一层的形状校验里
	hostname := strings.ToLower(parsed.Hostname())
	if err := checkHostname(hostname); err != nil {
		return "", err
	}

	return scheme + "://" + host + path, nil
}

// checkHostname 拦掉按名字就能认出来的内网目标。
// 入参是已经去端口、去方括号并转小写的主机名
func checkHostname(hostname string) error {
	if addr, err := netip.ParseAddr(hostname); err == nil {
		if !IsPublicIP(addr) {
			return errors.New("BaseURL 不能指向内网地址")
		}
		return nil
	}

	// 走到这里说明它不是合法的 IP 字面量。
	//
	// 必须先做形状校验，不能直接往下走后缀判断：不带方括号的 IPv6（"http://::1"）
	// 在 URL 里是畸形输入，而 url.Hostname() 对它的返回值**随模块的 go 指令版本而变**
	//（go 1.22 下返回 ":"，新语言版本下返回空串）。依赖这个差异会写出一段
	// "换个 go 版本就漏"的拦截逻辑，所以这里按字符集自己判死
	if err := validateHostnameShape(hostname); err != nil {
		return err
	}

	// 按名字判：localhost、以及 mDNS / 内网常用的保留后缀。
	// 这里判不出"某个域名解析到内网"，那种只能靠 Dial 层的实际解析结果
	name := strings.TrimSuffix(hostname, ".")
	if name == "localhost" || strings.HasSuffix(name, ".localhost") ||
		strings.HasSuffix(name, ".local") || strings.HasSuffix(name, ".internal") {
		return errors.New("BaseURL 不能指向内网地址")
	}
	return nil
}

// validateHostnameShape 主机名只能由字母、数字、连字符与点组成。
//
// 非 ASCII 的域名（IDN）会被拒 —— 模型接口用国际化域名的情形极少，
// 真有需要时用 punycode 形式填写即可。这个代价换的是"任何畸形输入都进不来"，
// 而畸形输入正是绕过内网判断的常见入口
func validateHostnameShape(hostname string) error {
	if hostname == "" || len(hostname) > 253 {
		return errors.New("BaseURL 的主机名不合法")
	}
	for _, r := range hostname {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return errors.New("BaseURL 的主机名不合法")
		}
	}
	return nil
}

// ValidateAIModel 校验模型名
func ValidateAIModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("模型名不能为空")
	}
	if len([]rune(model)) > AIModelMaxLen {
		return fmt.Errorf("模型名不能超过 %d 个字符", AIModelMaxLen)
	}
	// 模型名会拼进 JSON 请求体，控制字符只会让请求莫名其妙地失败
	for _, r := range model {
		if unicode.IsControl(r) {
			return errors.New("模型名不能包含控制字符")
		}
	}
	return nil
}

// ValidateAPIKey 校验用户填的 API Key。
//
// 调用前调用方应已 TrimSpace；这里也再 trim 一次，因为首尾空白
// （粘贴时最常见）会让鉴权失败而报错信息完全指不出原因
func ValidateAPIKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("API Key 不能为空")
	}
	if len(key) < 8 {
		return errors.New("API Key 太短，请检查是否复制完整")
	}
	if len(key) > APIKeyMaxLen {
		return fmt.Errorf("API Key 不能超过 %d 个字符", APIKeyMaxLen)
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return errors.New("API Key 不能包含空白或控制字符")
		}
	}
	return nil
}
