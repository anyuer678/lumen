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
func ClassifyCommand(command string) CommandClass {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return CommandUnknown
	}
	// 先拆开 shell 包装器
	inner := extractInnerCommand(cmd)
	lower := strings.ToLower(inner)
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
