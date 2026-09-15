package services

import (
	"bytes"
	"call-go/config"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// AIClient 大模型客户端。
//
// 走 OpenAI 兼容的 /chat/completions 协议，所以换供应商只需要改 baseURL 和 model，
// 不用改代码。刻意不引 SDK：go.mod 目前零 AI 依赖，保持轻。
type AIClient struct {
	http *http.Client
}

// NewAIClient 构造客户端。
//
// 刻意不在构造函数里读配置：那样在 LoadConfig 之前构造（例如只注册路由的单测）
// 会 nil 解引用崩溃。API Key / BaseURL / Model 都在发起请求时惰性读取，
// 顺带也让运行时改配置能立即生效。
func NewAIClient() *AIClient {
	return &AIClient{
		// 单次请求的真实超时由 context 控制（CompleteJSON 里按 AITimeoutSec 设置），
		// 这里给一个宽松上限兜底，避免连接层无限挂起
		http: &http.Client{Timeout: 5 * time.Minute},
	}
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
func (c *AIClient) CompleteJSON(ctx context.Context, system, user string) (*CompletionResult, error) {
	settings := config.AIConfig()
	if !settings.Enabled || settings.APIKey == "" {
		return nil, fmt.Errorf("服务端未配置 AI API Key，AI 分析不可用")
	}
	baseURL := strings.TrimRight(settings.BaseURL, "/")

	payload := chatRequest{
		Model: settings.Model,
		Messages: []ChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: &respFormat{Type: "json_object"},
		// 扑克分析要的是稳定可复现，不是创意。低温度能减少同一手牌两次分析结论差异过大
		Temperature: 0.3,
		MaxTokens:   3000,
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
			// 网络层错误（含超时）值得重试
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
