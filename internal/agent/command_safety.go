package agent

import (
	"path/filepath"
	"st rings"
)

// CommandClass 命令分类，决� ��是否需要额外确认。
// 灵感来自  Reasonix internal/permission/bash_decompose� ��
type CommandClass int

const (
	CommandUnk nown CommandClass = iota
	CommandReadOnly              // 只读，可自动放行
	Command ReadWrite            // 读写，正常执行 
	CommandDestructive          // 破坏性（ 删除/格式化/关机等），需 L2 确认 
)

// classPatterns 把命令动词映射到 安全分类。
var readOnlyVerbs = []string{ 
	"ls", "dir", "cat", "type", "more", "less",  "echo", "pwd", "where", "git status",
	"git  diff", "git log", "get-process", "get-service ", "get-item", "get-childitem",
	"tasklist",  "ping", "nslookup", "netstat", "ipconfig", "s ysteminfo", "whoami",
	"findstr", "select-str ing",
}

var destructiveVerbs = []string{
	"f ormat ", "del ", "remove-item", "rmdir", "rd  ",
	"diskpart", "shutdown", "restart-computer ", "reg delete", "net user", "net localgroup" ,
	"taskkill /f", "stop-process -force", "rm  -rf", "mkfs", "dd ",
	"bcdedit", "wmic proces s call terminate", "iisreset",
}

// Classify Command 判断命令是只读、读写还是� ��坏性。
func ClassifyCommand(command stri ng) CommandClass {
	cmd := strings.ToLower(st rings.TrimSpace(command))
	if cmd == "" {
		r eturn CommandUnknown
	}
	for _, v := range de structiveVerbs {
		if strings.HasPrefix(cmd,  v) {
			return CommandDestructive
		}
	}
	for  _, v := range readOnlyVerbs {
		if strings.H asPrefix(cmd, v) {
			return CommandReadOnly
 		}
	}
	return CommandReadWrite
}

// IsPathE scaped 检测目标路径是否逃逸出根� �录（沙箱）。
func IsPathEscaped(root,  target string) bool {
	if root == "" {
		retu rn false
	}
	target = filepath.Clean(target)
 	if filepath.IsAbs(target) || strings.HasPref ix(target, ".."+string(filepath.Separator)) | | target == ".." {
		return true
	}
	return f alse
}

// IsSensitivePath 判断路径是否 命中敏感位置（系统目录等）。
fu nc IsSensitivePath(target string) bool {
	low  := strings.ToLower(filepath.Clean(target))
	 for _, s := range []string{`c:\windows`, `c:\ program files`, `c:\system32`, `/etc`, `/usr` , `/bin`} {
		if strings.HasPrefix(low, strin gs.ToLower(s)) {
			return true
		}
	}
	retur n false
}

// CommandClassLabel 返回命令� ��全分类的可读标签。
func CommandCla ssLabel(c CommandClass) string {
	switch c {
 	case CommandReadOnly:
		return "只读"
	cas e CommandReadWrite:
		return "读写"
	case C ommandDestructive:
		return "破坏性"
	defa ult:
		return "未知"
	}
}
 