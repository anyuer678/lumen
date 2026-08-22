package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// WindowsTool Windows 深层控制工具
type WindowsTool struct{}

// NewWindowsTool 创建 Windows 工具
func NewWindowsTool() *WindowsTool {
	return &WindowsTool{}
}

func (t *WindowsTool) Name() string        { return "windows" }
func (t *WindowsTool) Description() string { return "Windows 控制（launch/window/notify/env/clipboard/app_list/powershell/registry/keyboard）" }
func (t *WindowsTool) RequiredLevel() int  { return 1 }

func (t *WindowsTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("windows tool only available on Windows")
	}

	action, _ := args["action"].(string)
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "powershell":
		return t.powershell(ctx, args)
	case "registry":
		return t.registry(ctx, args)
	case "env":
		subAction, _ := args["sub_action"].(string)
		return t.environment(ctx, subAction, args)
	case "clipboard":
		return t.clipboard(ctx, args)
	case "notify":
		return t.notify(ctx, args)
	case "app_list":
		return t.appList(ctx)
	case "launch":
		return t.launch(ctx, args)
	case "window_list":
		return t.windowList(ctx)
	case "window_focus":
		return t.windowFocus(ctx, args)
	case "keyboard":
		return t.keyboard(ctx, args)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// launch 启动程序
func (t *WindowsTool) launch(ctx context.Context, args map[string]any) (*ToolResult, error) {
	app, _ := args["app"].(string)
	if app == "" {
		return nil, fmt.Errorf("app is required for launch")
	}

	var cmd *exec.Cmd
	if strings.HasPrefix(app, "http") {
		cmd = exec.CommandContext(ctx, "cmd", "/C", "start", app)
	} else {
		cmd = exec.CommandContext(ctx, "cmd", "/C", "start", "", app)
	}
	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("launch %s: %w", app, err)
	}
	return &ToolResult{
		Raw:     fmt.Sprintf("已启动程序: %s", app),
		Kind:    "text",
		Summary: fmt.Sprintf("Launched %s", app),
	}, nil
}

// windowList 列出窗口
func (t *WindowsTool) windowList(ctx context.Context) (*ToolResult, error) {
	script := "Get-Process | Where-Object { $_.MainWindowTitle } | Select-Object ProcessName, MainWindowTitle, Id | Format-Table -AutoSize | Out-String"
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "窗口列表"}, nil
}

// windowFocus 激活匹配标题的窗口
func (t *WindowsTool) windowFocus(ctx context.Context, args map[string]any) (*ToolResult, error) {
	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required for window_focus")
	}
	// 转义单引号，防 PowerShell 注入
	titleEsc := strings.ReplaceAll(title, "'", "''")
	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
}
"@
$p = Get-Process | Where-Object { $_.MainWindowTitle -like '*%s*' } | Select-Object -First 1
if ($p) { [Win32]::SetForegroundWindow($p.MainWindowHandle); Write-Output ('Focused: ' + $p.MainWindowTitle) } else { Write-Output 'Not found' }`, titleEsc)
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "窗口激活"}, nil
}

// keyboard 模拟按键（发送到活动窗口）
func (t *WindowsTool) keyboard(ctx context.Context, args map[string]any) (*ToolResult, error) {
	text, _ := args["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("text is required for keyboard")
	}
	// 转义单引号
	text = strings.ReplaceAll(text, "'", "''")
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('%s')`, text)
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("keyboard: %w", err)
	}
	return &ToolResult{Raw: "按键已发送", Kind: "text", Summary: "Sent keys"}, nil
}

func (t *WindowsTool) powershell(ctx context.Context, args map[string]any) (*ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("command is required for powershell")
	}

	// 安全校验：拒绝危险命令（复用 shell.go 黑名单 + 破坏性分类），
	// 防止通过 powershell 动作绕过 shell 工具的安全限制
	if blocked, reason := checkCommandBlocked(command); blocked {
		return &ToolResult{
			Raw:     fmt.Sprintf("命令被安全策略拦截：%s", reason),
			Kind:    "text",
			Summary: "blocked: " + reason,
		}, fmt.Errorf("command blocked: %s", reason)
	}
	// 破坏性命令需更高权限（L2），此处直接拒绝并要求走确认流程
	if ClassifyCommand(command) == CommandDestructive {
		return &ToolResult{
			Raw:     fmt.Sprintf("命令被识别为破坏性操作，已拒绝（分类：%s）", CommandClassLabel(CommandDestructive)),
			Kind:    "text",
			Summary: "destructive blocked",
		}, fmt.Errorf("destructive command requires confirmation")
	}

	timeout := 30
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-Command", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nStderr: " + stderr.String()
	}

	if err != nil {
		return &ToolResult{
			Raw:     output,
			Kind:    "text",
			Summary: fmt.Sprintf("PowerShell failed: %v (%v)", err, duration),
		}, err
	}

	return &ToolResult{
		Raw:     output,
		Kind:    "text",
		Summary: fmt.Sprintf("PowerShell executed in %v", duration.Round(time.Millisecond)),
	}, nil
}

