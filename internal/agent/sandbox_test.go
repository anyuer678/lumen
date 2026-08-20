package agent

import (
	"testing"
)

func TestShellSandbox_BlockedPath(t *testing.T) {
	tool := &ShellTool{sandbox: true}

	blocked := []struct {
		cmd   string
		label string
	}{
		{"del C:\\Windows\\System32\\test.dll", "系统目录删除"},
		{"copy file.txt C:\\Program Files\\app\\file.txt", "Program Files 写入"},
		{"type C:\\Windows\\win.ini", "系统目录读取"},
		{"echo test > C:\\Windows\\temp.txt", "重定向到系统目录"},
	}

	for _, tc := range blocked {
		err := tool.checkSandboxCommand(tc.cmd)
		if err == nil {
			t.Errorf("sandbox should block %s: %s", tc.label, tc.cmd)
		}
	}
}

func TestShellSandbox_AllowedPath(t *testing.T) {
	tool := &ShellTool{sandbox: true}

	allowed := []string{
		"echo hello",
		"dir data\\workspace",
		"type conf\\config.yaml",
		"copy file.txt data\\workspace\\file.txt",
	}

	for _, cmd := range allowed {
		err := tool.checkSandboxCommand(cmd)
		if err != nil {
			t.Errorf("sandbox should allow: %s, got: %v", cmd, err)
		}
	}
}

func TestShellSandbox_Disabled(t *testing.T) {
	tool := &ShellTool{sandbox: false}
	err := tool.checkSandboxCommand("del C:\\Windows\\System32\\test.dll")
	if err != nil {
		t.Errorf("sandbox disabled should allow everything, got: %v", err)
	}
}

func TestClassifyFailure(t *testing.T) {
	tests := []struct {
		err          string
		recoverable  bool
	}{
		{"timeout", true},
		{"connection refused", true},
		{"exit status 1", true},
		{"permission denied", false},
		{"access denied", false},
		{"安全拦截", false},
		{"unknown tool: xyz", false},
		{"some random error", true}, // 未知错误默认可恢复
	}

	for _, tt := range tests {
		rec, _ := classifyFailure(tt.err)
		if rec != tt.recoverable {
			t.Errorf("classifyFailure(%q) = %v, want %v", tt.err, rec, tt.recoverable)
		}
	}
}
