package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent/internal/auth"
	"agent/internal/config"
	"agent/internal/task"
)

// checkToolPermission 用 PermissionEngine 策略表判定工具权限。
// 键为 tool[:action]（args.action 存在时拼上，覆盖 fs.delete/windows.launch 等
// action 分发工具）；策略未命中默认 fail-closed（L2 需确认）。
// principal 缺失（后台任务）按 Level1Normal 处理。
func (l *Loop) checkToolPermission(ctx context.Context, tool string, args map[string]any) auth.PermissionDecision {
	if l.permEngine == nil {
		return auth.PermissionDecision{Allowed: true, Level: auth.Level0ReadOnly}
	}
	permKey := tool
	if action, ok := args["action"].(string); ok && action != "" {
		permKey = tool + ":" + action
	}
	userLevel := auth.Level1Normal
	if p := auth.PrincipalFromContext(ctx); p != nil {
		userLevel = auth.PermissionLevel(p.PermLevel)
	}
	return l.permEngine.Check(strings.ReplaceAll(permKey, ".", ":"), userLevel)
}

// EffectiveRequiredLevel 计算工具+action 的有效权限下限。
// 优先使用工具上的 ActionRequiredLevel(action)；否则回退 RequiredLevel()。
// 对已知高危工具名再做强制下限，防止元数据被改松。
func EffectiveRequiredLevel(tool Tool, toolName string, args map[string]any) int {
	lvl := 0
	if tool != nil {
		lvl = tool.RequiredLevel()
		type actionLeveler interface {
			ActionRequiredLevel(action string) int
		}
		if al, ok := tool.(actionLeveler); ok {
			if action, aok := args["action"].(string); aok && action != "" {
				lvl = al.ActionRequiredLevel(action)
			}
		}
	}
	action := ""
	if a, ok := args["action"].(string); ok {
		action = strings.ToLower(a)
	}
	switch toolName {
	case "shell.run":
		if lvl < 2 {
			lvl = 2
		}
	case "computer", "mcp":
		if lvl < 2 {
			lvl = 2
		}
	case "fs":
		switch action {
		case "delete", "organize", "write", "mkdir":
			if lvl < 2 {
				lvl = 2
			}
		case "read", "list", "exists":
			// 保持 L0
		}
	}
	return lvl
}

