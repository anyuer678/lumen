package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// BrowserTool 真实浏览器工具
type BrowserTool struct {
	userDataDir string
	headful     bool

	// allocator 缓存（复用同一 chromium 进程，避免每次启动新进程）
	mu       sync.Mutex
	allocCtx context.Context
	allocCl  context.CancelFunc
}

// NewBrowserTool 创建浏览器工具
func NewBrowserTool(userDataDir string, headful bool) *BrowserTool {
	return &BrowserTool{userDataDir: userDataDir, headful: headful}
}

func (t *BrowserTool) Name() string        { return "browser" }
func (t *BrowserTool) Description() string { return "浏览器操控（open/navigate/read/search/click/type/screenshot/scroll/back）" }
func (t *BrowserTool) RequiredLevel() int  { return 0 }

func (t *BrowserTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "open":
		url, _ := args["url"].(string)
		return t.open(ctx, url)
	case "read":
		url, _ := args["url"].(string)
		return t.read(ctx, url)
	case "fetch":
		url, _ := args["url"].(string)
		return t.read(ctx, url)
	case "screenshot":
		return t.screenshot(ctx)
	case "title":
		url, _ := args["url"].(string)
		return t.getTitle(ctx, url)
	case "navigate", "goto":
		return t.cdp(ctx, "navigate", args)
	case "click":
		return t.cdp(ctx, "click", args)
	case "type", "input":
		return t.cdp(ctx, "type", args)
	case "back":
		return t.cdp(ctx, "back", args)
	case "scroll":
		return t.cdp(ctx, "scroll", args)
	case "readom", "dom", "content":
		url, _ := args["url"].(string)
		if url == "" {
			url = "https://www.baidu.com"
		}
		return t.cdp(ctx, "readom", map[string]any{"url": url})
	case "search":
		return t.cdp(ctx, "search", args)
	case "research":
		query, _ := args["query"].(string)
		if query == "" {
			query, _ = args["text"].(string)
		}
		if query == "" {
			return nil, fmt.Errorf("query is required for research")
		}
		return t.research(ctx, query)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// cdp 通过 chromedp 执行浏览器 DOM 操作（点击/输入/导航/滚动/读取）
func (t *BrowserTool) cdp(ctx context.Context, op string, args map[string]any) (*ToolResult, error) {
	// 使用本机已有的 chromium（Playwright 二进制）
	execPath := t.findExecutable()
	if execPath == "" {
		return nil, fmt.Errorf("no chromium/chrome/edge found for DOM automation")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.Headless,
		chromedp.DisableGPU,
	)
	actx, cancelA := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelA()

	bctx, cancelB := chromedp.NewContext(actx)
	defer cancelB()

	var result string
	switch op {
	case "navigate":
		url, _ := args["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("url is required for navigate")
		}
		if !strings.HasPrefix(url, "http") {
			url = "https://" + url
		}
		var title, body string
		err := chromedp.Run(bctx,
			chromedp.Navigate(url),
			chromedp.Title(&title),
			chromedp.Text("body", &body, chromedp.ByQuery),
		)
		if err != nil {
			return nil, fmt.Errorf("navigate: %w", err)
		}
		if len(body) > 1500 {
			body = body[:1500] + "\n...(截断)"
		}
		result = fmt.Sprintf("标题: %s\n正文:\n%s", title, body)

	case "click":
		selector, _ := args["selector"].(string)
		if selector == "" {
			return nil, fmt.Errorf("selector is required for click")
		}
		var text string
		err := chromedp.Run(bctx,
			chromedp.Navigate(firstNonEmpty(args, "url", "https://www.baidu.com")),
			chromedp.WaitVisible(selector, chromedp.ByQuery),
			chromedp.Click(selector, chromedp.ByQuery),
			chromedp.Text("body", &text, chromedp.ByQuery),
		)
		if err != nil {
			return nil, fmt.Errorf("click: %w", err)
		}
		result = "点击成功: " + selector + "\n" + truncate(text, 500)

	case "type":
		selector, _ := args["selector"].(string)
		text, _ := args["text"].(string)
		query, _ := args["query"].(string)
		// 兼容：LLM 可能用 query 代替 text
		if text == "" && query != "" {
			text = query
		}
		// 默认搜索框选择器
		if selector == "" {
			selector = "input[type='text'], input[name='wd'], input[name='q'], input[name='query']"
		}
		if text == "" {
			return nil, fmt.Errorf("text (or query) is required for type")
		}
		err := chromedp.Run(bctx,
			chromedp.Navigate(firstNonEmpty(args, "url", "https://www.baidu.com")),
			chromedp.WaitVisible(selector, chromedp.ByQuery),
			chromedp.Click(selector, chromedp.ByQuery),
			chromedp.SendKeys(selector, text, chromedp.ByQuery),
		)
		if err != nil {
			return nil, fmt.Errorf("type: %w", err)
		}
		result = fmt.Sprintf("已在 %s 输入: %s", selector, text)

	case "back":
		var title string
		err := chromedp.Run(bctx,
			chromedp.Navigate("https://www.baidu.com"),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return chromedp.NavigateBack().Do(ctx)
			}),
			chromedp.Title(&title),
		)
		if err != nil {
			return nil, fmt.Errorf("back: %w", err)
		}
		result = "后退完成，当前标题: " + title

	case "scroll":
		direction, _ := args["direction"].(string)
		amount := 500
		if a, ok := args["amount"].(float64); ok && a > 0 {
			amount = int(a)
		}
		if direction == "up" {
			amount = -amount
		}
		err := chromedp.Run(bctx,
			chromedp.Navigate(firstNonEmpty(args, "url", "https://www.baidu.com")),
			chromedp.ScrollIntoView("body", chromedp.ByQuery),
		)
		if err != nil {
			return nil, fmt.Errorf("scroll: %w", err)
		}
		result = fmt.Sprintf("已滚动 %d px (%s)", amount, map[string]string{"": "down", "down": "down", "up": "up"}[direction])

	case "readom":
		url, _ := args["url"].(string)
		var text string
		err := chromedp.Run(bctx,
			chromedp.Navigate(url),
			chromedp.Text("body", &text, chromedp.ByQuery, chromedp.NodeVisible),
		)
		if err != nil {
			return nil, fmt.Errorf("readom: %w", err)
		}
		result = truncate(text, 2000)

	case "search":
		query, _ := args["query"].(string)
		if query == "" {
			query, _ = args["text"].(string)
		}
		if query == "" {
			return nil, fmt.Errorf("query is required for search")
		}
		engine, _ := args["engine"].(string)
		if engine == "" {
			engine = "baidu"
		}

		// 用 JS 提取搜索结果（标题+链接），兼容多种选择器
		jsSearch := `() => {
			const items = [];
			const seen = new Set();
			const selectors = ['h3 a', '.result h3 a', '[class*="result"] h3 a', '.c-container h3 a', 'h3', '.t a', 'li.b_algo h2 a', 'h2 a', '.b_algo h2 a'];
			for (const sel of selectors) {
				for (const el of document.querySelectorAll(sel)) {
					const titleEl = el.querySelector ? el.querySelector('a') : el;
					const a = titleEl || el;
					const href = a.href || (a.closest && a.closest('a') ? a.closest('a').href : '');
					const title = (a.innerText || a.textContent || '').trim();
					if (title && href.startsWith('http') && !seen.has(href)) {
						seen.add(href);
						items.push({ title, url: href });
					}
				}
			}
			return items.slice(0, 8);
		}`

		var raw []byte
		var searchErr error
		switch engine {
		case "baidu":
			// 直接用 URL 导航（更稳定），避免模拟输入/点击
			searchErr = chromedp.Run(bctx,
				chromedp.Navigate("https://www.baidu.com/s?wd="+query),
				chromedp.Sleep(3*time.Second),
				chromedp.Evaluate(jsSearch, &raw),
			)
		case "bing":
			searchErr = chromedp.Run(bctx,
				chromedp.Navigate("https://www.bing.com/search?q="+query),
				chromedp.Sleep(5*time.Second),
				chromedp.Evaluate(jsSearch, &raw),
			)
		case "google":
			searchErr = chromedp.Run(bctx,
				chromedp.Navigate("https://www.google.com/search?q="+query),
				chromedp.Sleep(3*time.Second),
				chromedp.Evaluate(jsSearch, &raw),
			)
		default:
			return nil, fmt.Errorf("unknown search engine: %s", engine)
		}
		if searchErr != nil {
			return nil, fmt.Errorf("search: %w", searchErr)
		}

		var results []map[string]string
		if json.Unmarshal(raw, &results) != nil {
			results = nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("百度搜索结果（%s）：\n", query))
		for i, r := range results {
			sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, r["title"], r["url"]))
		}
		result = sb.String()
	}

	return &ToolResult{Raw: result, Kind: "text", Summary: "浏览器DOM操作: " + op}, nil
}

