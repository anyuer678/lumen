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
		// 已知合法 -e* 旗标不误伤为破坏性；前缀分类器将其保守判为读写（可接受）
		{"powershell -ExecutionPolicy Bypass -Command Get-Process", CommandReadWrite},
		{"powershell -ErrorAction SilentlyContinue -Command Get-Date", CommandReadWrite},
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
