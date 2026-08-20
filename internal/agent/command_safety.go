package agent

import (
	"path/filepath"
	"strings"
)

// CommandClass 命令分类，决定是否需要额外确认。
// 灵感来自 Reasonix internal/permission/bash_decompose。
type CommandClass int

const (
	CommandUnknown CommandClass = iota
	CommandReadOnly             // 只读，可自动放行
	CommandReadWrite            // 读写，正常执行
	CommandDestructive          // 破坏性（删除/格式化/关机等），需 L2 确认
)

// classPatterns 把命令动词映射到安全分类。
var readOnlyVerbs = []string{
	"ls", "dir", "cat", "type", "more", "less", "echo", "pwd", "where", "git status",
	"git diff", "git log", "get-process", "get-service", "get-item", "get-childitem",
	"tasklist", "ping", "nslookup", "netstat", "ipconfig", "systeminfo", "whoami",
	"findstr", "select-string",
}

var destructiveVerbs = []string{
	"format ", "del ", "remove-item", "rmdir", "rd ",
	"diskpart", "shutdown", "restart-computer", "reg delete", "net user", "net localgroup",
	"taskkill /f", "stop-process -force", "rm -rf", "mkfs", "dd ",
	"bcdedit", "wmic process call terminate", "iisreset",
}

// ClassifyCommand 判断命令是只读、读写还是破坏性。
func ClassifyCommand(command string) CommandClass {
	cmd := strings.ToLower(strings.TrimSpace(command))
	if cmd == "" {
		return CommandUnknown
	}
	for _, v := range destructiveVerbs {
		if strings.HasPrefix(cmd, v) {
			return CommandDestructive
		}
	}
	for _, v := range readOnlyVerbs {
		if strings.HasPrefix(cmd, v) {
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
	target = filepath.Clean(target)
	if filepath.IsAbs(target) || strings.HasPrefix(target, ".."+string(filepath.Separator)) || target == ".." {
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
