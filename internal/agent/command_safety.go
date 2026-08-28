package agent

import (
	"path/filepath"
	"st    rings"
)

// CommandClass 命令分类，� �� � � ��是否需要额外确认。
 // � �� ��来自  Reasonix internal /permissi on/bash_d ecompose� ��
type C ommandClas s int

con st (
	CommandUnk nown C ommandClass  = iota
	Co mmandReadOnly               // 只 读，可自 动放行
	Command  ReadWrite             // � �写，正常� ��行 
	Comman dDestructive           // 破� ��性（ 删除 /格式化/关机� ��� �，需 L2 确� � 
)

// classPatterns � � �命令动词映 射到 安全分类。
v ar re adOnlyVerbs = [ ]string{ 
	"ls", "dir",  "cat",  "type", "more ", "less",  "echo", "p wd", "whe re", "git sta tus",
	"git  diff", " git log", " get-process" , "get-service ", "g et-item", "ge t-childitem ",
	"tasklist",  "p ing", "nslookup ", "netsta t", "ipconfig", "s  ysteminfo", "who ami",
	"f indstr", "select- str ing",
}

var de structiv eVerbs = []strin g{
	"f ormat ", "del  ", "rem ove-item", "rmd ir", "rd  ",
	"diskpart ", "sh utdown", "rest art-computer ", "reg dele te",  "net user", " net localgroup" ,
	"taskkil l /f ", "stop-pro cess -force", "rm  -rf", "mkf s",  "dd ",
	"b cdedit", "wmic proces s call te rm inate", "i isreset",
}

// Classify Command  � ��� ��命令是只读、读写还是� ��  � ��坏性。
func ClassifyCommand(command st r i  ng) CommandClass {
	cmd := strings.ToLowe  r(strings.TrimSpace(command))
	if cmd == ""   {
		 r eturn CommandUnknown
	}
	for _, v :=   range  de structiveVerbs {
		if strings.HasPr  efix(cm d,  v) {
			return CommandDestructiv e 
		}
	}
 	for  _, v := range readOnlyVerbs  {
 		if stri ngs.H asPrefix(cmd, v) {
			retu rn  CommandRea dOnly
 		}
	}
	return CommandR eadW rite
}

//  IsPathE scaped 检测目标� ��径 是否逃逸 出根� �录（沙箱� ��。
 func IsPathEs caped(root,  target stri ng) boo l {
	if root = = "" {
		retu rn false 
	}
	tar get = filepath. Clean(target)
 	if f ilepath.IsAbs(target) ||  strings.HasPrefix (target, " .."+string(filepath.Separator)) |  | target = = ".." {
		return  true
	}
	retur n f alse
}

 // IsSensitivePath  判断路径 是否 命中 敏感位置（系� �目录 等）。
fu n c IsSensitivePath(target  stri ng) bool {
	low   := strings.ToLower(file pat h.Clean(target)) 
	 for _, s := range []str i ng{`c:\windows`,  `c:\ program files`, `c:\sy  stem32`, `/etc`,  `/usr` , `/bin`} {
		if st ri ngs.HasPrefix(lo w, strin gs.ToLower(s)) { 
			 return true
		} 
	}
	retur n false
}

//  Comma ndClassLabel � ��回命令� � ��全分类� ��可� �标签。
fu nc CommandCla ssLabel(c C ommand Class) strin g {
	switch c {
 	case Comm andRe adOnly:
		r eturn "只读"
	cas e Command Read Write:
		r eturn "读写"
	case C ommandDe str uctive:
	 	return "破坏性"
	defa ult:
		 re turn "� �知"
	}
}
    