func (t *WindowsTool) registry(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	key, _ := args["key"].(string)

	switch action {
	case "read", "list":
		cmd := exec.CommandContext(ctx, "reg", "query", key)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", action, err)
		}
		return &ToolResult{Raw: string(output), Kind: "text", Summary: "Registry query"}, nil
	default:
		return nil, fmt.Errorf("unknown registry action: %s", action)
	}
}

func (t *WindowsTool) environment(ctx context.Context, subAction string, args map[string]any) (*ToolResult, error) {
	name, _ := args["name"].(string)

	// 当有 name 但没 sub_action 时，默认 get
	if subAction == "" && name != "" {
		subAction = "get"
	}
	if subAction == "" {
		subAction = "list"
	}

	switch subAction {
	case "list":
		cmd := exec.CommandContext(ctx, "cmd", "/C", "set")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		return &ToolResult{Raw: string(output), Kind: "text", Summary: "Environment variables"}, nil

	case "get":
		if name == "" {
			return nil, fmt.Errorf("name is required for get")
		}
		cmd := exec.CommandContext(ctx, "cmd", "/C", "echo %"+name+"%")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		return &ToolResult{Raw: strings.TrimSpace(string(output)), Kind: "text", Summary: "Environment variable"}, nil

	case "set":
		value, _ := args["value"].(string)
		if name == "" || value == "" {
			return nil, fmt.Errorf("name and value are required for set")
		}
		cmd := exec.CommandContext(ctx, "cmd", "/C", "set", name+"="+value)
		_ = cmd.Run()
		return &ToolResult{Raw: fmt.Sprintf("Set %s=%s", name, value), Kind: "text", Summary: "Environment variable set"}, nil

	default:
		return nil, fmt.Errorf("unknown env action: %s", subAction)
	}
}

func (t *WindowsTool) clipboard(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	text, _ := args["text"].(string)
	command, _ := args["command"].(string)

	// 兼容：LLM 可能用 command 传内容或操作意图
	if text == "" && command != "" {
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "get", "read":
			action = "read"
		case "set", "write":
			action = "write"
		default:
			text = command
		}
	}

	// 当 action 不是 read/write 时（如 "clipboard"），根据是否有 text 决定默认行为
	if action != "read" && action != "write" {
		if text != "" {
			action = "write"
		} else {
			action = "read"
		}
	}

	switch action {
	case "read":
		cmd := exec.CommandContext(ctx, "powershell", "-Command", "Get-Clipboard")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		return &ToolResult{Raw: strings.TrimSpace(string(output)), Kind: "text", Summary: "Clipboard content"}, nil

	case "write":
		if text == "" {
			return nil, fmt.Errorf("text is required for write")
		}
		// 安全写入：用 base64 编码避免命令注入
		encoded := base64Encode(text)
		cmd := exec.CommandContext(ctx, "powershell", "-Command", fmt.Sprintf("[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s')) | Set-Clipboard -InputObject", encoded))
		_ = cmd.Run()
		return &ToolResult{Raw: "Clipboard updated", Kind: "text", Summary: "Clipboard written"}, nil

	default:
		return nil, fmt.Errorf("unknown clipboard action: %s", action)
	}
}

func (t *WindowsTool) notify(ctx context.Context, args map[string]any) (*ToolResult, error) {
	title, _ := args["title"].(string)
	body, _ := args["body"].(string)
	if title == "" {
		title = "Agent"
	}
	// 转义单引号，防 PowerShell 注入
	titleEsc := strings.ReplaceAll(title, "'", "''")
	bodyEsc := strings.ReplaceAll(body, "'", "''")

	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName('text')
$textNodes.Item(0).AppendChild($template.CreateTextNode('%s'))
$textNodes.Item(1).AppendChild($template.CreateTextNode('%s'))
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Agent').Show($toast)
`, titleEsc, bodyEsc)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	_ = cmd.Run()
	return &ToolResult{Raw: "Notification sent", Kind: "text", Summary: "Desktop notification"}, nil
}

func (t *WindowsTool) appList(ctx context.Context) (*ToolResult, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-Command", "Get-ItemProperty HKLM:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\* | Select-Object DisplayName, DisplayVersion | Sort-Object DisplayName | ConvertTo-Json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "Installed applications"}, nil
}

// base64Encode 将字符串 base64 编码，避免 PowerShell 命令注入。
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
