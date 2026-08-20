package llm

import (
	"context"
)

// ContentPart 内容块（用于多模态消息）
type ContentPart struct {
	Type     string     `json:"type"`                // "text" 或 "image_url"
	Text     string     `json:"text,omitempty"`       // type=text 时使用
	ImageURL *ImageURL  `json:"image_url,omitempty"` // type=image_url 时使用
}

// ImageURL 图片 URL（支持 base64 data URI）
type ImageURL struct {
	URL    string `json:"url"`    // http(s) URL 或 data:image/png;base64,... 格式
	Detail string `json:"detail,omitempty"` // "auto" | "low" | "high"
}

// Message 消息（支持纯文本和多模态两种模式）
type Message struct {
	Role    string `json:"role"`    // system|user|assistant|tool
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
	// 多模态内容：当 Content 为空且 ContentParts 非空时使用
	ContentParts []ContentPart `json:"content_parts,omitempty"`
}

// GetContent 获取消息文本内容（兼容多模态和纯文本两种模式）。
// 多模态消息中提取所有文本部分拼接返回。
func (m Message) GetContent() string {
	if m.Content != "" {
		return m.Content
	}
	if len(m.ContentParts) == 0 {
		return ""
	}
	var result string
	for _, part := range m.ContentParts {
		if part.Type == "text" && part.Text != "" {
			if result != "" {
				result += "\n"
			}
			result += part.Text
		}
	}
	return result
}

// IsMultimodal 检查消息是否包含图片内容。
func (m Message) IsMultimodal() bool {
	for _, part := range m.ContentParts {
		if part.Type == "image_url" && part.ImageURL != nil {
			return true
		}
	}
	return false
}

// NewTextMessage 创建纯文本消息
func NewTextMessage(role, content string) Message {
	return Message{Role: role, Content: content}
}

// NewImageMessage 创建包含图片的消息（文本 + 图片）
func NewImageMessage(role, text, imageURL string) Message {
	parts := []ContentPart{{Type: "text", Text: text}}
	if imageURL != "" {
		parts = append(parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: imageURL, Detail: "auto"},
		})
	}
	return Message{Role: role, ContentParts: parts}
}

// ToolDef 工具定义（给 LLM 的 schema）
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall LLM 返回的工具调用
type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"arguments"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// Response LLM 响应
type Response struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Finish    string     `json:"finish_reason"` // stop|tool_calls|length
	Usage     Usage      `json:"usage"`
}

// Usage Token 用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider LLM 提供者接口
type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}

// Config LLM 配置
type Config struct {
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	APIKey    string `yaml:"api_key"`
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
	Timeout   int    `yaml:"timeout"` // seconds
}
