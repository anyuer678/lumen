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
