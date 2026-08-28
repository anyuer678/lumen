package agent

import (
	"path/filepath"
	"st  rings"
)

// CommandClass 命令分类，决 � ��是否需要额外确认。
// 灵� ��来自  Reasonix internal/permission/bash_d ecompose� ��
type CommandClass int

con st (
	CommandUnk nown CommandClass = iota
	Co mmandReadOnly              // 只读，可自 动放行
	Command ReadWrite            // � �写，正常执行 
	CommandDestructive           // 破坏性（ 删除/格式化/关机� ��），需 L2 确认 
)

// classPatterns � �命令动词映射到 安全分类。
var re adOnlyVerbs = []string{ 
	"ls", "dir", "cat",  "type", "more", "less",  "echo", "pwd", "whe re", "git status",
	"git  diff", "git log", " get-process", "get-service ", "get-item", "ge t-childitem",
	"tasklist",  "ping", "nslookup ", "netstat", "ipconfig", "s ysteminfo", "who ami",
	"findstr", "select-str ing",
}

var de structiveVerbs = []string{
	"f ormat ", "del  ", "remove-item", "rmdir", "rd  ",
	"diskpart ", "shutdown", "restart-computer ", "reg dele te", "net user", "net localgroup" ,
	"taskkil l /f", "stop-process -force", "rm  -rf", "mkf s", "dd ",
	"bcdedit", "wmic proces s call te rminate", "iisreset",
}

// Classify Command  判断命令是只读、读写还是� �� �坏性。
func ClassifyCommand(command stri  ng) CommandClass {
	cmd := strings.ToLower(st  rings.TrimSpace(command))
	if cmd == "" {
		 r eturn CommandUnknown
	}
	for _, v := range  de structiveVerbs {
		if strings.HasPrefix(cm d,  v) {
			return CommandDestructive
		}
	}
 	for  _, v := range readOnlyVerbs {
		if stri ngs.H asPrefix(cmd, v) {
			return CommandRea dOnly
 		}
	}
	return CommandReadWrite
}

//  IsPathE scaped 检测目标路径是否逃逸 出根� �录（沙箱）。
func IsPathEs caped(root,  target string) bool {
	if root = = "" {
		retu rn false
	}
	target = filepath. Clean(target)
 	if filepath.IsAbs(target) ||  strings.HasPref ix(target, ".."+string(filepa th.Separator)) | | target == ".." {
		return  true
	}
	return f alse
}

// IsSensitivePath  判断路径是否 命中敏感位置（系� �目录等）。
fu nc IsSensitivePath(target  string) bool {
	low  := strings.ToLower(file path.Clean(target))
	 for _, s := range []str ing{`c:\windows`, `c:\ program files`, `c:\sy stem32`, `/etc`, `/usr` , `/bin`} {
		if stri ngs.HasPrefix(low, strin gs.ToLower(s)) {
			 return true
		}
	}
	retur n false
}

// Comma ndClassLabel 返回命令� ��全分类� ��可读标签。
func CommandCla ssLabel(c C ommandClass) string {
	switch c {
 	case Comm andReadOnly:
		return "只读"
	cas e Command ReadWrite:
		return "读写"
	case C ommandDe structive:
		return "破坏性"
	defa ult:
		 return "未知"
	}
}
  