package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agent/internal/vision"
)

// ComputerTool Computer Use 工具：通过 PowerShell/.NET 控制鼠标/键盘/截图/窗口
// 灵感来自 Reasonix boundedllm + OpenClaw 的 observe→act→verify 循环
type ComputerTool struct {
	workspaceDir string
	vision       *vision.Analyzer // 可选：截图后自动分析
}

// NewComputerTool 创建 Computer Use 工具
func NewComputerTool(workspaceDir string) *ComputerTool {
	return &ComputerTool{workspaceDir: workspaceDir}
}

// SetVisionAnalyzer 注入视觉分析器（LLM provider 可用后调用）
func (t *ComputerTool) SetVisionAnalyzer(a *vision.Analyzer) {
	t.vision = a
}

func (t *ComputerTool) Name() string { return "computer" }
func (t *ComputerTool) Description() string {
	if t.vision != nil {
		return "Computer Use（截图/鼠标/键盘/窗口管理，截图后可选视觉分析）"
	}
	return "Computer Use（截图/鼠标/键盘/窗口管理）"
}
func (t *ComputerTool) RequiredLevel() int { return 2 }

func (t *ComputerTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return nil, fmt.Errorf("action is required (screenshot|mouse|keyboard|window_list|window_focus|window_resize)")
	}

	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("computer tool only supports Windows")
	}

	switch action {
	case "screenshot":
		return t.screenshot(ctx, args)
	case "mouse":
		return t.mouse(ctx, args)
	case "keyboard":
		return t.keyboard(ctx, args)
	case "window_list":
		return t.windowList(ctx)
	case "window_focus":
		return t.windowFocus(ctx, args)
	case "window_resize":
		return t.windowResize(ctx, args)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// screenshot 截取当前屏幕，可选视觉分析
func (t *ComputerTool) screenshot(ctx context.Context, args map[string]any) (*ToolResult, error) {
	// 用 .NET 截取整个屏幕
	psScript := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
try {
    $bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
    $bitmap = New-Object System.Drawing.Bitmap($bounds.Width, $bounds.Height)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $graphics.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
    $path = Join-Path $env:TEMP ("screenshot_" + [DateTime]::Now.ToString("yyyyMMdd_HHmmss") + ".png")
    $bitmap.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
    $graphics.Dispose()
    $bitmap.Dispose()
    Write-Output $path
} catch {
    Write-Output "ERROR: $($_.Exception.Message)"
}
`
	path := t.runPowerShell(psScript)
	path = strings.TrimSpace(path)

	if strings.HasPrefix(path, "ERROR:") {
		return nil, fmt.Errorf("screenshot failed: %s", path)
	}

	// 复制到 workspace/artifacts
	if t.workspaceDir != "" {
		dest := filepath.Join(t.workspaceDir, "artifacts", filepath.Base(path))
		os.MkdirAll(filepath.Dir(dest), 0755)
		if data, err := os.ReadFile(path); err == nil {
			os.WriteFile(dest, data, 0644)
			path = dest
		}
	}

	raw := fmt.Sprintf("截图已保存: %s", path)
	summary := fmt.Sprintf("Screenshot saved: %s", path)

	// 可选：视觉分析截图
	if t.vision != nil {
		question := "分析这张屏幕截图的内容，识别主要 UI 元素和当前状态。"
		if q, ok := getArgString(args, "question"); ok && q != "" {
			question = q
		}
		analysis, err := t.vision.AnalyzeScreenshot(ctx, path, question)
		if err == nil && analysis != nil {
			raw += "\n\n--- 视觉分析 ---\n"
			raw += fmt.Sprintf("摘要: %s\n", analysis.Summary)
			if analysis.ActiveWindow != "" {
				raw += fmt.Sprintf("活动窗口: %s\n", analysis.ActiveWindow)
			}
			if len(analysis.Elements) > 0 {
				raw += "识别元素:\n"
				for _, el := range analysis.Elements {
					raw += fmt.Sprintf("  - [%s] %s (%s)\n", el.Type, el.Label, el.BBox)
				}
			}
			if analysis.Suggestion != "" {
				raw += fmt.Sprintf("建议: %s\n", analysis.Suggestion)
			}
			summary = fmt.Sprintf("Screenshot + analysis: %s", analysis.Summary)
		}
	}

	return &ToolResult{
		Raw:     raw,
		Kind:    "screenshot",
		Summary: summary,
	}, nil
}

// getArgString 从 args 中安全获取字符串值（nil-safe）
func getArgString(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	v, ok := args[key].(string)
	return v, ok
}

// mouse 鼠标操作（move/click/drag）
func (t *ComputerTool) mouse(ctx context.Context, args map[string]any) (*ToolResult, error) {
	sub, _ := args["sub_action"].(string)
	if sub == "" {
		sub, _ = args["action"].(string)
		if sub == "mouse" {
			sub = "click" // 默认点击
		}
	}

	x, _ := args["x"].(float64)
	y, _ := args["y"].(float64)

	switch sub {
	case "move":
		psScript := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
}
"@
[Mouse]::SetCursorPos(%d, %d)
Write-Output "moved to %d,%d"
`, int(x), int(y), int(x), int(y))
		output := t.runPowerShell(psScript)
		return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Mouse moved to (%d,%d)", int(x), int(y))}, nil

	case "click":
		psScript := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, int dwExtraInfo);
}
"@
[Mouse]::SetCursorPos(%d, %d)
Start-Sleep -Milliseconds 50
[Mouse]::mouse_event(0x0002, 0, 0, 0, 0)  # left down
Start-Sleep -Milliseconds 50
[Mouse]::mouse_event(0x0004, 0, 0, 0, 0)  # left up
Write-Output "clicked at %d,%d"
`, int(x), int(y), int(x), int(y))
		output := t.runPowerShell(psScript)
		return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Clicked at (%d,%d)", int(x), int(y))}, nil

	case "double_click":
		psScript := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, int dwExtraInfo);
}
"@
[Mouse]::SetCursorPos(%d, %d)
Start-Sleep -Milliseconds 50
[Mouse]::mouse_event(0x0002, 0, 0, 0, 0)
Start-Sleep -Milliseconds 30
[Mouse]::mouse_event(0x0004, 0, 0, 0, 0)
Start-Sleep -Milliseconds 50
[Mouse]::mouse_event(0x0002, 0, 0, 0, 0)
Start-Sleep -Milliseconds 30
[Mouse]::mouse_event(0x0004, 0, 0, 0, 0)
Write-Output "double-clicked at %d,%d"
`, int(x), int(y), int(x), int(y))
		output := t.runPowerShell(psScript)
		return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Double-clicked at (%d,%d)", int(x), int(y))}, nil

	case "right_click":
		psScript := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, int dwExtraInfo);
}
"@
[Mouse]::SetCursorPos(%d, %d)
Start-Sleep -Milliseconds 50
[Mouse]::mouse_event(0x0008, 0, 0, 0, 0)  # right down
Start-Sleep -Milliseconds 50
[Mouse]::mouse_event(0x0010, 0, 0, 0, 0)  # right up
Write-Output "right-clicked at %d,%d"
`, int(x), int(y), int(x), int(y))
		output := t.runPowerShell(psScript)
		return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Right-clicked at (%d,%d)", int(x), int(y))}, nil

	default:
		return nil, fmt.Errorf("unknown mouse action: %s (use move|click|double_click|right_click)", sub)
	}
}

// keyboard 键盘操作（type/hotkey）
func (t *ComputerTool) keyboard(ctx context.Context, args map[string]any) (*ToolResult, error) {
	sub, _ := args["sub_action"].(string)
	text, _ := args["text"].(string)
	hotkey, _ := args["hotkey"].(string)

	switch sub {
	case "type":
		if text == "" {
			return nil, fmt.Errorf("text is required for keyboard type")
		}
		// 用 SendKeys 输入
		psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('%s')
Write-Output "typed: %s"
`, escapeSendKeys(text), text)
		output := t.runPowerShell(psScript)
		return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Typed: %s", truncate(text, 50))}, nil

	case "hotkey":
		if hotkey == "" {
			return nil, fmt.Errorf("hotkey is required for keyboard hotkey")
		}
		// 热键映射
		psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('%s')
