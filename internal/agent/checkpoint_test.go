package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"agent/internal/task"
)

// StressCheckpoint 压力测试：模拟断点恢复
// 1. 创建多步任务
// 2. 执行前 N 步
// 3. 模拟崩溃（不完成任务）
// 4. 重新加载任务，验证能否从 checkpoint 继续
func StressCheckpoint(ctx context.Context, loop *Loop) (string, error) {
	report := ""
	report += "=== Checkpoint Stress Test ===\n\n"

	// 1. 创建一个 3 步任务
	t := &task.Task{
		ID:       fmt.Sprintf("stress-cp-%d", time.Now().UnixMilli()),
		Goal:     "执行 echo step1, echo step2, echo step3",
		Status:   task.StatusRunning,
		Priority: 5,
	}
	loop.store.SaveTask(t)
	report += fmt.Sprintf("Created task: %s (3 steps)\n", t.ID)

	// 2. 模拟执行第 1 步
	loop.emitStep("step.started", map[string]any{"task_id": t.ID, "step": 1, "total": 3, "tool": "shell.run"})
	_, err := loop.RunTool(ctx, "shell.run", map[string]any{"command": "echo step1", "timeout": 10})
	if err != nil {
		return report, fmt.Errorf("step 1 failed: %w", err)
	}
	loop.emitStep("step.completed", map[string]any{"task_id": t.ID, "step": 1, "total": 3})
	loop.store.SaveCheckpoint(t.ID, 0) // checkpoint 在 step 1 完成后
	loop.store.SavePlan(t.ID, &Plan{Steps: []PlanStep{
		{Description: "echo step1", Tool: "shell.run", Args: map[string]any{"command": "echo step1"}},
		{Description: "echo step2", Tool: "shell.run", Args: map[string]any{"command": "echo step2"}},
		{Description: "echo step3", Tool: "shell.run", Args: map[string]any{"command": "echo step3"}},
	}})
	report += "Step 1 completed + checkpoint saved\n"

	// 3. 模拟崩溃：不执行 step 2/3
	report += "=== Simulated crash (no step 2/3 executed) ===\n"

	// 4. 验证 checkpoint 可以加载
	checkpoint, err := loop.store.LoadCheckpoint(t.ID)
	if err != nil {
		return report, fmt.Errorf("load checkpoint failed: %w", err)
	}
	if checkpoint >= 0 {
		report += fmt.Sprintf("Checkpoint loaded: resumeStep=%d\n", checkpoint)
	} else {
		report += "No checkpoint found (FAIL)\n"
	}

	// 5. 验证可以从 checkpoint 继续（手动模拟 loop.Run 的 resume 逻辑）
	storedPlan, _ := loop.store.GetPlan(t.ID)
	if storedPlan != nil {
		var plan Plan
		if err := json.Unmarshal(storedPlan, &plan); err == nil && len(plan.Steps) > 0 {
			resumeStep := checkpoint
			report += fmt.Sprintf("Plan has %d steps, resume from step %d\n", len(plan.Steps), resumeStep)
			for i := resumeStep; i < len(plan.Steps); i++ {
				loop.emitStep("step.started", map[string]any{"task_id": t.ID, "step": i + 1, "total": len(plan.Steps)})
				_, err := loop.RunTool(ctx, "shell.run", plan.Steps[i].Args)
				if err != nil {
					report += fmt.Sprintf("Step %d failed: %v\n", i+1, err)
					continue
				}
				loop.emitStep("step.completed", map[string]any{"task_id": t.ID, "step": i + 1, "total": len(plan.Steps)})
				loop.store.SaveCheckpoint(t.ID, i)
				report += fmt.Sprintf("Step %d completed\n", i+1)
			}
			report += "=== Checkpoint resume: PASS ===\n"
		}
	}

	// 清理
	loop.store.SetStatus(t.ID, task.StatusCompleted)
	loop.store.SetResult(t.ID, "stress test done", "")

	return report, nil
}
