package services

import (
	"bytes"
	"call-go/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// ChatMessage 一条对话消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionResult 一次模型调用的结果
type CompletionResult struct {
	Content    string
	TokensIn   int
	TokensOut  int
	DurationMs int64
}

// AICallSettings 一次模型调用所需的凭据与模型名。
//
// 由调用方按 userID 解析好传进来（services.AISettingService.ResolveCallSettings），
// 客户端保持无状态：不碰数据库、不读全局配置。这样它既是纯粹的 HTTP 包装，
// 也顺手让"每个用户用自己的 Key"这件事只在一个地方决定
//
// 刻意不含 TimeoutSec —— 那是服务端参数（config.AIConfig），
// 假装它按用户而变只会误导
type AICallSettings struct {
	APIKey  string
	BaseURL string
	Model   string
}

// AIClient 大模型客户端。
//
// 走 OpenAI 兼容的 /chat/completions 协议，所以换供应商只需要改 baseURL 和 model，
// 不用改代码。刻意不引 SDK：go.mod 目前零 AI 依赖，保持轻。
type AIClient struct {
	http *http.Client
}

// aiDialer 拨号器。带超时，避免连接层无限挂起
var aiDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

// NewAIClient 构造客户端。
//
// 刻意不在构造函数里读配置：那样在 LoadConfig 之前构造（例如只注册路由的单测）
// 会 nil 解引用崩溃。凭据现在由每次调用的入参给出
func NewAIClient() *AIClient {
	// 在 DefaultTransport 的副本上改：保留 TLS 握手超时、HTTP/2 等默认值，
	// 只把拨号这一环换成带内网守卫的版本
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = guardedDialContext

	return &AIClient{
		http: &http.Client{
			// 单次请求的真实超时由 context 控制（调用方按 AITimeoutSec 设置），
			// 这里给一个宽松上限兜底
			Timeout: 5 * time.Minute,
			// 不跟随重定向：一个公网地址 302 到 http://169.254.169.254/ 就绕过了
			// 保存时的校验。默认的 CheckRedirect 会再走一次 DialContext 从而被拦下，
			// 但直接不跟随更简单也更严
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: transport,
		},
	}
}

// guardedDialContext 在真正建立连接前拦下指向内网的地址。
//
// 这是 SSRF 的最后一道闸，也是唯一有效的一道：保存时的校验能拦住内网字面量与
// localhost 这类名字，但拦不住"域名解析到内网"（DNS rebinding）——
// 校验发生时和拨号发生时，解析结果可以不同。
//
// 做法：先解析出全部 IP，**每一个**都必须是公网地址（不挑"第一个合格的"，
// 否则只要让域名同时返回一个公网 IP 和一个内网 IP 就能绕过），然后用**原始的
// addr** 去拨号 —— 不换成解析出的 IP，是为了不破坏 https 的 SNI（ServerName
// 由 http.Transport 从 URL 的 host 推导，而 transport 是跨用户共享的，
// 无法为每个用户预设）。
//
// 代价：解析校验与拨号之间留了一个极窄的 TOCTOU 窗口。利用它需要控制 DNS
// 并在两次解析之间翻转记录。这里接受这个窗口，因为真正要保护的是"服务器能不能
// 摸到内网"，而请求体是用户自己的手牌数据、凭据也是用户自己的 Key，
// 攻击者拿到的东西价值远低于传统 SSRF 场景
func guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("无法解析目标地址: %w", err)
	}

	// IP 字面量直接判，不必走 DNS
	if ip, err := netip.ParseAddr(host); err == nil {
		if !utils.IsPublicIP(ip) {
			return nil, errors.New("拒绝连接内网地址")
		}
		return aiDialer.DialContext(ctx, network, addr)
	}

	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("解析主机名失败: %w", err)
	}
	if len(resolved) == 0 {
		return nil, errors.New("主机名没有解析到任何地址")
	}
	for _, item := range resolved {
		ip, ok := netip.AddrFromSlice(item.IP)
		if !ok || !utils.IsPublicIP(ip) {
			return nil, errors.New("拒绝连接内网地址")
		}
	}

	return aiDialer.DialContext(ctx, network, addr)
}

