package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"agent/internal/auth"
	"agent/internal/task"
)

// waitForConfirmation 等待人工确认
func (l *Loop) waitForConfirmation(ctx context.Context, t *task.Task, step *PlanStep, seq int, decision auth.PermissionDecision) (bool, error) {
	conf := &auth.Confirmation{
		ID:          fmt.Sprintf("c-%s-%d", t.ID, seq),
		TaskID:      t.ID,
		StepSeq:     seq,
		Operation:   step.Tool,
		Tool:        step.Tool,
		RiskLevel:   decision.Level,
		Reason:      decision.Reason,
		Status:      "pending",
		Requester:   "agent",
		CreatedAt:   time.Now(),
		TimeoutSecs: 60,
	}

	argsJSON, _ := json.Marshal(step.Args)
	conf.ArgsJSON = string(argsJSON)

	if err := l.confirmStore.Create(conf); err != nil {
		return false, fmt.Errorf("create confirmation: %w", err)
	}

	// 轮询等待确认结果
	timeout := time.Duration(conf.TimeoutSecs) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(1 * time.Second):
		}

		// 检查确认状态
		updated, err := l.confirmStore.Get(conf.ID)
		if err != nil {
			continue
		}
		if updated == nil {
			continue
		}

		switch updated.Status {
		case "approved":
			return true, nil
		case "rejected", "timeout":
			return false, fmt.Errorf("confirmation %s", updated.Status)
		}
	}

	// 超时，标记超时
	l.confirmStore.Reject(conf.ID, "system", "timeout")
	return false, fmt.Errorf("confirmation timeout")
}

// executeStep 执行单个步骤，返回工具结果
func (l *Loop) executeStep(ctx context.Context, t *task.Task, step *PlanStep, seq int) (*ToolResult, error) {
	tool, ok := l.tools[step.Tool]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", step.Tool)
	}

	// 审计日志：工具调用
	l.auditLogRecord(t.ID, "tool.call", fmt.Sprintf("step=%d tool=%s", seq, step.Tool), "")

	// 记录执行的参数
	l.logger.Infof("executeStep: tool=%s args=%v", step.Tool, step.Args)

	result, err := tool.Execute(ctx, step.Args)
	if err != nil {
		// 审计日志：工具失败
		l.auditLogRecord(t.ID, "tool.failed", fmt.Sprintf("step=%d tool=%s error=%v", seq, step.Tool, err), "error")
		return result, fmt.Errorf("tool %s execute: %w", step.Tool, err)
	}

	// 审计日志：工具成功
	l.auditLogRecord(t.ID, "tool.success", fmt.Sprintf("step=%d tool=%s", seq, step.Tool), "ok")

	now := time.Now()
	stepRecord := &task.Step{
		ID:          fmt.Sprintf("s-%s-%d", t.ID, seq),
		TaskID:      t.ID,
		Seq:         seq,
		Description: step.Description,
		Status:      "completed",
		Tool:        step.Tool,
		Result:      result.Raw,
		Summary:     result.Summary,
		StartedAt:   &now,
		FinishedAt:  &now,
	}

	if err := l.store.SaveStep(stepRecord); err != nil {
		l.logger.Warnf("failed to save step: %v", err)
	}
	return result, nil
}

// truncateStepResult 截断步骤结果（供 SSE 事件推送，避免大输出撑爆事件流）
func truncateStepResult(s string) string {
	if len(s) <= 500 {
		return s
	}
	return s[:500] + "\n...（结果过长已截断）"
}

// auditLogRecord 记录审计日志（带错误处理）
func (l *Loop) auditLogRecord(target, action, detail, result string) {
	if l.auditLog == nil {
		return
	}
	_, err := l.auditLog.Exec(
		`INSERT INTO audit_logs (ts, actor, action, target, detail, result) VALUES (?, ?, ?, ?, ?, ?)`,
		time.Now(), "agent", action, target, detail, result,
	)
	if err != nil {
		l.logger.Warnf("failed to write audit log: %v", err)
	}
}
