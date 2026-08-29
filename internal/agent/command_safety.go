package agent

import (
	"path/filepath"
	"strings"
)

// CommandClass 命令分类，决定是否需要额外确认。
type CommandClass int

const (
	CommandUnknown     CommandClass = iota
	CommandReadOnly                   // 只读，可自动放行
	CommandReadWrite                  // 读写，正常执行
	CommandDestructive                // 破坏性（删除/格式化/关机等），需 L2 确认
)

// readOnlyVerbs 命令动词映射到安全分类。
var readOnlyVerbs = []string{
	"ls", "dir", "cat", "type", "more", "less", "echo", "pwd", "where", "git status",
	"git diff", "git log", "get-process", "get-service", "get-item", "get-childitem",
	"tasklist", "ping", "nslookup", "netstat", "ipconfig", "systeminfo", "whoami",
	"findstr", "select-string",
}

var destructiveVerbs = []string{
	"format ", "del ", "remove-item", "rmdir", "rd ",
	"diskpart ", "shutdown", "restart-computer ", "reg delete", "net user", "net localgroup",
	"taskkill /f", "stop-process -force", "rm -rf", "rm -r ", "rm -f ", "rm ",
	"mkfs", "dd ", "bcdedit", "wmic process call terminate", "iisreset",
}

// shellWrapperPrefixes 需要提取内部命令再分类的 shell 包装器。
var shellWrapperPrefixes = []string{"cmd /c ", "cmd /k ", "powershell -command ", "powershell -c ", "sh -c ", "bash -c ", "zsh -c "}

// extractInnerCommand 如果命令被 shell 包装器包裹，提取内部实际命令。
func extractInnerCommand(cmd string) string {
	lower := strings.ToLower(cmd)
	for _, prefix := range shellWrapperPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(cmd[len(prefix):])
		}
	}
	return cmd
}

// ClassifyCommand 判断命令是只读、读写还是破坏性。
// splitShellOperators splits command by shell operators.
// Supports: &&, ||, |, ;, &
func splitShellOperators(cmd string) []string {
	var segments []string
	current := strings.Builder{}

	i := 0
	for i < len(cmd) {
		if i+1 < len(cmd) {
			twoChar := string(cmd[i]) + string(cmd[i+1])
			if twoChar == "&&" || twoChar == "||" {
				if current.Len() > 0 {
					segments = append(segments, strings.TrimSpace(current.String()))
					current.Reset()
				}
				i += 2
				continue
			}
		}
		if cmd[i] == '|' || cmd[i] == ';' || cmd[i] == '&' {
			if current.Len() > 0 {
				segments = append(segments, strings.TrimSpace(current.String()))
				current.Reset()
			}
			i++
			continue
		}
		current.WriteByte(cmd[i])
		i++
	}
	if current.Len() > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}
	return segments
}

func classifySingleCommand(command string) CommandClass {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return CommandUnknown
	}
	// 先拆开 shell 包装器
	inner := extractInnerCommand(cmd)
	lower := strings.ToLower(inner)
	// 编码式 PowerShell（-e/-ec/-enc/-EncodedCommand）的载荷无法静态分析，
	// base64 可携带任意指令，一律按破坏性处理
	if looksLikeEncodedPowerShell(lower) {
		return CommandDestructive
	}
	// 检查破坏性
	for _, v := range destructiveVerbs {
		if strings.HasPrefix(lower, v) {
			return CommandDestructive
		}
	}
	// 检查只读
	for _, v := range readOnlyVerbs {
		if strings.HasPrefix(lower, v) {
			return CommandReadOnly
		}
	}
	return CommandReadWrite
}

// looksLikeEncodedPowerShell 识别编码式 PowerShell 调用。
// 已知 -e* 合法长旗标（-ErrorAction/-ErrorVariable/-ExecutionPolicy/-Exit）不误伤。
func looksLikeEncodedPowerShell(lower string) bool {
	if !strings.HasPrefix(lower, "powershell") && !strings.HasPrefix(lower, "pwsh") {
		return false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(lower, "powershell"), "pwsh")
	knownEFlags := map[string]bool{
		"-erroraction": true, "-errorvariable": true, "-executionpolicy": true, "-exit": true,
	}
	for _, f := range strings.Fields(rest) {
		if !strings.HasPrefix(f, "-e") {
			continue
		}
		switch f {
		case "-e", "-ec", "-enc", "-encoded", "-encodedcommand":
			return true
		}
		if knownEFlags[f] {
			continue
		}
		// -encodedcommand<base64> 紧贴形式
		if strings.HasPrefix(f, "-encodedcommand") && len(f) > len("-encodedcommand") {
			return true
		}
		// -e<base64> 紧贴形式（排除上面的已知合法旗标）
		if len(f) > 2 {
			return true
		}
	}
	return false
}
// ClassifyCommand classifies command as read-only, read-write, or destructive.
// Splits by shell operators and classifies each segment; worst segment wins.
func ClassifyCommand(command string) CommandClass {
	segments := splitShellOperators(command)
	if len(segments) == 0 {
		return CommandUnknown
	}
	worst := CommandReadOnly
	for _, seg := range segments {
		cls := classifySingleCommand(seg)
		if cls > worst {
			worst = cls
		}
	}
	return worst
}

// IsPathEscaped 检测目标路径是否逃逸出根目录（沙箱）。
func IsPathEscaped(root, target string) bool {
	if root == "" {
		return false
	}
	// 解析为绝对路径再比对，防止 root 参数被忽略
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return true // 无法解析 root，保守拒绝
	}
	// target 如果是相对路径，基于 root 解析
	absTarget, err := filepath.Abs(filepath.Join(root, target))
	if err != nil {
		return true
	}
	// 清理后检查 target 是否仍在 root 下
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return true
	}
	return false
}

// IsSensitivePath 判断路径是否命中敏感位置（系统目录等）。
func IsSensitivePath(target string) bool {
	low := strings.ToLower(filepath.Clean(target))
	for _, s := range []string{`c:\windows`, `c:\program files`, `c:\system32`, `/etc`, `/usr`, `/bin`} {
		if strings.HasPrefix(low, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

// CommandClassLabel 返回命令安全分类的可读标签。
func CommandClassLabel(c CommandClass) string {
	switch c {
	case CommandReadOnly:
		return "只读"
	case CommandReadWrite:
		return "读写"
	case CommandDestructive:
		return "破坏性"
	default:
		return "未知"
	}
}