// chatRequest OpenAI 兼容的请求体
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// CompleteJSON 调用模型并强制返回 JSON。
//
// 请求里带 response_format=json_object，配合提示词中给出的 Schema，
// 能大幅降低"返回一段带 markdown 围栏的散文"这类解析失败。
func (c *AIClient) CompleteJSON(
	ctx context.Context,
	settings AICallSettings,
	system, user string,
) (*CompletionResult, error) {
	return c.complete(ctx, settings, system, user, true, 3000)
}

// Complete 普通文本补全。
//
// 画像总结这类输出是一段给人读的中文，套 JSON 反而要多一层解析，
// 而且 response_format=json_object 会让模型倾向于写成字段化的短句，
// 读起来不像人话。maxTokens 由调用方给：总结限 500 字，不需要 3000 的额度。
func (c *AIClient) Complete(
	ctx context.Context,
	settings AICallSettings,
	system, user string,
	maxTokens int,
) (*CompletionResult, error) {
	return c.complete(ctx, settings, system, user, false, maxTokens)
}

// complete 发起一次对话补全，JSON 模式与普通模式共用这套重试与错误处理
func (c *AIClient) complete(
	ctx context.Context,
	settings AICallSettings,
	system, user string,
	jsonMode bool,
	maxTokens int,
) (*CompletionResult, error) {
	// 业务上的"能不能调"由 AISettingService.ResolveCallSettings 在上游判定。
	// 这里只是防编程错误的兜底：绕过那个入口就会发一个没有凭据的请求出去，
	// 拿回一个看不懂的 401
	if settings.APIKey == "" || settings.BaseURL == "" || settings.Model == "" {
		return nil, errors.New("模型配置不完整，请到「设置 → 模型设置」检查")
	}

	baseURL := strings.TrimRight(settings.BaseURL, "/")

	payload := chatRequest{
		Model: settings.Model,
		Messages: []ChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		// 扑克分析要的是稳定可复现，不是创意。低温度能减少同一手牌两次分析结论差异过大
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	}
	if jsonMode {
		payload.ResponseFormat = &respFormat{Type: "json_object"}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	// 只对暂时性错误重试一次：限流、网关抖动。
	// 参数错误、鉴权失败重试多少次都一样，反而浪费时间
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("构造请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+settings.APIKey)

		start := time.Now()
		resp, err := c.http.Do(req)
		if err != nil {
			// 网络层错误（含超时、被内网守卫拒绝）值得重试：
			// 后者不会因为重试变好，但也没有副作用，交给上面的次数上限收敛
			lastErr = fmt.Errorf("请求模型失败: %w", err)
			continue
		}

		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("读取模型响应失败: %w", readErr)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			// 鉴权失败不回显响应体：不少供应商会在 body 里回显提交的 Key 片段
			//（"Incorrect API key provided: sk-abc***"），而这条错误会经
			// error_msg 落库并展示给用户。既然是 Key 的问题就只说 Key 的问题
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("模型鉴权失败（%d），请检查 API Key 是否正确", resp.StatusCode)
			}

			lastErr = fmt.Errorf("模型返回 %d: %s", resp.StatusCode, shorten(string(raw), 200))

			// 4xx 里除了限流都是确定性错误，重试没意义
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return nil, lastErr
			}
			continue
		}

		var parsed chatResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("解析模型响应失败: %w", err)
		}
		if parsed.Error != nil {
			return nil, fmt.Errorf("模型返回错误: %s", parsed.Error.Message)
		}
		if len(parsed.Choices) == 0 {
			return nil, fmt.Errorf("模型返回内容为空")
		}

		content := strings.TrimSpace(parsed.Choices[0].Message.Content)
		if content == "" {
			// 输出被 max_tokens 截断时 content 可能为空，这个原因要明确告诉调用方
			if parsed.Choices[0].FinishReason == "length" {
				return nil, fmt.Errorf("模型输出被长度限制截断，请精简这手牌的记录内容后重试")
			}
			return nil, fmt.Errorf("模型返回内容为空")
		}

		return &CompletionResult{
			Content:    content,
			TokensIn:   parsed.Usage.PromptTokens,
			TokensOut:  parsed.Usage.CompletionTokens,
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return nil, lastErr
}

// shorten 截断过长的错误信息。模型返回的报错可能很长，全塞进数据库没意义
func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
