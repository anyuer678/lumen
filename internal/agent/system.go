package agent

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// SystemTool 系统工具（进程/服务/网络/Git）
type SystemTool struct{}

// NewSystemTool 创建系统工具
func NewSystemTool() *SystemTool {
	return &SystemTool{}
}

func (t *SystemTool) Name() string        { return "system" }
func (t *SystemTool) Description() string { return "系统操作（进程/服务/网络/Git）" }
func (t *SystemTool) RequiredLevel() int  { return 1 }

func (t *SystemTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "processes":
		return t.processes(ctx)
	case "services":
		return t.services(ctx)
	case "network":
		target, _ := args["target"].(string)
		return t.network(ctx, target)
	case "git", "git_status":
		return t.gitStatus(ctx)
	case "disk":
		return t.disk(ctx)
	default:
		return nil, fmt.Errorf("unknown system action: %s", action)
	}
}

func (t *SystemTool) processes(ctx context.Context) (*ToolResult, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "tasklist")
	} else {
		cmd = exec.CommandContext(ctx, "ps", "aux")
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "进程列表"}, nil
}

func (t *SystemTool) services(ctx context.Context) (*ToolResult, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "net", "start")
	} else {
		cmd = exec.CommandContext(ctx, "systemctl", "list-units", "--type=service")
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "服务列表"}, nil
}

func (t *SystemTool) network(ctx context.Context, target string) (*ToolResult, error) {
	if target == "" {
		target = "baidu.com"
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", "4", target)
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", "4", target)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ping %s: %w", target, err)
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: fmt.Sprintf("Ping %s", target)}, nil
}

func (t *SystemTool) gitStatus(ctx context.Context) (*ToolResult, error) {
	cmd := exec.CommandContext(ctx, "git", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "Git 仓库状态"}, nil
}

func (t *SystemTool) disk(ctx context.Context) (*ToolResult, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "wmic", "logicaldisk", "get", "size,freespace,caption")
	} else {
		cmd = exec.CommandContext(ctx, "df", "-h")
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("check disk: %w", err)
	}
	return &ToolResult{Raw: string(output), Kind: "text", Summary: "磁盘空间"}, nil
}
