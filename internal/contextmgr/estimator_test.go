package contextmgr

import (
	"testing"
)

func TestEstimateTokens_Chinese(t *testing.T) {
	text := "你好世界，这是一个测试。"
	tokens := EstimateTokens(text)
	// 8 个中文字符 + 3 个标点 ≈ 16+3 = ~19 tokens
	if tokens < 10 || tokens > 30 {
		t.Errorf("EstimateTokens(%q) = %d, want 10-30", text, tokens)
	}
}

func TestEstimateTokens_English(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog"
	tokens := EstimateTokens(text)
	// 9 个单词 ≈ 12 tokens
	if tokens < 5 || tokens > 20 {
		t.Errorf("EstimateTokens(%q) = %d, want 5-20", text, tokens)
	}
}

func TestEstimateTokens_JSON(t *testing.T) {
	text := `{"steps": [{"description": "test", "tool": "shell.run"}]}`
	tokens := EstimateTokens(text)
	if tokens < 10 || tokens > 30 {
		t.Errorf("EstimateTokens(JSON) = %d, want 10-30", tokens)
	}
}

func TestEstimateTokens_Mixed(t *testing.T) {
	text := `你是一个电脑操控 Agent 的规划器。你的任务是把用户目标拆解成可执行步骤。
可用工具：
- shell.run: 执行命令行命令
- fs: 文件系统操作
输出 JSON 格式: {"steps": []}`
	tokens := EstimateTokens(text)
	if tokens < 50 || tokens > 200 {
		t.Errorf("EstimateTokens(mixed) = %d, want 30-100", tokens)
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	if tokens := EstimateTokens(""); tokens != 0 {
		t.Errorf("EstimateTokens(empty) = %d, want 0", tokens)
	}
}

func TestNewEstimator(t *testing.T) {
	e := NewEstimator(8192)
	if e.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", e.MaxTokens)
	}
	if e.AvailableForHistory() <= 0 {
		t.Errorf("AvailableForHistory() = %d, want > 0", e.AvailableForHistory())
	}
}

func TestNewEstimator_Default(t *testing.T) {
	e := NewEstimator(0)
	if e.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192 (default)", e.MaxTokens)
	}
}

func TestSummarizeHint(t *testing.T) {
	hint := SummarizeHint(5)
	if hint == "" {
		t.Error("SummarizeHint(5) = empty, want non-empty")
	}
	if SummarizeHint(0) != "" {
		t.Error("SummarizeHint(0) = non-empty, want empty")
	}
}

func BenchmarkEstimateTokens(b *testing.B) {
	text := `你是一个电脑操控 Agent 的规划器。你的任务是把用户目标拆解成可执行步骤。
可用工具：shell.run, fs, browser, system, windows
规则：1. 优先使用专用工具 2. 一步只做一件事 3. 输出严格的 JSON 格式
{"steps": [{"description": "步骤描述", "tool": "shell.run", "args": {"command": "echo hello"}}]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(text)
	}
}