// RunTool 运行指定工具（手动 /v1/tools/run 与 Chat tool_call 路径）。
// 与任务路径 Run 共用同一道防线：scope（有 principal 时）+ PermissionEngine + 确认流 + shell 审计。
// 安全硬化：
// - 有 principal 时强制 tools:run scope（空 scopes fail-closed）
// - 有 principal 时校验 EffectiveRequiredLevel ≤ PermLevel
// - shell.run：argv-only 档直接拒绝；strict 档每一次强制确认；审计必写
// - 策略 NeedConfirm 与破坏性命令分类合并为 **一次** 确认；确认服务不可用时 fail-closed
func (l *Loop) RunTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	tool, ok := l.tools[name]
	if !ok {
		// Sprint3：默认未注册的高危面给出明确拒绝原因，而非含糊的 not found
		if name == "computer" && !config.ComputerUseEnabled() {
			l.auditLogRecord(name, "feature.disabled", "computer_use_enabled=false", "denied")
			return nil, fmt.Errorf("computer tool disabled by default; set permissions.computer_use_enabled=true to opt in")
		}
		if (name == "mcp" || strings.HasPrefix(name, "mcp.")) && !config.MCPEnabled() {
			l.auditLogRecord(name, "feature.disabled", "mcp.enabled=false", "denied")
			return nil, fmt.Errorf("mcp disabled by default; set mcp.enabled=true to opt in")
		}
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Scope + perm：有 principal 时必须显式通过；无 principal（任务/内部路径）仍走策略门
	if p := auth.PrincipalFromContext(ctx); p != nil {
		if !p.HasScope(auth.ScopeToolsRun) {
			l.auditLogRecord(name, "authz.denied", fmt.Sprintf("missing scope %s", auth.ScopeToolsRun), "forbidden")
			return nil, fmt.Errorf("scope %q required but not granted", auth.ScopeToolsRun)
		}
		need := EffectiveRequiredLevel(tool, name, args)
		if need > p.PermLevel {
			l.auditLogRecord(name, "authz.denied",
				fmt.Sprintf("perm: need L%d have L%d", need, p.PermLevel), "forbidden")
			return nil, fmt.Errorf("permission denied: tool %s requires level %d, token has level %d", name, need, p.PermLevel)
		}
	}

	// Shell profile：默认档永不提供「无 opt-in 的自由字符串 shell」
	if name == "shell.run" {
		profile := config.GetShellProfile()
		if profile == config.ShellProfileArgvOnly {
			l.auditLogRecord(name, "shell.blocked", "shell_profile=argv-only", "denied")
			return nil, fmt.Errorf("shell.run disabled by shell_profile=argv-only; use exec.argv allowlist tool")
		}
	}

	destructive := false
	if name == "shell.run" {
		if cmd, ok := args["command"].(string); ok && ClassifyCommand(cmd) == CommandDestructive {
			destructive = true
		}
	}

	var confirmDecision *auth.PermissionDecision

	if l.permEngine != nil {
		decision := l.checkToolPermission(ctx, name, args)
		if !decision.Allowed && !decision.NeedConfirm {
			if name == "shell.run" {
				l.auditLogRecord(name, "shell.denied", decision.Reason, "denied")
			}
			return nil, fmt.Errorf("denied by policy: %s", decision.Reason)
		}
		if decision.NeedConfirm {
			d := decision
			confirmDecision = &d
		}
	}

	// strict 档：每一次 shell.run 都必须进确认流（即使命令被分类为只读）
	if name == "shell.run" && !config.StringShellAllowed() && !ShellApproved(ctx) {
		if confirmDecision == nil {
			confirmDecision = &auth.PermissionDecision{
				Allowed:     false,
				NeedConfirm: true,
				Level:       auth.Level2Dangerous,
				Reason:      "shell.run 在 shell_profile=strict 下每次均需人工确认",
			}
		} else if !confirmDecision.NeedConfirm {
			confirmDecision.NeedConfirm = true
			confirmDecision.Allowed = false
			if confirmDecision.Level < auth.Level2Dangerous {
				confirmDecision.Level = auth.Level2Dangerous
			}
		}
	}

	// 破坏性命令：若尚未进入确认，则强制进入；若已在确认，提升说明
	if destructive {
		if confirmDecision == nil {
			confirmDecision = &auth.PermissionDecision{
				Allowed:     false,
				NeedConfirm: true,
				Level:       auth.Level2Dangerous,
				Reason:      fmt.Sprintf("破坏性命令需要确认（分类：%s）", CommandClassLabel(CommandDestructive)),
			}
		} else {
			confirmDecision.Reason = confirmDecision.Reason + "; " +
				fmt.Sprintf("破坏性命令分类：%s", CommandClassLabel(CommandDestructive))
			if confirmDecision.Level < auth.Level2Dangerous {
				confirmDecision.Level = auth.Level2Dangerous
			}
			confirmDecision.NeedConfirm = true
		}
	}

	if confirmDecision != nil && confirmDecision.NeedConfirm && !ShellApproved(ctx) && !DestructiveApproved(ctx) {
		approved, err := l.confirmAdhoc(ctx, name, args, *confirmDecision)
		if err != nil {
			if name == "shell.run" {
				l.auditLogRecord(name, "shell.confirm_error", err.Error(), "error")
			}
			return nil, fmt.Errorf("confirmation error: %w", err)
		}
		if !approved {
			if name == "shell.run" {
				l.auditLogRecord(name, "shell.denied", "user confirmation denied", "denied")
			}
			return nil, fmt.Errorf("denied by user confirmation")
		}
		if name == "shell.run" || destructive {
			ctx = WithShellApproval(ctx)
			if destructive {
				ctx = WithDestructiveApproval(ctx)
			}
		}
	}

	if name == "shell.run" {
		l.auditLogRecord(name, "shell.run",
			fmt.Sprintf("profile=%s destructive=%v", config.GetShellProfile(), destructive), "attempt")
	}

	result, err := tool.Execute(ctx, args)
	if name == "shell.run" {
		if err != nil {
			l.auditLogRecord(name, "shell.failed", fmt.Sprintf("%v", err), "error")
		} else {
			l.auditLogRecord(name, "shell.success", "ok", "ok")
		}
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

// confirmAdhoc 为非任务路径的工具调用创建确认流（合成 adhoc 任务上下文），
// 确认项与任务路径一样写入 confirmStore，出现在 dashboard 的待确认列表中。
func (l *Loop) confirmAdhoc(ctx context.Context, toolName string, args map[string]any, decision auth.PermissionDecision) (bool, error) {
	if l.confirmStore == nil {
		return false, fmt.Errorf("需要人工确认但确认服务不可用，按拒绝处理（%s）", decision.Reason)
	}
	taskID := fmt.Sprintf("adhoc-%s-%d", strings.NewReplacer(":", "-", ".", "-").Replace(toolName), time.Now().UnixNano()%1_000_000_000)
	t := &task.Task{ID: taskID, Goal: "手动/Chat 工具调用"}
	step := PlanStep{Tool: toolName, Args: args, Description: "手动/Chat 工具调用"}
	return l.waitForConfirmation(ctx, t, &step, 0, decision)
}
