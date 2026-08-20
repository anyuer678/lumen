package agent

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// IntentRouter 意图路由器（预设规则优先，节约算力）
type IntentRouter struct {
	tools map[string]Tool
}

// NewIntentRouter 创建路由器
func NewIntentRouter(tools map[string]Tool) *IntentRouter {
	return &IntentRouter{tools: tools}
}

// Intent 匹配到的意图
type Intent struct {
	Type    string         // direct_answer | tool_call | llm_needed
	Message string         // 直接回答的内容
	Tool    string         // 要调用的工具
	Args    map[string]any // 工具参数
}

// Route 分析用户消息，返回意图
func (r *IntentRouter) Route(message string) Intent {
	lower := message

	// ========== 1. 具体操作类（最高优先级）==========

	// 任务类
	if matchAny(lower, []string{"创建任务", "新建任务"}) {
		goal := extractAfter(lower, []string{"创建任务", "新建任务"})
		if goal == "" {
			return Intent{Type: "direct_answer", Message: "请告诉我任务目标，例如：创建任务 整理下载文件夹"}
		}
		return Intent{Type: "tool_call", Tool: "_create_task", Args: map[string]any{"goal": goal, "priority": 5}}
	}

	// 命令执行类（具体关键词优先）
	if matchAny(lower, []string{"执行命令", "运行命令"}) {
		cmd := extractAfter(lower, []string{"执行命令", "运行命令"})
		if cmd == "" {
			return Intent{Type: "direct_answer", Message: "请指定要执行的命令，例如：执行命令 echo hello"}
		}
		return Intent{Type: "tool_call", Tool: "shell.run", Args: map[string]any{"command": cmd, "timeout": 30}}
	}

	// 文件操作类
	if matchAny(lower, []string{"整理下载", "整理文件夹", "整理文件", "清理下载"}) {
		return Intent{Type: "tool_call", Tool: "fs.list", Args: map[string]any{"action": "list", "path": getDownloadPath()}}
	}
	if matchAny(lower, []string{"查看目录", "列出文件"}) {
		path := extractPath(lower, []string{"查看目录", "列出文件"}, "")
		if path == "" {
			path = "."
		}
		return Intent{Type: "tool_call", Tool: "fs.list", Args: map[string]any{"action": "list", "path": path}}
	}
	if matchAny(lower, []string{"读取文件", "查看文件"}) {
		path := extractPath(lower, []string{"读取文件", "查看文件"}, "")
		if path == "" {
			return Intent{Type: "direct_answer", Message: "请指定要读取的文件路径，例如：读取文件 D:\\test.txt"}
		}
		return Intent{Type: "tool_call", Tool: "fs.read", Args: map[string]any{"action": "read", "path": path}}
	}

	// 网页浏览类
	if matchAny(lower, []string{"查一下", "查查", "帮我查", "帮我搜", "搜索", "搜一下", "上网查", "查资料"}) {
		query := extractSearchQuery(lower)
		if query != "" {
			return Intent{Type: "tool_call", Tool: "browser", Args: map[string]any{"action": "research", "query": query}}
		}
	}
	if matchAny(lower, []string{"打开网页", "访问网站"}) {
		url := extractURL(lower)
		if url == "" {
			return Intent{Type: "direct_answer", Message: "请指定网址，例如：打开网页 https://github.com"}
		}
		return Intent{Type: "tool_call", Tool: "browser", Args: map[string]any{"action": "open", "url": url}}
	}
	if matchAny(lower, []string{"读取页面", "读取网页", "网页内容", "抓取"}) {
		url := extractURL(lower)
		if url == "" {
			return Intent{Type: "direct_answer", Message: "请指定网址，例如：读取网页 https://example.com"}
		}
		return Intent{Type: "tool_call", Tool: "browser", Args: map[string]any{"action": "read", "url": url}}
	}
	if matchAny(lower, []string{"截图", "截屏", "screenshot"}) {
		return Intent{Type: "tool_call", Tool: "browser", Args: map[string]any{"action": "screenshot"}}
	}

	// 系统信息类
	if matchAny(lower, []string{"系统状态", "电脑状态"}) {
		return Intent{Type: "tool_call", Tool: "shell.run", Args: map[string]any{"command": "systeminfo", "timeout": 15}}
	}
	// 系统信息类
	if matchAny(lower, []string{"进程列表", "进程", "正在运行的进程"}) {
		return Intent{Type: "tool_call", Tool: "system", Args: map[string]any{"action": "processes"}}
	}
	if matchAny(lower, []string{"服务列表", "运行的服务", "services"}) {
		return Intent{Type: "tool_call", Tool: "system", Args: map[string]any{"action": "services"}}
	}
	if matchAny(lower, []string{"磁盘空间", "磁盘", "disk"}) {
		return Intent{Type: "tool_call", Tool: "system", Args: map[string]any{"action": "disk"}}
	}
	if matchAny(lower, []string{"git status", "仓库状态", "代码状态"}) {
		return Intent{Type: "tool_call", Tool: "system", Args: map[string]any{"action": "git_status"}}
	}
	if matchAny(lower, []string{"网络状态", "ping", "网络测试"}) {
		target := extractAfter(lower, []string{"ping", "网络测试"})
		if target == "" {
			target = "baidu.com"
		}
		return Intent{Type: "tool_call", Tool: "system", Args: map[string]any{"action": "network", "target": target}}
	}

	// Windows 深层控制类
	if matchAny(lower, []string{"powershell", "执行脚本"}) {
		cmd := extractAfter(lower, []string{"powershell", "执行脚本"})
		if cmd == "" {
			return Intent{Type: "direct_answer", Message: "请指定 PowerShell 命令，例如：powershell Get-Process"}
		}
		return Intent{Type: "tool_call", Tool: "windows", Args: map[string]any{"action": "powershell", "command": cmd}}
	}
	if matchAny(lower, []string{"已安装的软件", "安装的应用", "软件列表", "应用列表"}) {
		return Intent{Type: "tool_call", Tool: "windows", Args: map[string]any{"action": "app_list"}}
	}
	if matchAny(lower, []string{"环境变量", "env"}) {
		return Intent{Type: "tool_call", Tool: "windows", Args: map[string]any{"action": "env", "sub_action": "list"}}
	}
	if matchAny(lower, []string{"剪贴板", "clipboard", "复制内容"}) {
		return Intent{Type: "tool_call", Tool: "windows", Args: map[string]any{"action": "clipboard"}}
	}
	if matchAny(lower, []string{"发通知", "提醒我", "通知"}) {
		msg := extractAfter(lower, []string{"发通知", "提醒我", "通知"})
		if msg == "" {
			msg = "来自智能管家的通知"
		}
		return Intent{Type: "tool_call", Tool: "windows", Args: map[string]any{"action": "notify", "title": "智能管家", "body": msg}}
	}

	// ========== 2. 知识/闲聊类（不调工具）==========
	if matchAny(lower, []string{"帮助", "怎么用", "功能", "你能做什么"}) {
		return Intent{Type: "direct_answer", Message: "我是你的智能管家 AI 助手，核心能力：\n\n🖥 **操作电脑**\n  · 执行 Shell 命令\n  · 读写文件、列目录\n  · 浏览网页\n\n📋 **任务管理**\n  · 自然语言创建任务\n  · AI 自主规划 + 执行\n\n⏰ **定时任务** | 🧠 **记忆系统**\n\n直接告诉我你想做什么！"}
	}
	if matchAny(lower, []string{"时间", "几点", "date", "time", "现在什么时候"}) {
		now := time.Now()
		return Intent{Type: "direct_answer", Message: fmt.Sprintf("当前时间：%s\n星期%s", now.Format("2006-01-02 15:04:05"), weekdayChinese(now.Weekday()))}
	}
	if matchAny(lower, []string{"工具列表", "有哪些工具"}) {
		return Intent{Type: "direct_answer", Message: "当前可用工具：\n\n🖥 shell.run - 执行 Shell 命令\n📁 fs - 文件系统操作\n🌐 browser - 浏览器操控\n\n每个工具都可以通过「工具」页面测试调用。"}
	}

	// ========== 记住类（自动存入知识库）==========
	if matchAny(lower, []string{"记住", "我叫", "我是", "我喜欢", "我的名字", "我住在", "我的生日", "我的邮箱", "我的手机", "我的项目", "我的偏好"} ) {
		if body := extractRemember(lower); body != "" && !isQuestion(lower) {
			tag := detectRememberTag(lower)
			return Intent{Type: "remember", Tool: "_remember", Args: map[string]any{"content": body, "tag": tag}}
		}
		// 疑问句（如"我喜欢什么"）→ 走知识库检索
		if isQuestion(lower) {
			return Intent{Type: "kb_query"}
		}
	}

	// ========== 3. 闲聊类（放最后）==========
	if matchAny(lower, []string{"你好", "嗨", "早上好", "下午好", "晚上好"}) {
		return Intent{Type: "direct_answer", Message: "你好！我是你的智能管家 AI 助手。\n\n我可以帮你：\n🖥 操作电脑（执行命令、读写文件）\n📋 任务管理（创建任务、自主执行）\n⏰ 定时任务\n🧠 记忆系统\n\n直接告诉我你想做什么！"}
	}
	if matchAny(lower, []string{"你是谁", "你叫什么", "你是 ai", "你是机器人"}) {
		return Intent{Type: "direct_answer", Message: "我是运行在你电脑上的 AI 智能管家。\n\n我可以自主分析你的需求，选择合适的工具，然后执行操作。\n\n（关于你的个人信息，需要你先告诉我，我会记住。）"}
	}

	// ========== 未匹配 ==========
	return Intent{Type: "llm_needed"}
}