Write-Output "hotkey: %s"
`, translateHotkey(hotkey), hotkey)
		output := t.runPowerShell(psScript)
		return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Hotkey: %s", hotkey)}, nil

	default:
		return nil, fmt.Errorf("unknown keyboard action: %s (use type|hotkey)", sub)
	}
}

// windowList 列出所有窗口
func (t *ComputerTool) windowList(ctx context.Context) (*ToolResult, error) {
	psScript := `
Get-Process | Where-Object {$_.MainWindowTitle -ne ""} | Select-Object Id, MainWindowTitle | ForEach-Object {
    Write-Output "$($_.Id): $($_.MainWindowTitle)"
}
`
	output := t.runPowerShell(psScript)
	return &ToolResult{Raw: output, Kind: "text", Summary: "Window list retrieved"}, nil
}

// windowFocus 聚焦窗口
func (t *ComputerTool) windowFocus(ctx context.Context, args map[string]any) (*ToolResult, error) {
	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required for window_focus")
	}

	psScript := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win {
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
}
"@
$proc = Get-Process | Where-Object {$_.MainWindowTitle -like "*%s*"} | Select-Object -First 1
if ($proc) {
    [Win]::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
    [Win]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null  # SW_RESTORE
    Write-Output "focused: $($proc.MainWindowTitle)"
} else {
    Write-Output "ERROR: window not found"
}
`, escapePS(title))
	output := t.runPowerShell(psScript)
	if strings.Contains(output, "ERROR") {
		return nil, fmt.Errorf("window_focus failed: %s", output)
	}
	return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Focused: %s", title)}, nil
}

