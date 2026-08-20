package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// mockWorkflowStore 测试用内存工作流存储
type mockWorkflowStore struct {
	workflows map[string]*Workflow
}

func newMockWorkflowStore() *mockWorkflowStore {
	return &mockWorkflowStore{workflows: make(map[string]*Workflow)}
}

func (m *mockWorkflowStore) Save(wf *Workflow) error {
	m.workflows[wf.ID] = wf
	return nil
}
func (m *mockWorkflowStore) Get(id string) (*Workflow, error) {
	wf, ok := m.workflows[id]
	if !ok {
		return nil, nil
	}
	return wf, nil
}
func (m *mockWorkflowStore) UpdateStatus(id string, status string) error {
	if wf, ok := m.workflows[id]; ok {
		wf.Status = status
	}
	return nil
}
func (m *mockWorkflowStore) UpdateStepStatus(wfID string, stepID string, status string, result string, errText string) error {
	if wf, ok := m.workflows[wfID]; ok {
		for i := range wf.Steps {
			if wf.Steps[i].ID == stepID {
				wf.Steps[i].Status = status
				wf.Steps[i].Result = result
				wf.Steps[i].Error = errText
				break
			}
		}
	}
	return nil
}

func TestValidate_NoSteps(t *testing.T) {
	wf := &Workflow{Steps: []WorkflowStep{}}
	if err := Validate(wf); err == nil {
		t.Error("expected error for empty workflow")
	}
}

func TestValidate_DuplicateID(t *testing.T) {
	wf := &Workflow{Steps: []WorkflowStep{
		{ID: "a"}, {ID: "a"},
	}}
	if err := Validate(wf); err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestValidate_MissingDependency(t *testing.T) {
	wf := &Workflow{Steps: []WorkflowStep{
		{ID: "a", DependsOn: []string{"b"}},
	}}
	if err := Validate(wf); err == nil {
		t.Error("expected error for missing dependency")
	}
}

func TestValidate_Cycle(t *testing.T) {
	wf := &Workflow{Steps: []WorkflowStep{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}}
	if err := Validate(wf); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestValidate_Valid(t *testing.T) {
	wf := &Workflow{Steps: []WorkflowStep{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}}
	if err := Validate(wf); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestDAGExecutor_Linear(t *testing.T) {
	store := newMockWorkflowStore()
	var executed []string
	var mu sync.Mutex

	executor := NewDAGExecutor(store, func(ctx context.Context, step *WorkflowStep) error {
		mu.Lock()
		executed = append(executed, step.ID)
		mu.Unlock()
		step.Result = "done"
		return nil
	})

	wf := &Workflow{
		ID:    "test-1",
		Name:  "linear",
		Steps: []WorkflowStep{
			{ID: "a", Tool: "shell.run", Args: map[string]any{"command": "echo a"}},
			{ID: "b", Tool: "shell.run", Args: map[string]any{"command": "echo b"}, DependsOn: []string{"a"}},
			{ID: "c", Tool: "shell.run", Args: map[string]any{"command": "echo c"}, DependsOn: []string{"b"}},
		},
	}

	err := executor.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if wf.Status != "completed" {
		t.Errorf("status = %s, want completed", wf.Status)
	}
	if len(executed) != 3 {
		t.Errorf("executed %d steps, want 3", len(executed))
	}
	// 验证执行顺序：a → b → c
	if executed[0] != "a" || executed[1] != "b" || executed[2] != "c" {
		t.Errorf("execution order = %v, want [a b c]", executed)
	}
}

func TestDAGExecutor_Parallel(t *testing.T) {
	store := newMockWorkflowStore()
	var executed []string
	var mu sync.Mutex

	executor := NewDAGExecutor(store, func(ctx context.Context, step *WorkflowStep) error {
		mu.Lock()
		executed = append(executed, step.ID)
		mu.Unlock()
		step.Result = "done"
		return nil
	})

	wf := &Workflow{
		ID:    "test-2",
		Name:  "parallel",
		Steps: []WorkflowStep{
			{ID: "a", Tool: "shell.run"},
			{ID: "b", Tool: "shell.run", DependsOn: []string{"a"}},
			{ID: "c", Tool: "shell.run", DependsOn: []string{"a"}}, // b 和 c 无依赖，应并行
			{ID: "d", Tool: "shell.run", DependsOn: []string{"b", "c"}},
		},
	}

	err := executor.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(executed) != 4 {
		t.Errorf("executed %d steps, want 4", len(executed))
	}
	// a 必须在 b、c 之前；d 必须在 b、c 之后
	ai, bi, ci, di := -1, -1, -1, -1
	for i, id := range executed {
		switch id {
		case "a":
			ai = i
		case "b":
			bi = i
		case "c":
			ci = i
		case "d":
			di = i
		}
	}
	if ai > bi || ai > ci || bi > di || ci > di {
		t.Errorf("wrong order: a=%d b=%d c=%d d=%d", ai, bi, ci, di)
	}
}

func TestDAGExecutor_StepFailure(t *testing.T) {
	store := newMockWorkflowStore()

	executor := NewDAGExecutor(store, func(ctx context.Context, step *WorkflowStep) error {
		if step.ID == "b" {
			return fmt.Errorf("step b failed")
		}
		step.Result = "done"
		return nil
	})

	wf := &Workflow{
		ID:    "test-3",
		Name:  "failure",
		Steps: []WorkflowStep{
			{ID: "a", Tool: "shell.run"},
			{ID: "b", Tool: "shell.run", DependsOn: []string{"a"}},
			{ID: "c", Tool: "shell.run", DependsOn: []string{"b"}}, // 依赖失败的 b → 被跳过
		},
	}

	err := executor.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if wf.Status != "failed" {
		t.Errorf("status = %s, want failed", wf.Status)
	}

	// c 应该被跳过
	for _, s := range wf.Steps {
		if s.ID == "c" && s.Status != "skipped" {
			t.Errorf("step c status = %s, want skipped", s.Status)
		}
	}
}

func TestSummarize(t *testing.T) {
	wf := &Workflow{
		Name:   "test",
		Status: "completed",
		Steps: []WorkflowStep{
			{ID: "a", Tool: "shell.run", Description: "step a", Status: "completed"},
			{ID: "b", Tool: "fs", Description: "step b", Status: "failed", Error: "not found"},
		},
	}
	summary := Summarize(wf)
	if !strings.Contains(summary, "✅") || !strings.Contains(summary, "❌") {
		t.Errorf("summary missing status icons: %s", summary)
	}
}
