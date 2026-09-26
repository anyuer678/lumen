package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent/internal/auth"
	"agent/internal/llm"
	"agent/internal/memory"
	"agent/internal/task"
)

// Run 执行任务
func (l *Loop) Run(ctx context.Context, t *task.Task) error {
	l.logger.Infof("agent loop: starting task %s", t.ID)
	startTime := time.Now()

	// 初始化追踪记录器
	trace := NewTraceRecorder(l.traceDir, t.ID)

	var plan *Plan
	var err error

	// 检索相关长期记忆 + 知识库
	memoryContext := ""
	if l.memoryStore != nil {
		if mems, memErr := l.memoryStore.Search(t.Goal, 5); memErr == nil && len(mems) > 0 {
			var parts []string
			for _, m := range mems {
				if m.Confirmed {
					parts = append(parts, fmt.Sprintf("[记忆] %s", m.Content))
				}
			}
			memoryContext = stringListJoin(parts, "\n")
			if memoryContext != "" {
				l.logger.Infof("agent loop: task %s retrieved %d memories", t.ID, len(parts))
			}
		}
	}

	// 检索知识库，并入上下文
	kbContext := l.retrieveKnowledge(t.Goal)
	if kbContext != "" {
		if memoryContext != "" {
			memoryContext += "\n" + kbContext
		} else {
			memoryContext = kbContext
		}
		l.logger.Infof("agent loop: task %s retrieved knowledge for %s", t.ID, t.Goal)
	}

	// 使用上下文管理器裁剪记忆上下文（防止注入过多内容导致 Planner 超预算）
	if memoryContext != "" {
		_, memoryContext = l.ctxMgr.FitForPlanner(t.Goal, memoryContext, nil)
	}

	// 简化模式下也把知识输出到结果
	if l.planner == nil && kbContext != "" {
		l.store.SetResult(t.ID, "已检索相关用户知识。", "")
	}

	// 根据是否有 LLM provider 决定使用哪种模式
	// 断点恢复：若任务已有 checkpoint，加载已保存的计划并从断点继续
	var resumeStep int
	if storedPlan, perr := l.store.GetPlan(t.ID); perr == nil && len(storedPlan) > 0 {
		if p, derr := DecodePlan(storedPlan); derr == nil && len(p.Steps) > 0 {
			if rs, cerr := l.store.LoadCheckpoint(t.ID); cerr == nil && rs > 0 && rs < len(p.Steps) {
				resumeStep = rs
				plan = p
				l.logger.Infof("agent loop: task %s resuming from checkpoint step %d/%d", t.ID, resumeStep, len(p.Steps))
				// 预填 progress
				t.Progress = float64(resumeStep) / float64(len(p.Steps)) * 100
				t.CurrentStep = resumeStep
			}
		}
	}

	// 若无断点，则重新生成计划
	if plan == nil && l.planner != nil {
		// 1. 用 LLM 生成计划（注入记忆）
		endTrace := trace.Start(StagePlanner, t.Goal)
		plan, err = l.planner.Plan(ctx, t.Goal, memoryContext)
		if err != nil {
			endTrace(false, "", err)
			// 降级：LLM 计划失败时，对可提取命令的目标用简化单步，避免整体失败
			if cmd := extractCommandFromGoal(t.Goal); cmd != "" && cmd != t.Goal {
				l.logger.Warnf("agent loop: task %s LLM plan failed (%v), falling back to single shell step", t.ID, err)
				plan = &Plan{Steps: []PlanStep{{
					Description: "执行任务: " + t.Goal,
					Tool:        "shell.run",
					Args:        map[string]any{"command": cmd, "timeout": 30},
					MaxRetries:  2,
				}}}
			} else {
				l.emitStep("task.failed", map[string]any{"task_id": t.ID, "error": "plan: " + err.Error()})
				// 通过 EventBus 发射任务失败事件（触发主动恢复）
				if l.eventBus != nil {
					l.eventBus.EmitAsync(Event{
						Source:   "agent-loop",
						Type:     EventTaskFailed,
						Payload:  t.ID,
						Priority: 5,
					})
				}
				return fmt.Errorf("plan: %w", err)
			}
		} else {
			endTrace(true, fmt.Sprintf("%d steps", len(plan.Steps)), nil)
		}
	} else if plan == nil {
		// 简化模式：解析目标并直接执行
		l.logger.Warnf("agent loop: using simplified mode (no LLM)")
		cmd := extractCommandFromGoal(t.Goal)
		l.logger.Infof("agent loop: simplified command=%q", cmd)
		plan = &Plan{
			Steps: []PlanStep{
				{
					Description: "执行任务: " + t.Goal,
					Tool:        "shell.run",
					Args: map[string]any{
						"command": cmd,
						"timeout": 30,
					},
					MaxRetries: 2,
				},
			},
		}
	}

	if err := l.store.SavePlan(t.ID, plan); err != nil {
		l.logger.Warnf("failed to save plan: %v", err)
	}

	// 防止 0 步骤任务
	if len(plan.Steps) == 0 {
		l.store.SetResult(t.ID, "No steps generated", "")
		return fmt.Errorf("no steps generated for goal: %s", t.Goal)
	}

	l.logger.Infof("agent loop: task %s plan has %d steps", t.ID, len(plan.Steps))

	// 2. 执行步骤（支持从断点 resumeStep 继续）
	for i := resumeStep; i < len(plan.Steps); i++ {
		select {
		case <-ctx.Done():
			l.logger.Infof("agent loop: task %s cancelled", t.ID)
			return ctx.Err()
		default:
		}

		step := plan.Steps[i]
		totalSteps := len(plan.Steps)
		l.logger.Infof("agent loop: task %s step %d/%d - %s", t.ID, i+1, totalSteps, step.Description)

		progress := float64(i) / float64(totalSteps) * 100
		l.store.UpdateProgress(t.ID, progress, i)

		// 广播步骤开始
		l.emitStep("step.started", map[string]any{
			"task_id": t.ID, "step": i + 1, "total": totalSteps,
			"description": step.Description, "tool": step.Tool,
		})

		// 权限检查：PermissionEngine 策略表判定（键 = tool[:action]）。
		// 用户默认 Level1Normal（任务经 task manager 后台运行，principal 不入库）；
		// 策略等级 >= L2 且用户不足时进入确认流，直接拒绝则失败该步。
		if l.permEngine != nil {
			decision := l.checkToolPermission(ctx, step.Tool, step.Args)
			if !decision.Allowed && !decision.NeedConfirm {
				l.logger.Warnf("agent loop: task %s step %d denied by policy: %s", t.ID, i+1, decision.Reason)
				l.store.SetResult(t.ID, "", decision.Reason)
				return fmt.Errorf("step %d denied: %s", i+1, decision.Reason)
			}
			if decision.NeedConfirm {
				l.logger.Infof("agent loop: task %s step %d requires confirmation for %s (level=%d)", t.ID, i+1, step.Tool, decision.Level)
				approved, err := l.waitForConfirmation(ctx, t, &step, i, decision)
				if err != nil {
					l.logger.Warnf("confirmation error: %v, skipping step", err)
					continue
				}
				if !approved {
					l.logger.Warnf("agent loop: task %s step %d denied by user", t.ID, i+1)
					l.store.SetResult(t.ID, "", "Step denied by user confirmation")
					return fmt.Errorf("step %d denied by user", i+1)
				}
			}
		}

		// 破坏性命令检测：shell.run 的命令若被 ClassifyCommand 判定为破坏性，
		// 构造 NeedConfirm 决策走确认流（shell.go 的硬拒绝保留为最后防线）
		if step.Tool == "shell.run" {
			if cmd, ok := step.Args["command"].(string); ok {
				if ClassifyCommand(cmd) == CommandDestructive {
					l.logger.Infof("agent loop: task %s step %d shell.run destructive, requiring confirmation", t.ID, i+1)
					confirmDecision := auth.PermissionDecision{
						Allowed:     false,
						NeedConfirm: true,
						Level:       auth.Level2Dangerous,
						Reason:      fmt.Sprintf("破坏性命令需要确认（分类：%s）", CommandClassLabel(CommandDestructive)),
					}
					approved, err := l.waitForConfirmation(ctx, t, &step, i, confirmDecision)
					if err != nil {
						l.logger.Warnf("destructive confirmation error: %v, skipping step", err)
						continue
					}
					if !approved {
						l.logger.Warnf("agent loop: task %s step %d destructive command denied by user", t.ID, i+1)
						l.store.SetResult(t.ID, "", "破坏性命令被用户拒绝")
						return fmt.Errorf("step %d denied: destructive command rejected by user", i+1)
					}
					step.destructiveApproved = true
				}
			}
		}

		// 执行步骤（含重试）
		var lastErr error
		var lastResult *ToolResult
		maxRetries := step.MaxRetries
		if maxRetries == 0 {
			maxRetries = 2
		}

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				l.logger.Infof("agent loop: task %s step %d retry %d", t.ID, i+1, attempt-1)
				time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
			}

			res, err := l.executeStep(ctx, t, &step, i)
			if err == nil {
				lastErr = nil
				lastResult = res
				break
			}
			lastErr = err
		}

		if lastErr != nil {
			l.logger.Errorf("agent loop: task %s step %d failed: %v", t.ID, i+1, lastErr)
			l.store.SetResult(t.ID, "", lastErr.Error())
			l.emitStep("step.failed", map[string]any{"task_id": t.ID, "step": i + 1, "error": lastErr.Error()})

			// 通过 EventBus 发射任务失败事件（触发主动恢复）
			if l.eventBus != nil {
				l.eventBus.EmitAsync(Event{
					Source:   "agent-loop",
					Type:     EventTaskFailed,
					Payload:  t.ID,
					Priority: 5,
				})
			}

			// 尝试 Replanner（替换剩余步骤，保留当前步骤位置）
			if i+1 < totalSteps {
				l.logger.Infof("agent loop: task %s trying replanner", t.ID)
				remaining := plan.Steps[i+1:]
				newPlan, replanErr := l.replanner.Replan(ctx, t.Goal, step, lastErr, remaining, lastErr.Error())
				if replanErr == nil && len(newPlan.Steps) > 0 {
					plan.Steps = append(plan.Steps[:i+1], newPlan.Steps...)
					l.logger.Infof("agent loop: task %s replanned, now %d steps", t.ID, len(plan.Steps))
					continue // 重新执行当前 i 位置
				}
			}
			// 收集失败反馈
			t.Status = task.StatusFailed
			t.Error = lastErr.Error()
			if l.feedback != nil {
				l.feedback.CollectFromTask(t, time.Since(startTime).Seconds(), i+1)
			}
			// 保存追踪记录
			trace.Save()
			return lastErr
		}

		// 广播步骤完成（带工具结果摘要，供前端实时展示）
		stepEvent := map[string]any{
			"task_id": t.ID, "step": i + 1, "total": totalSteps, "tool": step.Tool,
		}
		if lastResult != nil {
			if lastResult.Summary != "" {
				stepEvent["summary"] = lastResult.Summary
			}
			if lastResult.Raw != "" {
				stepEvent["result"] = truncateStepResult(lastResult.Raw)
			}
		}
		l.emitStep("step.completed", stepEvent)

		// 保存 Checkpoint（每步执行完后）
		l.store.SaveCheckpoint(t.ID, i)
	}

	l.store.UpdateProgress(t.ID, 100, len(plan.Steps))
	l.store.SetResult(t.ID, fmt.Sprintf("Completed %d steps", len(plan.Steps)), "")
	// 修正任务状态为 completed（SetResult 不更新 status）
	l.store.SetStatus(t.ID, task.StatusCompleted)
	l.emitStep("task.completed", map[string]any{"task_id": t.ID, "steps": len(plan.Steps)})
	l.logger.Infof("agent loop: task %s completed", t.ID)

	// 保存追踪记录
	trace.Save()

	// 收集反馈（成功率/工具/耗时等统计）
	if l.feedback != nil {
		l.feedback.CollectFromTask(t, time.Since(startTime).Seconds(), len(plan.Steps))
	}

	// 独立目标达成复核（bounded reviewer）：不污染主会话上下文

	// 任务完成，沉淀记忆（确认标记=0，等待用户确认）
	if l.memoryStore != nil && t.Goal != "" {
		mem := &memory.Memory{
			ID:         fmt.Sprintf("m-%s", t.ID),
			Type:       memory.MemoryLongTerm,
			Content:    fmt.Sprintf("任务执行成功: %s（已完成 %d 步）", t.Goal, len(plan.Steps)),
			Tags:       "task,completed",
			SourceTask: t.ID,
			Confirmed:  false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := l.memoryStore.Save(mem); err != nil {
			l.logger.Warnf("failed to save memory: %v", err)
		}
	}

	// 独立目标达成复核（bounded reviewer）：不污染主会话上下文
	l.reviewGoal(ctx, t, plan)

	return nil
}