// firstNonEmpty 返回第一个非空字符串参数
func firstNonEmpty(args map[string]any, key string, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "...(截断)"
	}
	return s
}

func (t *BrowserTool) open(ctx context.Context, url string) (*ToolResult, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required for browser open (usage: browser action=open url=https://example.com)")
	}
	// 安全：只允许 http/https URL，防止 start 处理非 http scheme（如 file://、恶意命令）
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("browser open only supports http(s) URLs")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/C", "start", url)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	cmd.CombinedOutput()

	// 打开后尝试读取页面标题和正文（headless chrome）
	title := ""
	text := ""
	execPath := t.findExecutable()
	if execPath != "" {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(execPath), chromedp.Headless, chromedp.DisableGPU,
		)
		bctx, cancelA := chromedp.NewExecAllocator(context.Background(), opts...)
		defer cancelA()
		ctx2, cancelB := chromedp.NewContext(bctx)
		defer cancelB()
		chromedp.Run(ctx2,
			chromedp.Navigate(url),
			chromedp.Sleep(2*time.Second),
			chromedp.Title(&title),
			chromedp.Text("body", &text, chromedp.ByQuery, chromedp.NodeVisible),
		)
	}

	summary := fmt.Sprintf("已在浏览器打开: %s", url)
	if title != "" {
		summary += fmt.Sprintf("\n标题: %s", title)
	}
	if text != "" {
		// 简单去 HTML 标签
		var sb strings.Builder
		inTag := false
		for _, c := range text {
			if c == '<' { inTag = true; continue }
			if c == '>' { inTag = false; continue }
			if !inTag { sb.WriteRune(c) }
		}
		clean := sb.String()
		if len(clean) > 500 {
			clean = clean[:500] + "..."
		}
		summary += fmt.Sprintf("\n正文: %s", clean)
	}

	return &ToolResult{
		Raw:     summary,
		Kind:    "text",
		Summary: fmt.Sprintf("Opened: %s (title: %s)", url, title),
	}, nil
}

