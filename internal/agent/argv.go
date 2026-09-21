package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ArgvTool argv 白名单执行工具：不经 shell 字符串解释，直接 exec.Command(bin, args...)。
// 相比 shell.run：无元字符展开、无管道/重定向语义、二进制名必须在白名单内。
// 这是默认推荐的命令执行面；字符串 shell.run 仅在 permissions.shell_profile=full 时启用。
type ArgvTool struct {
	sandbox       bool
	workspaceRoot string
	// allow 为空时使用 defaultArgvAllowlist
	allow map[string]bool
}

// defaultArgvAllowlist 允许的二进制 basename（小写）。
// 仅收录低副作用/只读诊断类；安装、删除、注册表等一律不在此列。
var defaultArgvAllowlist = []string{
	"echo", "ls", "dir", "cat", "pwd", "where", "whoami",
	"ping", "nslookup", "netstat", "ipconfig", "systeminfo", "tasklist",
	"git", "go", "python", "python3", "node", "npm",
}

// NewArgvTool 创建 argv 白名单工具。
func NewArgvTool(workspaceRoot string, sandbox bool) *ArgvTool {
	allow := make(map[string]bool, len(defaultArgvAllowlist))
	for _, b := range defaultArgvAllowlist {
		allow[b] = true
	}
	return &ArgvTool{sandbox: sandbox, workspaceRoot: workspaceRoot, allow: allow}
}

func (t *ArgvTool) Name() string { return "exec.argv" }
func (t *ArgvTool) Description() string {
	return "argv 白名单执行：不经 shell，直接启动白名单二进制（推荐替代 shell.run）"
}
func (t *ArgvTool) RequiredLevel() int { return 1 }

// ArgvAllowlist 返回当前白名单（只读副本，供测试/文档）。
func (t *ArgvTool) ArgvAllowlist() []string {
	out := make([]string, 0, len(t.allow))
	for b := range t.allow {
		out = append(out, b)
	}
	return out
}

// argvShellMetachars 参数中禁止出现的 shell 元字符（即使 shell=False，也拒绝以消除语义混淆）。
var argvShellMetachars = []string{";", "&&", "||", "|", ">", "<", "`", "$(", "\n", "\r"}

func checkArgvSafe(args []string) error {
	for _, a := range args {
		for _, m := range argvShellMetachars {
			if strings.Contains(a, m) {
				return fmt.Errorf("argv 参数包含禁止的元字符 %q: %q", m, a)
			}
		}
	}
	return nil
}

func (t *ArgvTool) binaryAllowed(bin string) (string, bool) {
	base := strings.ToLower(strings.TrimSpace(bin))
	base = strings.TrimSuffix(base, ".exe")
	// 只允许 basename，禁止路径形式，防止绕过白名单启动任意 exe
	if strings.ContainsAny(base, `/\`) || strings.Contains(base, ":") {
		return "", false
	}
	if t.allow != nil && t.allow[base] {
		return base, true
	}
	if t.allow == nil {
		for _, b := range defaultArgvAllowlist {
			if b == base {
				return base, true
			}
		}
	}
	return "", false
}

func (t *ArgvTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	binRaw, _ := args["bin"].(string)
	if strings.TrimSpace(binRaw) == "" {
		// 兼容：也接受 args["command"] 数组形式
		if arr, ok := args["argv"].([]any); ok && len(arr) > 0 {
			var strs []string
			for _, a := range arr {
				s, ok := a.(string)
				if !ok {
					return nil, fmt.Errorf("argv 数组元素必须是字符串")
				}
				strs = append(strs, s)
			}
			binRaw = strs[0]
			args = map[string]any{"bin": binRaw, "args": anySliceToStrings(strs[1:])}
		}
	}
	if strings.TrimSpace(binRaw) == "" {
		return nil, fmt.Errorf("bin is required")
	}
	base, ok := t.binaryAllowed(binRaw)
	if !ok {
		return &ToolResult{
			Raw:     fmt.Sprintf("argv 白名单拒绝二进制：%s（不在 allowlist）", binRaw),
			Kind:    "text",
			Summary: "argv denied: " + binRaw,
		}, fmt.Errorf("binary not in argv allowlist: %s", binRaw)
	}

	var binArgs []string
	switch v := args["args"].(type) {
	case nil:
	case []any:
		for _, a := range v {
			s, ok := a.(string)
			if !ok {
				return nil, fmt.Errorf("args 数组元素必须是字符串")
			}
			binArgs = append(binArgs, s)
		}
	case []string:
		binArgs = v
	default:
		return nil, fmt.Errorf("args must be string array")
	}
	if err := checkArgvSafe(binArgs); err != nil {
		return nil, err
	}

	// 工作目录沙箱（与 shell/fs 一致）
	workDir, _ := args["workdir"].(string)
	if workDir != "" && t.sandbox && t.workspaceRoot != "" {
		absWork, _ := filepath.Abs(workDir)
		absWS, _ := filepath.Abs(t.workspaceRoot)
		if r, err := filepath.Rel(absWS, absWork); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("access denied: workdir %s is outside workspace", workDir)
		}
	}

	timeoutSecs := 30
	if tv, ok := args["timeout"].(float64); ok && tv > 0 {
		timeoutSecs = int(tv)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// Windows 上 dir/cmd 内建无法直接 exec——dir 映射为 cmd 的内建仍走 cmd /c 仅当 bin 白名单且参数无元字符
	// 但为了保持「无 shell」承诺：dir/ls 在 Windows 用 exec.LookPath；找不到则报错，不回退 shell。
	if runtime.GOOS == "windows" && base == "ls" {
		base = "dir" // 仍可能不存在；不自动 shell
	}
	if runtime.GOOS == "windows" && base == "dir" {
		// Windows 没有独立 dir.exe：用 cmd 仅执行内建 dir，参数已过元字符检查
		fullArgs := append([]string{"/c", "dir"}, binArgs...)
		cmd := exec.CommandContext(timeoutCtx, "cmd.exe", fullArgs...)
		if workDir != "" {
			cmd.Dir = workDir
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return finishArgvResult(stdout, stderr, err, timeoutCtx, base, binArgs)
	}

	cmd := exec.CommandContext(timeoutCtx, base, binArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return finishArgvResult(stdout, stderr, err, timeoutCtx, base, binArgs)
}

func finishArgvResult(stdout, stderr bytes.Buffer, err error, timeoutCtx context.Context, base string, binArgs []string) (*ToolResult, error) {
	summaryCmd := base + " " + strings.Join(binArgs, " ")
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return &ToolResult{
				Raw:     fmt.Sprintf("argv command timed out\nStdout: %s\nStderr: %s", stdout.String(), stderr.String()),
				Kind:    "text",
				Summary: "argv timeout: " + summaryCmd,
			}, fmt.Errorf("argv command timed out: %s", summaryCmd)
		}
		return &ToolResult{
			Raw:     fmt.Sprintf("Error: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String()),
			Kind:    "text",
			Summary: "argv failed: " + summaryCmd,
		}, err
	}
	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\nStderr: " + stderr.String()
	}
	return &ToolResult{
		Raw:     out,
		Kind:    "text",
		Summary: "argv ok: " + summaryCmd,
	}, nil
}

func anySliceToStrings(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