// matchAny 检查是否匹配任一关键词
func matchAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// extractPath 提取路径
func extractPath(s string, patterns []string, defaultVal string) string {
	for _, p := range patterns {
		idx := strings.Index(s, p)
		if idx >= 0 {
			after := strings.TrimSpace(s[idx+len(p):])
			after = strings.TrimRight(after, "的里的")
			if after != "" {
				return after
			}
		}
	}
	return defaultVal
}

// extractAfter 提取关键词后面的内容
func extractAfter(s string, keywords []string) string {
	for _, kw := range keywords {
		idx := strings.Index(s, kw)
		if idx >= 0 {
			after := strings.TrimSpace(s[idx+len(kw):])
			after = strings.TrimLeft(after, "：: 的")
			return after
		}
	}
	return ""
}

// extractURL 提取消息中的URL
func extractURL(s string) string {
	for _, prefix := range []string{"https://", "http://"} {
		idx := strings.Index(s, prefix)
		if idx >= 0 {
			end := idx + len(prefix)
			for end < len(s) && s[end] != ' ' && s[end] != '\n' {
				end++
			}
			return s[idx:end]
		}
	}
	return ""
}

// extractSearchQuery 提取搜索查询词（去掉"查一下/搜索"等引导词，及常见废话尾）
func extractSearchQuery(s string) string {
	for _, kw := range []string{"查一下", "查查", "帮我查", "帮我搜", "搜索", "搜一下", "上网查", "查资料"} {
		idx := strings.Index(s, kw)
		if idx >= 0 {
			q := strings.TrimSpace(s[idx+len(kw):])
			q = strings.TrimLeft(q, "：:，,")
			// 去掉常见句式尾饰
			for _, end := range []string{"这个问题", "这些内容", "这个", "吧", "？"} {
				q = strings.TrimSuffix(q, end)
			}
			q = strings.TrimSpace(q)
			if q != "" && !strings.HasPrefix(q, "这是什么") {
				return q
			}
		}
	}
	return ""
}