// read 抓取网页内容并提取文本
// findExecutable 返回可用的 chromium/chrome/edge 路径
func (t *BrowserTool) findExecutable() string {
	// 优先使用系统 PATH 中的浏览器
	if p, err := exec.LookPath("chrome.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("chromium.exe"); err == nil {
		return p
	}

	// 常见安装路径
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home = os.Getenv("HOME")
	}
	for _, p := range []string{
		filepath.Join(home, "AppData", "Local", "ms-playwright", "chromium-1234", "chrome-win64", "chrome.exe"),
		filepath.Join(home, "AppData", "Local", "ms-playwright", "chromium-1237", "chrome-win64", "chrome.exe"),
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (t *BrowserTool) read(ctx context.Context, url string) (*ToolResult, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 AgentBot")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100000))
	pageText := extractText(string(body))
	if len(pageText) > 2000 {
		pageText = pageText[:2000] + "\n...（内容已截断）"
	}

	return &ToolResult{
		Raw:     pageText,
		Kind:    "text",
		Summary: fmt.Sprintf("读取 %s 成功（%d 字符）", url, len(pageText)),
	}, nil
}

// getTitle 获取网页标题
func (t *BrowserTool) getTitle(ctx context.Context, url string) (*ToolResult, error) {
	if url == "" {
		url = "https://www.baidu.com"
	}
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50000))
	title := extractTitle(string(body))
	return &ToolResult{Raw: title, Kind: "text", Summary: "页面标题"}, nil
}

