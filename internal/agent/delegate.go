package agent

import (
	"context"
	"fmt"
	"strings"
)

// 子代理并发上限
const maxSubagents = 4

type subagentCtxKey struct{}

// delegateTool 允许父 Agent 把子目标委托给一个受限的并行 worker 独立执行，
// 并把子代理的最终结论作为工具结果返回。灵感来自 Reasonix 的子代理委派。
type delegateTool struct{ l *Loop }

func (d *delegateTool) Name() string        { return "subagent" }
func (d *delegateTool) Description() string { return "把子目标委托给一个并行子代理独立执行并返回其结论（不能嵌套）" }
func (d *delegateTool) RequiredLevel() int  { return 1 }

func (d *delegateTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	objective, _ := args["objective"].(string)
	if strings.TrimSpace(objective) == "" {
		return nil, fmt.Errorf("objective is required")
	}
	// 禁止嵌套子代理（防无限递归）
	if _, nested := ctx.Value(subagentCtxKey{}).(bool); nested {
		return nil, fmt.Errorf("subagents cannot nest")
	}
	return d.l.runSubagent(ctx, objective)
}

// runSubagent 在并发信号量保护下执行子目标，返回结论文本。
func (l *Loop) runSubagent(ctx context.Context, objective string) (*ToolResult, error) {
	if l.subagentSem == nil {
		l.subagentSem = make(chan struct{}, maxSubagents)
	}
	select {
	case l.subagentSem <- struct{}{}:
		defer func() { <-l.subagentSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	l.subagentRunning.Add(1)
	defer l.subagentRunning.Add(-1)

	// 1. 生成子计划。
	// 可提取出明确命令的简单目标 → 直接 shell.run 单步（更可靠、可重复），
	// 否则走 LLM Planner（复杂/探索性子目标）。
	var plan *Plan
	if cmd := extractCommandFromGoal(objective); cmd != "" && cmd != objective {
		plan = &Plan{Steps: []PlanStep{{
			Description: "执行子目标: " + objective,
			Tool:        "shell.run",
			Args:        map[string]any{"command": cmd, "timeout": 30},
			MaxRetries:  1,
		}}}
	} else if l.planner != nil {
		p, err := l.planner.Plan(ctx, objective, "")
		if err != nil {
			plan = &Plan{Steps: []PlanStep{{
				Description: "执行子目标: " + objective,
				Tool:        "shell.run",
				Args:        map[string]any{"command": extractCommandFromGoal(objective), "timeout": 30},
				MaxRetries:  1,
			}}}
		} else {
			plan = p
		}
	} else {
		cmd := extractCommandFromGoal(objective)
		plan = &Plan{Steps: []PlanStep{{
			Description: "执行子目标: " + objective,
			Tool:        "shell.run",
			Args:        map[string]any{"command": cmd, "timeout": 30},
			MaxRetries:  1,
		}}}
	}

	// 2. 执行各步骤（用父 Agent 的工具集；结果不透传到主会话）
	subCtx := context.WithValue(ctx, subagentCtxKey{}, true)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[子代理完成 %d 步]\n", len(plan.Steps)))
	for i, step := range plan.Steps {
		select {
		case <-subCtx.Done():
			return nil, subCtx.Err()
		default:
		}
		// 权限闸门：子代理无法发起交互确认，策略要求确认/拒绝的步骤一律拦截。
		// 此前直接 RunTool 会绕过主循环的确认流（computer/mcp/fs:delete 可静默执行）。
		if d := l.checkToolPermission(subCtx, step.Tool, step.Args); !d.Allowed {
			sb.WriteString(fmt.Sprintf("[步骤%d %s: 被权限策略拒绝（%s）——如需执行请在主计划中申请确认]\n",
				i+1, step.Tool, d.Reason))
			continue
		}
		res, err := l.RunTool(subCtx, step.Tool, step.Args)
		if err != nil {
			sb.WriteString(fmt.Sprintf("[步骤%d %s: 失败 %v]\n", i+1, step.Tool, err))
			continue
		}
		// 优先输出原始结果（含命令实际输出，可验证），过长才截断
		summary := truncate(res.Raw, 300)
		if summary == "" {
			summary = res.Summary
		}
		sb.WriteString(fmt.Sprintf("[步骤%d %s] %s\n", i+1, step.Tool, summary))
	}
	out := sb.String()
	return &ToolResult{
		Raw:     out,
		Kind:    "text",
		Summary: fmt.Sprintf("子代理执行完成（%d 步）", len(plan.Steps)),
	}, nil
}
