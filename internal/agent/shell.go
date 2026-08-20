package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ShellTool 真实 Shell 工具
type ShellTool struct {
	sandbox bool // 启用路径沙箱
}

// 命令黑名单：危险操作默认拒绝
var commandBlacklist = []struct{ pattern, reason string }{
	{"format ", "禁止格式化磁盘"},
	{"format c:", "禁止格式化系统盘"},
	{"del /f /s /q c:\\", "禁止删除整个C盘"},
	{"del /s", "禁止递归删除文件"},
	{"rmdir /s", "禁止递归删除目录"},
	{"rd /s", "禁止递归删除目录"},
	{"rm -rf /", "禁止递归删除根目录"},
	{"rm -rf c:", "禁止递归删除C盘"},
	{"diskpart", "禁止磁盘分区操作"},
	{"reg delete hk", "禁止删除注册表键"},
	{"shutdown", "禁止关机/重启（可用 windows.notify）"},
	{"restart-computer", "禁止重启计算机"},
	{"taskkill /f /im svchost", "禁止结束关键系统进程"},
	{"wmic process call terminate", "禁止通过WMI终止进程"},
	{"bcdedit", "禁止修改引导配置"},
	{"net user", "禁止修改用户账户"},
	{"net localgroup", "禁止修改用户组"},
	{"iisreset", "禁止重启IIS服务"},
}

// sandboxBlockedPaths 沙箱模式下禁止操作的系统路径前缀
var sandboxBlockedPaths = []string{
	"C:\\Windows",
	"C:\\Program Files",
	"C:\\Program Files (x86)",
	"C:\\ProgramData",
	"$env:SystemRoot",
	"$env:windir",
}

func (t *ShellTool) Name() string        { return "shell.run" }
func (t *ShellTool) Description() string { return "执行 shell 命令（支持超时）" }
func (t *ShellTool) RequiredLevel() int  { return 1 }

// checkCommandBlocked 检查命令是否命中黑名单
// allow_unsafe=false 时返回 blocked
func checkCommandBlocked(command string) (bool, string) {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, b := range commandBlacklist {
		if strings.Contains(lower, b.pattern) {
			return true, b.reason
		}
	}
	return false, ""
}

// checkSandboxCommand 检查 shell 命令是否违反沙箱规则
// isPowerShellCommand 检测命令是否为 PowerShell 命令
func isPowerShellCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))

	// PowerShell cmdlet 模式：Verb-Noun（如 Get-Date, Get-Process）
	psVerbs := []string{
		"get-", "set-", "new-", "remove-", "start-", "stop-", "restart-",
		"invoke-", "select-", "where-", "foreach-", "sort-", "measure-",
		"write-", "read-", "copy-", "move-", "test-", "compare-",
		"import-", "export-", "convert-", "format-", "group-",
	}
	for _, verb := range psVerbs {
		if strings.HasPrefix(lower, verb) {
			return true
		}
	}

	// PowerShell 特有语法
	psPatterns := []string{
		"$env:", "$psversiontable", "get-help", "get-command",
		"get-module", "import-module", "out-file", "out-string",
		"pipe", "|", ">>", "-eq", "-ne", "-gt", "-lt",
	}
	for _, p := range psPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	return false
}

func (t *ShellTool) checkSandboxCommand(command string) error {
	if !t.sandbox {
		return nil
	}

	lower := strings.ToLower(strings.TrimSpace(command))

	// 检查是否操作沙箱外的系统路径
	for _, blocked := range sandboxBlockedPaths {
		if strings.Contains(lower, strings.ToLower(blocked)) {
			return fmt.Errorf("sandbox: 命令涉及系统路径 %s，被安全策略阻止", blocked)
		}
	}

	// 检查是否有文件重定向到系统目录
	if idx := strings.Index(lower, ">"); idx > 0 {
		target := strings.TrimSpace(lower[idx+1:])
		for _, blocked := range sandboxBlockedPaths {
			if strings.Contains(target, strings.ToLower(blocked)) {
				return fmt.Errorf("sandbox: 输出重定向到系统路径 %s，被安全策略阻止", blocked)
			}
		}
	}

	return nil
}

func (t *ShellTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// 沙箱检查：系统路径保护
	if err := t.checkSandboxCommand(command); err != nil {
		return nil, err
	}

	// 命令黑名单（危险操作默认拒绝，除非明确绕过）
	if blocked, reason := checkCommandBlocked(command); blocked {
		return &ToolResult{
			Raw:     fmt.Sprintf("命令被安全策略拦截：%s", reason),
			Kind:    "text",
			Summary: "blocked: " + reason,
		}, fmt.Errorf("command blocked: %s", reason)
	}

	// 命令安全分类：破坏性命令标记需高权限确认
	class := ClassifyCommand(command)
	if class == CommandDestructive {
		return &ToolResult{
			Raw:     fmt.Sprintf("命令被识别为破坏性操作，需在任务中单独确认（分类：%s）", CommandClassLabel(class)),
			Kind:    "text",
			Summary: "destructive: " + command,
		}, fmt.Errorf("destructive command requires confirmation: %s", command)
	}

	// 获取超时
	timeoutSecs := 30
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeoutSecs = int(t)
	}

	// 获取工作目录
	workDir, _ := args["workdir"].(string)

	// 创建带超时的 context
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// 根据平台和命令类型选择 shell
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// 检测是否为 PowerShell 命令（包含 cmdlet 或 PowerShell 特有语法）
		if isPowerShellCommand(command) {
			cmd = exec.CommandContext(timeoutCtx, "powershell", "-NoProfile", "-Command", command)
		} else {
			cmd = exec.CommandContext(timeoutCtx, "cmd.exe", "/C", command)
		}
	} else {
		cmd = exec.CommandContext(timeoutCtx, "sh", "-c", command)
	}

	// 设置工作目录
	if workDir != "" {
		cmd.Dir = workDir
	}

	// 捕获输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 执行
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// 构建结果
	result := &ToolResult{
		Kind: "text",
	}

	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			result.Raw = fmt.Sprintf("Command timed out after %d seconds\nStdout: %s\nStderr: %s",
				timeoutSecs, stdout.String(), stderr.String())
			return result, fmt.Errorf("command timed out: %w", err)
		}
		result.Raw = fmt.Sprintf("Error: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
		result.Summary = fmt.Sprintf("Command failed: %v", err)
		return result, err
	}

	// 成功
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nStderr: " + stderr.String()
	}

	result.Raw = output
	result.Summary = fmt.Sprintf("Command executed successfully in %v", duration.Round(time.Millisecond))

	return result, nil
}