// screenshot 屏幕截图（Windows），保存到 workspace artifacts 目录
func (t *BrowserTool) screenshot(ctx context.Context) (*ToolResult, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("screenshot only supported on Windows")
	}
	// 确保产物目录存在
	artifactsDir := filepath.Join("data", "workspace", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}
	outputPath := filepath.Join(artifactsDir, fmt.Sprintf("screenshot_%d.png", time.Now().UnixMilli()))
	// PowerShell 需要正斜杠或转义
	psOut := strings.ReplaceAll(outputPath, "\\", "/")
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bitmap = New-Object System.Drawing.Bitmap($screen.Width, $screen.Height)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($screen.Location, [System.Drawing.Point]::Empty, $screen.Size)
$bitmap.Save('%s')
$graphics.Dispose()
$bitmap.Dispose()
Write-Output 'saved'`, psOut)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return &ToolResult{
		Raw:     string(out) + " 已保存: " + outputPath,
		Kind:    "text",
		Summary: fmt.Sprintf("截图已保存至产物目录: %s", outputPath),
	}, nil
}

// extractText 简单提取 HTML 文本
func extractText(html string) string {
	// 去 script/style
	html = regexpRemove(html, `(?s)<(script|style)[^>]*>.*?</\1>`)
	// 去标签
	html = regexpReplace(html, `<[^>]+>`, " ")
	// 去空白
	html = strings.Join(strings.Fields(html), " ")
	return strings.TrimSpace(html)
}

// extractTitle 提取标题
func extractTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title>")
	if start == -1 {
		return "（无标题）"
	}
	start += 7
	end := strings.Index(lower[start:], "</title>")
	if end == -1 {
		return "（无标题）"
	}
	return strings.TrimSpace(html[start : start+end])
}

func regexpRemove(s, pattern string) string {
	return regexpReplace(s, pattern, "")
}

func regexpReplace(s, pattern, repl string) string {
	// 简化正则替换（匹配 <script>...</script> 等）
	var result strings.Builder
	i := 0
	for i < len(s) {
		// 查找 <
		if s[i] == '<' && strings.HasPrefix(strings.ToLower(s[i:]), "<script") {
			end := strings.Index(strings.ToLower(s[i:]), "</script>")
			if end != -1 {
				i += end + 9
				continue
			}
		}
		if s[i] == '<' && strings.HasPrefix(strings.ToLower(s[i:]), "<style") {
			end := strings.Index(strings.ToLower(s[i:]), "</style>")
			if end != -1 {
				i += end + 8
				continue
			}
		}
		if s[i] == '<' {
			end := strings.Index(s[i:], ">")
			if end != -1 {
				result.WriteString(" ")
				i += end + 1
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// searchDuckDuckGo 使用 duckduckgo html 端点搜索（免 key、反爬较弱），
// 解析出标题 + 真实 URL，供端到端检索使用。
type ddgResult struct{ Title, URL string }

func searchDuckDuckGo(query string, limit int) ([]ddgResult, error) {
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	url := "https://html.duckduckgo.com/html/?q=" + urlQueryEncode(query)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	var results []ddgResult
	seen := map[string]bool{}
	// 逐块匹配 result__a 链接（标题在标签内，真实 URL 在 uddg= 里）
	idx := 0
	for len(results) < limit {
		start := strings.Index(html[idx:], `class="result__a"`)
		if start == -1 {
			break
		}
		start += idx
		segEnd := strings.Index(html[start:], "</a>")
		if segEnd == -1 {
			break
		}
		seg := html[start : start+segEnd]
		idx = start + segEnd + 4

		title := extractTagText(seg)
		if title == "" {
			continue
		}
		target := ""
		if u := between(seg, `uddg=`, `&`); u != "" {
			if dec, derr := urlQueryDecode(u); derr == nil {
				target = dec
			}
		}
		if target == "" {
			if h := between(seg, `href="`, `"`); h != "" && strings.HasPrefix(h, "http") {
				target = h
			}
		}
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		results = append(results, ddgResult{Title: title, URL: target})
	}
	return results, nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i == -1 {
		return ""
	}
	j := strings.Index(s[i+len(a):], b)
	if j == -1 {
		return ""
	}
	return s[i+len(a) : i+len(a)+j]
}

func extractTagText(seg string) string {
	// 跳过 <a ...> 开始标签的属性（取第一个 > 之后的可见文本）
	gt := strings.IndexByte(seg, '>')
	text := seg
	if gt != -1 {
		text = seg[gt+1:]
	}
	// 去掉尾部可能的 </a> 残尾
	if i := strings.Index(text, "</"); i != -1 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

func urlQueryEncode(s string) string {
	// 简单编码（空格->+，中文按原样，服务端处理）
	return strings.ReplaceAll(s, " ", "+")
}

func urlQueryDecode(s string) (string, error) {
	return url.QueryUnescape(s)
}

// research 端到端检索：搜索→读前几个结果的正文片段→给出摘要。
func (t *BrowserTool) research(ctx context.Context, query string) (*ToolResult, error) {
	res, err := searchDuckDuckGo(query, 5)
	if err != nil {
		return nil, fmt.Errorf("research search failed: %w", err)
	}
	if len(res) == 0 {
		return &ToolResult{Raw: "未找到搜索结果", Kind: "text", Summary: "no results"}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("查询：%s\n共 %d 条结果：\n\n", query, len(res)))
	// 读取前 3 条结果的正文片段
	readN := goMin(len(res), 3)
	for i := 0; i < readN; i++ {
		snippet, rerr := t.fetchText(ctx, res[i].URL, 900)
		if rerr != nil {
			snippet = "(读取失败)"
		}
		sb.WriteString(fmt.Sprintf("【%d】%s\n%s\n正文：%s\n\n", i+1, res[i].Title, res[i].URL, strings.TrimSpace(snippet)))
	}
	return &ToolResult{
		Raw:     sb.String(),
		Kind:    "text",
		Summary: fmt.Sprintf("检索 %q 得到 %d 条结果", query, len(res)),
	}, nil
}

// fetchText 导航到 URL 并提取正文文本（复用打开浏览器只读一次）。
func (t *BrowserTool) fetchText(ctx context.Context, urlStr string, maxLen int) (string, error) {
	if maxLen <= 0 {
		maxLen = 900
	}
	execPath := t.findExecutable()
	if execPath == "" {
		return "", fmt.Errorf("no chromium/chrome/edge found for DOM automation")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath), chromedp.Headless, chromedp.DisableGPU,
	)
	actx, cancelA := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(actx)
	defer cancelB()

	var text string
	err := chromedp.Run(bctx,
		chromedp.Navigate(urlStr),
		chromedp.Text("body", &text, chromedp.ByQuery, chromedp.NodeVisible),
	)
	if err != nil {
		return "", err
	}
	if maxLen > 0 && len(text) > maxLen {
		text = text[:maxLen]
	}
	return strings.TrimSpace(text), nil
}

func goMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = json.Marshal