// reviewGoal 用一次独立的有界 LLM 调用复核任务目标是否达成，不污染主会话上下文。
// 仅当 LLM 可用且 provider 支持 BoundedChat 时执行；失败不影响任务结果。
func (l *Loop) reviewGoal(ctx context.Context, t *task.Task, plan *Plan) {
	if l.planner == nil || l.provider == nil {
		return
	}
	op, ok := l.provider.(*llm.OpenAIProvider)
	if !ok {
		return
	}
	var stepsBrief []string
	for _, s := range plan.Steps {
		stepsBrief = append(stepsBrief, fmt.Sprintf("- %s (tool=%s)", s.Description, s.Tool))
	}
	system := "你是一个严格且简洁的目标达成审核员。只回答：达成情况(达成/部分达成/未达成) + 一句话理由。不要多余内容。"
	user := fmt.Sprintf("任务目标: %s\n\n执行步骤:\n%s\n\n请判定该目标是否达成。", t.Goal, strings.Join(stepsBrief, "\n"))
	verdict, err := op.BoundedChat(ctx, system, user, 128, 30)
	if err != nil {
		l.logger.Warnf("goal review failed for task %s: %v", t.ID, err)
		return
	}
	l.logger.Infof("agent loop: task %s goal review → %s", t.ID, strings.TrimSpace(verdict))
	l.emitStep("task.review", map[string]any{"task_id": t.ID, "verdict": strings.TrimSpace(verdict)})
}
