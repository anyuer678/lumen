package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider OpenAI 兼容的 LLM 提供者（支持 DeepSeek/OpenAI/Ollama 等）
type OpenAIProvider struct {
	config  Config
	client  *http.Client
	tracker *TokenTracker
}

// NewOpenAIProvider 创建 OpenAI 兼容提供者
func NewOpenAIProvider(config Config) *OpenAIProvider {
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &OpenAIProvider{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

// SetTracker 设置 token 追踪器（可选，不设置则不记录）
func (p *OpenAIProvider) SetTracker(t *TokenTracker) {
	p.tracker = t
}

func (p *OpenAIProvider) Name() string { return p.config.Provider }

// openAIRequest OpenAI 兼容请求格式
type openAIRequest struct {
	Model     string           `json:"model"`
	Messages  []openAIMessage  `json:"messages"`
	Tools     []openAITool     `json:"tools,omitempty"`
	MaxTokens int              `json:"max_tokens,omitempty"`
	Stream    bool             `json:"stream"`
	// Temperature 为 nil 时使用提供者默认；显式 0 表示最确定性的输出。
	Temperature *int `json:"temperature,omitempty"`
}

// openAIMessageContent 自定义 content 字段：支持 string 和 array 两种格式。
// OpenAI API 的 content 字段可以是纯字符串，也可以是 ContentPart 数组。
type openAIMessageContent struct {
	Text      string
	Parts     []openAIContentPart
	IsMulti   bool
}

// openAIContentPart OpenAI API 的内容块
type openAIContentPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *openAIImageURL   `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

func (c openAIMessageContent) MarshalJSON() ([]byte, error) {
	if c.IsMulti {
		return json.Marshal(c.Parts)
	}
	return json.Marshal(c.Text)
}

type openAIMessage struct {
	Role      string               `json:"role"`
	Content   openAIMessageContent `json:"content"`
	Name      string               `json:"name,omitempty"`
	ToolCalls []openAIToolCall     `json:"tool_calls,omitempty"`
	ToolID    string               `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string       `json:"type"`
	Function openAIFunc   `json:"function"`
}

type openAIFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function openAIFuncCall   `json:"function"`
}

type openAIFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	// 转换消息格式（支持多模态）
	msgs := make([]openAIMessage, len(messages))
	for i, m := range messages {
		om := openAIMessage{
			Role:   m.Role,
			Name:   m.Name,
			ToolID: "", // 仅 tool 角色使用
		}
		// 多模态消息：ContentParts 非空时序列化为数组
		if len(m.ContentParts) > 0 {
			var parts []openAIContentPart
			for _, p := range m.ContentParts {
				op := openAIContentPart{Type: p.Type, Text: p.Text}
				if p.ImageURL != nil {
					op.ImageURL = &openAIImageURL{URL: p.ImageURL.URL, Detail: p.ImageURL.Detail}
				}
				parts = append(parts, op)
			}
			om.Content = openAIMessageContent{IsMulti: true, Parts: parts}
		} else {
			om.Content = openAIMessageContent{Text: m.Content}
		}
		msgs[i] = om
	}

	// 转换工具定义
	var toolDefs []openAITool
	if len(tools) > 0 {
		toolDefs = make([]openAITool, len(tools))
		for i, t := range tools {
			toolDefs[i] = openAITool{
				Type: "function",
				Function: openAIFunc{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			}
		}
	}

	reqBody := openAIRequest{
		Model:     p.config.Model,
		Messages:  msgs,
		Tools:     toolDefs,
		MaxTokens: p.config.MaxTokens,
		Stream:    false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		// 如果解析失败，尝试作为错误响应处理
		rawResp := string(respBody)
		if len(rawResp) > 200 {
			rawResp = rawResp[:200] + "..."
		}
		return nil, fmt.Errorf("parse response: %w, raw: %s", err, rawResp)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("LLM error: %s (%s)", openAIResp.Error.Message, openAIResp.Error.Type)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openAIResp.Choices[0]

	// 转换工具调用
	var toolCalls []ToolCall
	if len(choice.Message.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = make(map[string]any)
			}
			toolCalls[i] = ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			}
		}
	}

	// 映射 finish_reason
	finish := "stop"
	switch choice.FinishReason {
	case "tool_calls":
		finish = "tool_calls"
	case "length":
		finish = "length"
	}

	// 自动记录 token 用量
	if p.tracker != nil && openAIResp.Usage.TotalTokens > 0 {
		p.tracker.Record(p.config.Provider, p.config.Model, "chat", "", openAIResp.Usage, 0)
	}

	return &Response{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Finish:    finish,
		Usage: Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
	}, nil
}

// BoundedChat 发起一次独立的、无工具、temperature=0、带硬输出预算的模型调用，
// 用于 goal evaluator / reviewer 等不应污染主会话上下文的确定性复核。
// 灵感来自 Reasonix boundedllm。
func (p *OpenAIProvider) BoundedChat(ctx context.Context, system, user string, maxTokens int, timeoutSec int) (string, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if maxTokens <= 0 {
		maxTokens = 256
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	zero := 0
	reqBody := openAIRequest{
		Model: p.config.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: openAIMessageContent{Text: system}},
			{Role: "user", Content: openAIMessageContent{Text: user}},
		},
		MaxTokens:   maxTokens,
		Stream:      false,
		Temperature: &zero,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal bounded request: %w", err)
	}
	url := p.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(callCtx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create bounded request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	client := p.client
	if timeoutSec > 0 && (p.config.Timeout == 0 || time.Duration(timeoutSec)*time.Second < time.Duration(p.config.Timeout)*time.Second) {
		client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send bounded request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read bounded response: %w", err)
	}
	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return "", fmt.Errorf("parse bounded response: %w, raw: %s", err, truncateBytes(respBody, 200))
	}
	if openAIResp.Error != nil {
		return "", fmt.Errorf("LLM error: %s (%s)", openAIResp.Error.Message, openAIResp.Error.Type)
	}
	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in bounded response")
	}
	return openAIResp.Choices[0].Message.Content, nil
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
