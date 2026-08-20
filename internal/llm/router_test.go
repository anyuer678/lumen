package llm

import (
	"context"
	"math"
	"testing"
)

// mockProvider 测试用最小 Provider 实现
type mockProvider struct{ name string }

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	return &Response{Content: "mock"}, nil
}

func TestRouter_Route(t *testing.T) {
	providers := map[string]Provider{
		"zhipu":    &mockProvider{name: "zhipu"},
		"ollama":   &mockProvider{name: "ollama"},
		"deepseek": &mockProvider{name: "deepseek"},
	}
	config := RouterConfig{
		Default: "zhipu",
		Simple:  "ollama",
		Complex: "zhipu",
		Vision:  "zhipu",
		Local:   "ollama",
	}
	router := NewRouter(providers, config)

	tests := []struct {
		name     string
		task     string
		tools    []string
		expected string
	}{
		{"简单任务", "echo hello", nil, "ollama"},
		{"复杂任务", "分析代码架构", nil, "zhipu"},
		{"图像任务", "截图分析", nil, "zhipu"},
		{"未知任务", "随便做点什么", nil, "zhipu"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.Route(tt.task, tt.tools)
			if result != tt.expected {
				t.Errorf("Route(%q) = %q, want %q", tt.task, result, tt.expected)
			}
		})
	}
}

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		prompt   int
		compl    int
		expected float64
	}{
		{"zhipu flash free", "zhipu", "glm-4-flash", 100, 100, 0.0},
		{"deepseek chat", "deepseek", "deepseek-chat", 1000, 1000, 0.00042},
		{"ollama local free", "ollama", "qwen3:0.6b", 1000, 1000, 0.0},
		{"unknown provider", "unknown", "model", 100, 100, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := Usage{
				PromptTokens:     tt.prompt,
				CompletionTokens: tt.compl,
				TotalTokens:      tt.prompt + tt.compl,
			}
			cost := calculateCost(tt.provider, tt.model, usage)
			// 浮点容差比较（0.00042 vs 0.00042000000000006）
			if math.Abs(cost-tt.expected) > 1e-9 {
				t.Errorf("calculateCost(%q, %q) = %f, want %f", tt.provider, tt.model, cost, tt.expected)
			}
		})
	}
}
