package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// WorkflowStore 接口：工作流持久化
type WorkflowStoreInterface interface {
	Save(wf *Workflow) error
	Get(id string) (*Workflow, error)
	UpdateStatus(id string, status string) error
	UpdateStepStatus(wfID string, stepID string, status string, result string, errText string) error
}

// DAGExecutor DAG 工作流执行器
// 按拓扑序执行步骤：无依赖的步骤并行执行，有依赖的等前置完成后执行。
type DAGExecutor struct {
	store    WorkflowStoreInterface
	runStep  func(ctx context.Context, step *WorkflowStep) error
	onUpdate func(wfID string, step *WorkflowStep) // 进度回调（SSE 推送用）
}

// NewDAGExecutor 创建 DAG 执行器
func NewDAGExecutor(store WorkflowStoreInterface, runStep func(ctx context.Context, step *WorkflowStep) error) *DAGExecutor {
	return &DAGExecutor{
		store:   store,
		runStep: runStep,
	}
}

// SetUpdateCallback 设置进度更新回调
func (e *DAGExecutor) SetUpdateCallback(cb func(wfID string, step *WorkflowStep)) {
	e.onUpdate = cb
}

// Validate 验证 DAG 是否合法（无环、依赖存在）
func Validate(wf *Workflow) error {
	if len(wf.Steps) == 0 {
		return fmt.Errorf("workflow has no steps")
	}

	// 检查所有依赖引用的 step ID 都存在
	stepIDs := make(map[string]bool)
	for _, s := range wf.Steps {
		if s.ID == "" {
			return fmt.Errorf("step missing ID")
		}
		if stepIDs[s.ID] {
			return fmt.Errorf("duplicate step ID: %s", s.ID)
		}
		stepIDs[s.ID] = true
	}

	for _, s := range wf.Steps {
		for _, dep := range s.DependsOn {
			if !stepIDs[dep] {
				return fmt.Errorf("step %s depends on unknown step %s", s.ID, dep)
			}
		}
	}

	// 拓扑排序 + 环检测（Kahn's algorithm）
	inDegree := make(map[string]int)
	adj := make(map[string][]string) // parent → children
	for _, s := range wf.Steps {
		if _, ok := inDegree[s.ID]; !ok {
			inDegree[s.ID] = 0
		}
		for _, dep := range s.DependsOn {
			adj[dep] = append(adj[dep], s.ID)
			inDegree[s.ID]++
		}
	}

	// 放入入度为 0 的节点
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range adj[node] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if visited != len(wf.Steps) {
		return fmt.Errorf("workflow has a cycle (visited %d/%d steps)", visited, len(wf.Steps))
	}

	return nil
}

