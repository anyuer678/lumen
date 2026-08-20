package agent

import (
	"testing"
)

func TestCategorizeGoal(t *testing.T) {
	tests := []struct {
		goal     string
		expected string
	}{
		{"搜索 Go 语言最新动态", "search"},
		{"读取 conf/config.yaml 文件", "filesystem"},
		{"执行命令 echo hello", "shell"},
		{"打开网页 https://example.com", "browser"},
		{"查看系统进程", "system"},
		{"创建任务整理桌面", "task"},
		{"今天天气怎么样", "general"},
	}
	for _, tt := range tests {
		got := categorizeGoal(tt.goal)
		if got != tt.expected {
			t.Errorf("categorizeGoal(%q) = %q, want %q", tt.goal, got, tt.expected)
		}
	}
}

func TestClassifyErrorType(t *testing.T) {
	tests := []struct {
		err      string
		expected string
	}{
		{"timeout exceeded", "timeout"},
		{"permission denied", "permission"},
		{"安全拦截", "permission"},
		{"file not found", "not_found"},
		{"tool not found: xyz", "tool_not_found"},
		{"connection refused", "network"},
		{"some random error", "other"},
	}
	for _, tt := range tests {
		got := classifyErrorType(tt.err)
		if got != tt.expected {
			t.Errorf("classifyErrorType(%q) = %q, want %q", tt.err, got, tt.expected)
		}
	}
}
