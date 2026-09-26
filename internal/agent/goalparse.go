package agent

import (
	"regexp"
	"strings"
)

// extractFilePath 从中文描述中提取文件路径
func extractFilePath(goal string) string {
	// 方法1：找反斜杠或正斜杠分割的路径
	for i := 0; i < len(goal); i++ {
		if goal[i] == '/' || goal[i] == '\\' {
			start := i
			for start > 0 {
				c := goal[start-1]
				if c == ' ' || c == ',' || c == '.' || c > 127 {
					break
				}
				start--
			}
			end := i
			for end < len(goal) {
				c := goal[end]
				if c == ' ' || c == ',' || c == '.' || c == '\n' || c > 127 {
					break
				}
				end++
			}
			path := goal[start:end]
			if strings.Contains(path, ".") || strings.Contains(path, "/") || strings.Contains(path, "\\") {
				return strings.TrimSpace(path)
			}
		}
	}

	// 方法2：匹配常见文件名模式
	filePatterns := []string{
		`conf/config\.yaml`,
		`data/workspace/[\w\-\.]+`,
		`[\w\-]+\.(txt|md|yaml|json|go|py|js|log)`,
	}
	for _, pattern := range filePatterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindString(goal); match != "" {
			return match
		}
	}

	return ""
}

// extractCommandFromGoal 从任务目标中提取命令
func extractCommandFromGoal(goal string) string {
	lower := strings.ToLower(goal)

	// 1. 自然语言 → 命令映射（最高优先级，先匹配再提取）
	type goalPattern struct {
		keywords []string
		cmd      string
		exact    bool // 是否需要精确匹配（包含而非部分匹配）
		extract  func(goal string) string
	}
	patterns := []goalPattern{
		// 系统信息
		{[]string{"系统时间", "当前时间", "几点"}, "Get-Date", false, nil},
		{[]string{"whoami", "当前用户", "用户名"}, "whoami", false, nil},
		{[]string{"磁盘空间", "查询 C 盘磁盘"}, "Get-PSDrive C", false, nil},
		{[]string{"hostname", "计算机名", "主机名"}, "hostname", false, nil},
		{[]string{"环境变量", "查看系统环境变量"}, "Get-ChildItem env:", false, nil},
		{[]string{"列出当前运行的进程", "进程列表"}, "Get-Process | Select-Object -First 10", false, nil},
		{[]string{"网络状态", "ipconfig", "ip 地址"}, "ipconfig", false, nil},
		{[]string{"ping "}, "ping baidu.com", false, func(g string) string {
			idx := strings.Index(strings.ToLower(g), "ping ")
			if idx >= 0 {
				after := strings.TrimSpace(g[idx+5:])
				after = strings.TrimLeft(after, "：: 的")
				if after != "" {
					return "ping " + after
				}
			}
			return "ping baidu.com"
		}},
		// 文件操作（单步）
		{[]string{"读取文件", "查看文件", "读取配置"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "type " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取文件路径"
		}},
		{[]string{"列出目录", "查看目录", "目录内容"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path == "" {
				path = "."
			}
			return "dir " + strings.ReplaceAll(path, "/", "\\")
		}},
		{[]string{"检查文件存在", "文件是否存在"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "if exist " + strings.ReplaceAll(path, "/", "\\") + " echo EXISTS"
			}
			return "echo 无法提取文件路径"
		}},
		{[]string{"统计文件数", "有多少个文件"}, "powershell -Command \"(Get-ChildItem -File).Count\"", false, nil},
		{[]string{"列出子目录", "子目录"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path == "" {
				path = "."
			}
			return "dir /ad " + strings.ReplaceAll(path, "/", "\\")
		}},
		{[]string{"创建目录", "新建目录"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "mkdir " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取目录路径"
		}},
		{[]string{"创建文件", "新建文件"}, "", false, func(g string) string {
			// 尝试提取文件路径和内容
			path := extractFilePath(g)
			content := ""
			// 手动提取"内容为"后面的内容
			for _, kw := range []string{"内容为", "内容是"} {
				idx := strings.Index(g, kw)
				if idx >= 0 {
					after := g[idx+len(kw):]
					after = strings.TrimLeft(after, "：: 的")
					endIdx := len(after)
					for i, c := range after {
						if c == '。' || c == '\n' || c == '，' || c == '；' {
							endIdx = i
							break
						}
					}
					content = strings.TrimSpace(after[:endIdx])
					break
				}
			}
			if path != "" && content != "" {
				return "echo " + content + " > " + strings.ReplaceAll(path, "/", "\\")
			} else if path != "" {
				return "echo. > " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取文件路径"
		}},
		{[]string{"删除文件", "移除文件"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "del " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取文件路径"
		}},
	}

	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				if p.extract != nil {
					return p.extract(goal)
				}
				return p.cmd
			}
		}
	}

	// 2. 显式命令前缀（"执行命令 xxx"）
	for _, prefix := range []string{"执行命令", "运行命令", "执行", "运行"} {
		if strings.HasPrefix(goal, prefix) {
			after := strings.TrimSpace(goal[len(prefix):])
			after = strings.TrimLeft(after, "：: 的 ")
			if after != "" {
				return after
			}
		}
	}

	// 3. 提取英文命令
	if idx := strings.IndexAny(goal, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"); idx >= 0 {
		// 找到连续的英文单词
		end := idx
		for end < len(goal) && goal[end] != ' ' && goal[end] != '\n' {
			end++
		}
		cmd := goal[idx:end]
		// 过滤掉常见的非命令英文词
		nonCommands := []string{"the", "and", "for", "with", "from", "to", "http", "https", "www"}
		isCmd := true
		for _, nc := range nonCommands {
			if strings.ToLower(cmd) == nc {
				isCmd = false
				break
			}
		}
		if isCmd && len(cmd) >= 2 {
			return cmd
		}
	}

	// 4. 尝试去掉乱码前缀
	for _, prefix := range []string{"???? ", "??? ", "?? ", "? "} {
		if strings.HasPrefix(goal, prefix) {
			after := strings.TrimSpace(goal[len(prefix):])
			if after != "" {
				return after
			}
		}
	}

	// 4. 如果是纯英文命令，直接使用
	if !containsChinese(goal) {
		return goal
	}

	// 5. 无法提取，返回 echo
	return "echo " + goal
}

// containsChinese 检查字符串是否包含中文字符
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