// Run 执行工作流（按 DAG 拓扑序，无依赖的步骤并行）
func (e *DAGExecutor) Run(ctx context.Context, wf *Workflow) error {
	wf.Status = "running"
	e.store.UpdateStatus(wf.ID, "running")

	// 跟踪每个步骤状态（空状态视为 pending）
	stepStatus := make(map[string]string) // stepID → status
	for _, s := range wf.Steps {
		status := s.Status
		if status == "" {
			status = "pending"
		}
		stepStatus[s.ID] = status
	}

	// 获取已完成的步骤（支持断点恢复）
	completed := make(map[string]bool)
	for _, s := range wf.Steps {
		if s.Status == "completed" {
			completed[s.ID] = true
		}
	}

	maxIterations := len(wf.Steps) * 2 // 防止无限循环
	iteration := 0

	for iteration < maxIterations {
		iteration++

		// 检查是否全部完成
		allDone := true
		for _, s := range wf.Steps {
			if stepStatus[s.ID] != "completed" && stepStatus[s.ID] != "failed" {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}

		// 找到可执行的步骤（依赖已满足）
		var ready []*WorkflowStep
		for i := range wf.Steps {
			s := &wf.Steps[i]
			if stepStatus[s.ID] != "pending" {
				continue
			}
			if len(s.DependsOn) == 0 {
				ready = append(ready, s)
				continue
			}
			allDepsMet := true
			for _, dep := range s.DependsOn {
				if stepStatus[dep] != "completed" {
					allDepsMet = false
					break
				}
			}
			if allDepsMet {
				ready = append(ready, s)
			}
		}

		if len(ready) == 0 {
			// 没有可执行的步骤了，但还有未完成的 → 检查是否有失败的依赖
			failed := false
			for i := range wf.Steps {
				s := &wf.Steps[i]
				if stepStatus[s.ID] == "failed" {
					// 检查是否有步骤依赖这个失败的步骤
					for j := range wf.Steps {
						s2 := &wf.Steps[j]
						if stepStatus[s2.ID] == "pending" {
							for _, dep := range s2.DependsOn {
								if dep == s.ID {
									// 依赖的步骤失败了 → 标记为跳过
									stepStatus[s2.ID] = "skipped"
									s2.Status = "skipped"
									s2.Error = fmt.Sprintf("dependency %s failed", s.ID)
									e.store.UpdateStepStatus(wf.ID, s2.ID, "skipped", "", s2.Error)
									if e.onUpdate != nil {
										e.onUpdate(wf.ID, s2)
									}
								}
							}
						}
					}
					failed = true
				}
			}
			if !failed {
				// 没有失败的，也没有可执行的 → 等待中
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// 有失败导致的阻塞，继续检查
			continue
		}

		// 并行执行所有 ready 步骤
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, s := range ready {
			wg.Add(1)
			go func(step *WorkflowStep) {
				defer wg.Done()

				// 标记为运行中
				mu.Lock()
				stepStatus[step.ID] = "running"
				step.Status = "running"
				now := time.Now()
				step.StartedAt = &now
				mu.Unlock()

				e.store.UpdateStepStatus(wf.ID, step.ID, "running", "", "")
				if e.onUpdate != nil {
					e.onUpdate(wf.ID, step)
				}

				// 执行步骤
				err := e.runStep(ctx, step)

				mu.Lock()
				if err != nil {
					stepStatus[step.ID] = "failed"
					step.Status = "failed"
					step.Error = err.Error()
					e.store.UpdateStepStatus(wf.ID, step.ID, "failed", "", err.Error())
				} else {
					stepStatus[step.ID] = "completed"
					step.Status = "completed"
					completed[step.ID] = true
					e.store.UpdateStepStatus(wf.ID, step.ID, "completed", step.Result, "")
				}
				mu.Unlock()

				if e.onUpdate != nil {
					e.onUpdate(wf.ID, step)
				}
			}(s)
		}
		wg.Wait()
	}

	// 判断最终状态
	finalStatus := "completed"
	for _, s := range wf.Steps {
		if s.Status == "failed" {
			finalStatus = "failed"
			break
		}
		if s.Status == "skipped" {
			if finalStatus != "failed" {
				finalStatus = "partial"
			}
		}
	}

	wf.Status = finalStatus
	e.store.UpdateStatus(wf.ID, finalStatus)
	return nil
}

// Summarize 生成工作流执行摘要
func Summarize(wf *Workflow) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("工作流: %s (状态: %s)\n", wf.Name, wf.Status))
	sb.WriteString(fmt.Sprintf("步骤: %d\n\n", len(wf.Steps)))

	for i, s := range wf.Steps {
		status := "⏳"
		switch s.Status {
		case "completed":
			status = "✅"
		case "failed":
			status = "❌"
		case "running":
			status = "🔄"
		case "skipped":
			status = "⏭️"
		}
		sb.WriteString(fmt.Sprintf("%d. %s [%s] %s\n", i+1, status, s.Tool, s.Description))
		if s.Error != "" {
			sb.WriteString(fmt.Sprintf("   错误: %s\n", s.Error))
		}
	}
	return sb.String()
}
