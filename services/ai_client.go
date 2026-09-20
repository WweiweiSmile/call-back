package services

import (
	"bufio"
	"bytes"
	"call-go/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
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
// 刻意不含任何超时/重试参数 —— 调用时长不是"按用户而变"的东西，
// 放进这里只会让人以为某个用户能有更长的思考时间
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
			// 刻意不设 Timeout（0 = 不限）。模型「思考」多久由它自己决定：
			// K3 这类始终推理的模型一次分析可能跑好几分钟，这里兜一道
			// 5 分钟的上限，就等于把超时从 context 挪到了客户端，问题原样保留。
			//
			// 真正拦住"连不上"的是下面拨号器的 30 秒超时（连不通就报错，
			// 不会有半开的连接无限挂着），以及各家服务端自己的请求上限
			Timeout: 0,
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

// chatRequest OpenAI 兼容的请求体。
//
// 刻意不带 max_tokens：K3 这类「始终推理」模型的推理轨迹（reasoning_content）
// 与最终答案共用这一份预算，给一个偏小的值（曾经写死 3000）会被推理吃光 ——
// 表现是 content 为空、finish_reason=length，看着像模型坏了，其实是预算给少了。
//
// 不给上限是安全的：各家服务端都有自己的默认上限（deepseek-chat 4096、
// K3 32768、gpt-4o-mini 16384），我们省下的那个数字既拦不住失控输出，
// 反而先在推理模型上翻了车。长度约束交给提示词自己写（见 prompt_builder.go）
type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []ChatMessage  `json:"messages"`
	ResponseFormat *respFormat    `json:"response_format,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	Stream         bool           `json:"stream"`
	StreamOptions  *streamOptions `json:"stream_options,omitempty"`
}

// streamOptions 目前只有一个用途：让服务端在最后一块里带上 usage。
//
// 流式响应默认不返回 token 用量，不要它就只能自己估。真被哪家供应商拒了
// （回 400 unknown parameter），把这行去掉即可 —— 少一个统计不值当让分析失败
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

const (
	// defaultTemperature 默认温度。扑克分析要的是稳定可复现，不是创意 ——
	// 低温度能减少同一手牌两次分析结论差异过大
	defaultTemperature = 0.3

	// kimiK3Prefix 不接受 temperature 参数的模型前缀。
	//
	// Moonshot 的 K3 是固定温度的推理模型，请求里带上 temperature 会被直接拒绝，
	// 所以这类模型必须**整个字段都不发**，而不是发一个默认值。
	//
	// 用前缀而不是全等，是为了覆盖 kimi-k3-0905、kimi-k3-preview 这类带日期或
	// 阶段后缀的快照名：全等会把它们漏掉，而模型名是用户可以手改的，
	// 漏掉的表现是保存配置成功、一到分析就报 400
	kimiK3Prefix = "kimi-k3"

	// aiIdleTimeout 流式响应多久没有任何数据就判定连接已断。
	//
	// 这是流式下唯一该有的超时形态：**不是总时长限制**。模型算多久都行，
	// 只要它一直在吐字。实测 K3 一次分析 464 秒、平均每秒 31 块，最长静默只有
	// 几秒，120 秒已经非常宽松；它要抓的是"连接活着但对面再也不说话了"
	aiIdleTimeout = 120 * time.Second

	// aiIdleCheckInterval 空闲看门狗的检查间隔。取 10 秒，最坏情况在
	// aiIdleTimeout + 10 秒时被发现，对七八分钟一次的调用来说无所谓
	aiIdleCheckInterval = 10 * time.Second
)

// temperatureFor 返回该模型应当使用的 temperature，nil 表示请求体里不带这个字段。
//
// 只有 Kimi K3 这一支返回 nil，其余模型一律拿默认温度
func temperatureFor(model string) *float64 {
	if strings.HasPrefix(model, kimiK3Prefix) {
		return nil
	}
	t := defaultTemperature
	return &t
}

type respFormat struct {
	Type string `json:"type"`
}

// chatStreamChunk 流式响应里的一块。
//
// 推理模型把思考过程放在 reasoning_content，与最终答案 content 是两个字段：
// 前者我们只数长度（实测一次分析 4 万多字符，存下来既占地方又没什么用），
// 后者才是要落库的正文
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// streamOutcome 一次流式调用读回来的东西
type streamOutcome struct {
	content   string
	tokensIn  int
	tokensOut int
	// reasoningChars 推理轨迹的字符数。只用来记日志，不入库
	reasoningChars int
}

// errStreamOptionsUnsupported 供应商不认 stream_options。
//
// 单独给一个类型，是为了让调用方能认出它、把请求体退化成不带这个字段的版本重来，
// 而不是把整次调用判失败 —— 它只是个可选的统计增强
type errStreamOptionsUnsupported struct{ body string }

func (e *errStreamOptionsUnsupported) Error() string {
	return "供应商不支持 stream_options: " + shorten(e.body, 120)
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
	return c.complete(ctx, settings, system, user, true)
}

// Complete 普通文本补全。
//
// 画像总结这类输出是一段给人读的中文，套 JSON 反而要多一层解析，
// 而且 response_format=json_object 会让模型倾向于写成字段化的短句，
// 读起来不像人话。
//
// 输出长度不由这里管：字数一律交给调用方的提示词，客户端不压 max_tokens 上限
// （聊天提示词要求 300 字；画像总结不限字数）
func (c *AIClient) Complete(
	ctx context.Context,
	settings AICallSettings,
	system, user string,
) (*CompletionResult, error) {
	return c.complete(ctx, settings, system, user, false)
}

// complete 发起一次对话补全，JSON 模式与普通模式共用这套重试与错误处理。
//
// **必须用流式**，这不是偏好问题：非流式的响应在模型把整段答案写完之前一个字节
// 都不发，而 K3 这类「始终推理」模型一次要算七八分钟。实测同一条提示词，非流式
// 跑满 240 秒连响应头都没到，而 Kimi 后台把那次请求记为成功并计了费 —— 响应在
// 链路上被丢了（api.moonshot.cn 背后是阿里云的 DDoS 防护地址）。流式同一条链路、
// 同一时刻，2.8 秒就有数据、464 秒拿到完整结果。调大超时治不了这个病，
// 只有让数据持续流动才行
func (c *AIClient) complete(
	ctx context.Context,
	settings AICallSettings,
	system, user string,
	jsonMode bool,
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
		// 温度与「这个模型收不收 temperature」的判断都在 temperatureFor 里。
		// max_tokens 见 chatRequest 的说明：刻意不发
		Temperature:   temperatureFor(settings.Model),
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	if jsonMode {
		payload.ResponseFormat = &respFormat{Type: "json_object"}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	// 备用请求体：去掉 stream_options。见下面 errStreamOptionsUnsupported 的分支
	noUsage := payload
	noUsage.StreamOptions = nil
	bodyNoUsage, err := json.Marshal(noUsage)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	start := time.Now()
	out, err := c.streamWithRetry(ctx, settings, baseURL, body)

	var unsupported *errStreamOptionsUnsupported
	if errors.As(err, &unsupported) {
		// stream_options 只是个可选的统计增强，被拒了就退一步重来。
		// 为它把整次分析判失败不值得 —— 用户要的是结论，不是 token 数
		log.Printf("[AI] %s 不接受 stream_options，改为不带 usage 重新请求", settings.Model)
		out, err = c.streamWithRetry(ctx, settings, baseURL, bodyNoUsage)
	}
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(start)
	// 推理长度只进日志：一次四万多字符，落库既占地方又没什么可查的
	log.Printf("[AI] 完成 model=%s 正文=%d字 推理=%d字 耗时=%.1fs",
		settings.Model, len([]rune(out.content)), out.reasoningChars, elapsed.Seconds())

	return &CompletionResult{
		Content:    out.content,
		TokensIn:   out.tokensIn,
		TokensOut:  out.tokensOut,
		DurationMs: elapsed.Milliseconds(),
	}, nil
}

// streamWithRetry 发一次流式请求，只对暂时性错误重试一次：限流、网关抖动。
//
// 参数错误、鉴权失败重试多少次都一样，反而浪费时间
func (c *AIClient) streamWithRetry(
	ctx context.Context,
	settings AICallSettings,
	baseURL string,
	body []byte,
) (*streamOutcome, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}

		out, retriable, err := c.streamOnce(ctx, settings, baseURL, body)
		if err == nil {
			return out, nil
		}
		if !retriable {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// streamOnce 发一次流式请求并把整段回答读完。
//
// 第二个返回值表示"这个错误值得重试一次"：网络层错误与 5xx/限流算，
// 参数错误、鉴权失败不算 —— 重试多少次都一样，反而浪费时间
func (c *AIClient) streamOnce(
	ctx context.Context,
	settings AICallSettings,
	baseURL string,
	body []byte,
) (*streamOutcome, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		// 网络层错误（含连接被内网守卫拒绝）值得重试：
		// 后者不会因为重试变好，但也没有副作用，交给上面的次数上限收敛
		return nil, true, fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

		// 鉴权失败不回显响应体：不少供应商会在 body 里回显提交的 Key 片段
		//（"Incorrect API key provided: sk-abc***"），而这条错误会经
		// error_msg 落库并展示给用户。既然是 Key 的问题就只说 Key 的问题
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, false, fmt.Errorf("模型鉴权失败（%d），请检查 API Key 是否正确", resp.StatusCode)
		}

		// 400 里点名 stream_options 的，交给调用方去掉该字段重来。
		// 按字符串匹配而不是按错误码：各家的报错文案不统一，而"参数名出现在
		// 报错里"是它们共同的做法
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(raw), "stream_options") {
			return nil, false, &errStreamOptionsUnsupported{body: string(raw)}
		}

		httpErr := fmt.Errorf("模型返回 %d: %s", resp.StatusCode, shorten(string(raw), 200))
		// 4xx 里除了限流都是确定性错误，重试没意义
		if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return nil, false, httpErr
		}
		return nil, true, httpErr
	}

	return c.readStream(ctx, resp.Body)
}

// readStream 逐块读 SSE，累积最终答案。
//
// 空闲看门狗：每收到一块就刷新"最后活动时间"，超过 aiIdleTimeout 没有任何数据
// 就取消请求判失败。模型算多久都行，只要它一直在吐字
func (c *AIClient) readStream(ctx context.Context, body io.Reader) (*streamOutcome, bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	// 看门狗自己取消的和上游取消的要分开报错，否则用户只会看到一句
	// "context canceled"，完全不知道是模型不说话了
	var idleFired atomic.Bool

	go func() {
		ticker := time.NewTicker(aiIdleCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastActivity.Load())) > aiIdleTimeout {
					idleFired.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	var (
		out     streamOutcome
		content strings.Builder
		finish  string
	)

	sc := bufio.NewScanner(body)
	// 单块可能有几 KB，Scanner 默认 64KB 上限在长回答上会被撑破
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for sc.Scan() {
		line := sc.Text()
		// 只认 data: 行：空行是块分隔符，": keep-alive" 之类的注释行直接跳过
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		lastActivity.Store(time.Now().UnixNano())

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// 单块解不开不该让整次调用失败：已经读到的部分还在，
			// 继续读下一块。真的整段都没内容时下面会统一报错
			continue
		}
		if chunk.Error != nil {
			return nil, false, fmt.Errorf("模型返回错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			out.tokensIn = chunk.Usage.PromptTokens
			out.tokensOut = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.ReasoningContent != "" {
			out.reasoningChars += len([]rune(delta.ReasoningContent))
		}
		if delta.Content != "" {
			content.WriteString(delta.Content)
		}
		if fr := chunk.Choices[0].FinishReason; fr != "" {
			finish = fr
		}
	}

	if err := sc.Err(); err != nil {
		if idleFired.Load() {
			return nil, true, fmt.Errorf("模型已 %d 秒没有任何输出，判定为连接中断",
				int(aiIdleTimeout.Seconds()))
		}
		return nil, true, fmt.Errorf("读取模型响应失败: %w", err)
	}

	out.content = strings.TrimSpace(content.String())
	if out.content == "" {
		// 服务端自己的输出上限也可能把内容截断（我们不发 max_tokens），
		// 这个原因要明确告诉调用方，否则用户只会看到"内容为空"而不知道该改什么
		if finish == "length" {
			return nil, false, errors.New("模型输出被长度限制截断，请精简这手牌的记录内容后重试")
		}
		return nil, false, errors.New("模型返回内容为空")
	}

	return &out, false, nil
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
