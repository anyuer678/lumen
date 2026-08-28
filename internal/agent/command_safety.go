package agent

import (
	"path/filepath"
	"st   rings"
)

// CommandClass 命令分类，� � � ��是否需要额外确认。
// � �� ��来自  Reasonix internal/permissi on/bash_d ecompose� ��
type CommandClas s int

con st (
	CommandUnk nown CommandClass  = iota
	Co mmandReadOnly              // 只 读，可自 动放行
	Command ReadWrite             // � �写，正常执行 
	Comman dDestructive           // 破坏性（ 删除 /格式化/关机� ��），需 L2 确� � 
)

// classPatterns � �命令动词映 射到 安全分类。
var re adOnlyVerbs = [ ]string{ 
	"ls", "dir", "cat",  "type", "more ", "less",  "echo", "pwd", "whe re", "git sta tus",
	"git  diff", "git log", " get-process" , "get-service ", "get-item", "ge t-childitem ",
	"tasklist",  "ping", "nslookup ", "netsta t", "ipconfig", "s ysteminfo", "who ami",
	"f indstr", "select-str ing",
}

var de structiv eVerbs = []string{
	"f ormat ", "del  ", "rem ove-item", "rmdir", "rd  ",
	"diskpart ", "sh utdown", "restart-computer ", "reg dele te",  "net user", "net localgroup" ,
	"taskkil l /f ", "stop-process -force", "rm  -rf", "mkf s",  "dd ",
	"bcdedit", "wmic proces s call te rm inate", "iisreset",
}

// Classify Command  � ��断命令是只读、读写还是� ��  �坏性。
func ClassifyCommand(command st ri  ng) CommandClass {
	cmd := strings.ToLowe r(strings.TrimSpace(command))
	if cmd == ""  {
		 r eturn CommandUnknown
	}
	for _, v :=  range  de structiveVerbs {
		if strings.HasPr efix(cm d,  v) {
			return CommandDestructive 
		}
	}
 	for  _, v := range readOnlyVerbs {
 		if stri ngs.H asPrefix(cmd, v) {
			return  CommandRea dOnly
 		}
	}
	return CommandReadW rite
}

//  IsPathE scaped 检测目标路径 是否逃逸 出根� �录（沙箱）。
 func IsPathEs caped(root,  target string) boo l {
	if root = = "" {
		retu rn false
	}
	tar get = filepath. Clean(target)
 	if filepath.I sAbs(target) ||  strings.HasPref ix(target, " .."+string(filepa th.Separator)) | | target = = ".." {
		return  true
	}
	return f alse
}

 // IsSensitivePath  判断路径是否 命中 敏感位置（系� �目录等）。
fu n c IsSensitivePath(target  string) bool {
	low   := strings.ToLower(file path.Clean(target)) 
	 for _, s := range []str ing{`c:\windows`,  `c:\ program files`, `c:\sy stem32`, `/etc`,  `/usr` , `/bin`} {
		if stri ngs.HasPrefix(lo w, strin gs.ToLower(s)) {
			 return true
		} 
	}
	retur n false
}

// Comma ndClassLabel � ��回命令� ��全分类� ��可� �标签。
func CommandCla ssLabel(c C ommand Class) string {
	switch c {
 	case Comm andRe adOnly:
		return "只读"
	cas e Command Read Write:
		return "读写"
	case C ommandDe str uctive:
		return "破坏性"
	defa ult:
		 re turn "未知"
	}
}
   