// extractRemember 提取要记住的信息
func extractRemember(s string) string {
	patterns := []struct{ kw, prefix string }{
		{"记住", "记住"},
		{"我叫", "我叫"},
		{"我是", "我是"},
		{"我喜欢", "我喜欢"},
		{"我的名字是", "我的名字是"},
		{"我住在", "我住在"},
		{"我的生日是", "我的生日是"},
		{"我的邮箱是", "我的邮箱是"},
		{"我的手机是", "我的手机是"},
		{"我的项目", "我的项目"},
		{"我的偏好", "我的偏好"},
	}
	for _, p := range patterns {
		if idx := strings.Index(s, p.kw); idx >= 0 {
			after := strings.TrimSpace(s[idx+len(p.kw):])
			after = strings.TrimLeft(after, "：: ，,")
			if after != "" {
				// 去掉结尾语气词
				after = strings.TrimRight(after, "了。！!？?，,")
				return after
			}
		}
	}
	return ""
}

// isQuestion 判断是否是疑问句
func isQuestion(s string) bool {
	// 疑问词或结尾符号
	qs := []string{"什么", "吗", "呢", "几", "哪", "怎么", "是不是", "有没有"}
	for _, q := range qs {
		if strings.Contains(s, q) {
			return true
		}
	}
	s = strings.TrimSpace(s)
	for _, suf := range []string{"?", "？", "?"} {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

// detectRememberTag 检测信息类型标签
func detectRememberTag(s string) string {
	switch {
	case strings.Contains(s, "生日"):
		return "birthday"
	case strings.Contains(s, "名字") || strings.Contains(s, "叫"):
		return "name"
	case strings.Contains(s, "喜欢"):
		return "preference"
	case strings.Contains(s, "邮箱") || strings.Contains(s, "email"):
		return "email"
	case strings.Contains(s, "手机") || strings.Contains(s, "电话"):
		return "phone"
	case strings.Contains(s, "住"):
		return "address"
	case strings.Contains(s, "项目"):
		return "project"
	default:
		return "user_info"
	}
}

// getDownloadPath 获取下载文件夹路径
func getDownloadPath() string {
	if runtime.GOOS == "windows" {
		home, _ := os.UserHomeDir()
		return home + "\\Downloads"
	}
	home, _ := os.UserHomeDir()
	return home + "/Downloads"
}

// weekdayChinese 星期中文
func weekdayChinese(d time.Weekday) string {
	days := [7]string{"日", "一", "二", "三", "四", "五", "六"}
	return days[d]
}