// windowResize 调整窗口大小
func (t *ComputerTool) windowResize(ctx context.Context, args map[string]any) (*ToolResult, error) {
	title, _ := args["title"].(string)
	w, _ := args["width"].(float64)
	h, _ := args["height"].(float64)
	if title == "" || w == 0 || h == 0 {
		return nil, fmt.Errorf("title, width, height are required for window_resize")
	}

	psScript := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win {
    [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr hWnd, int X, int Y, int nWidth, int nHeight, bool bRepaint);
}
"@
$proc = Get-Process | Where-Object {$_.MainWindowTitle -like "*%s*"} | Select-Object -First 1
if ($proc) {
    [Win]::MoveWindow($proc.MainWindowHandle, 0, 0, %d, %d, $true) | Out-Null
    Write-Output "resized: $($proc.MainWindowTitle) to %dx%d"
} else {
    Write-Output "ERROR: window not found"
}
`, escapePS(title), int(w), int(h), int(w), int(h))
	output := t.runPowerShell(psScript)
	if strings.Contains(output, "ERROR") {
		return nil, fmt.Errorf("window_resize failed: %s", output)
	}
	return &ToolResult{Raw: strings.TrimSpace(output), Kind: "text", Summary: fmt.Sprintf("Resized %s to %dx%d", title, int(w), int(h))}, nil
}

func (t *ComputerTool) runPowerShell(script string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	output, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(output))
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func escapeSendKeys(s string) string {
	s = strings.ReplaceAll(s, "{", "{{}")
	s = strings.ReplaceAll(s, "}", "{{}}")
	s = strings.ReplaceAll(s, "+", "{+}")
	s = strings.ReplaceAll(s, "^", "{^}")
	s = strings.ReplaceAll(s, "%", "{%}")
	s = strings.ReplaceAll(s, "~", "{~}")
	s = strings.ReplaceAll(s, "(", "{(}")
	s = strings.ReplaceAll(s, ")", "{)}")
	s = strings.ReplaceAll(s, "[", "{[}")
	s = strings.ReplaceAll(s, "]", "{]}")
	return s
}

func translateHotkey(h string) string {
	// 翻译常见热键到 SendKeys 格式
	// Ctrl+C → ^c, Alt+Tab → %tab, Win+R → ^{esc}r
	h = strings.ToLower(h)
	if strings.Contains(h, "ctrl+c") || strings.Contains(h, "ctrl+c") {
		return "^c"
	}
	if strings.Contains(h, "ctrl+v") {
		return "^v"
	}
	if strings.Contains(h, "ctrl+z") {
		return "^z"
	}
	if strings.Contains(h, "alt+tab") {
		return "%{TAB}"
	}
	if strings.Contains(h, "alt+f4") {
		return "%{F4}"
	}
	if strings.Contains(h, "win+r") {
		return "^{ESC}r"
	}
	if strings.Contains(h, "win+d") {
		return "^(ESC)d"
	}
	// 直接返回原样
	return h
}
