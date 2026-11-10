package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent/internal/auth"
	"agent/internal/config"
	agentDB "agent/internal/db"
	"agent/internal/llm"
	"agent/internal/task"

	"go.uber.org/zap"
)

// stubProvider 同时实现 llm.Provider 与 boundedChatter，供目标闭环单测使用。
// plans 按调用顺序返回计划 JSON；verdicts 按调用顺序返回复核结论。
type stubProvider struct {
	mu          sync.Mutex
	chatCalls   int
	reviewCalls int
	plans       []string
	verdicts    []string
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (*llm.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chatCalls++
	if len(s.plans) == 0 {
		return nil, fmt.Errorf("stub: no more plans")
	}
	p := s.plans[0]
	s.plans = s.plans[1:]
	return &llm.Response{Content: p, Finish: "stop"}, nil
}

func (s *stubProvider) BoundedChat(ctx context.Context, system, user string, maxTokens, timeoutSec int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviewCalls++
	if len(s.verdicts) == 0 {
		return "达成：默认结论", nil
	}
	v := s.verdicts[0]
	s.verdicts = s.verdicts[1:]
	return v, nil
}

func (s *stubProvider) counts() (chat, review int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatCalls, s.reviewCalls
}

// fakeTool 记录执行次数的无副作用测试工具。
type fakeTool struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeTool) Name() string        { return "noop" }
func (f *fakeTool) Description() string { return "test no-op tool" }
func (f *fakeTool) RequiredLevel() int  { return 0 }
func (f *fakeTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return &ToolResult{Raw: "noop-ok", Kind: "text", Summary: "noop ok"}, nil
}

func (f *fakeTool) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func planJSON(desc string) string {
	b, _ := json.Marshal(map[string]any{
		"steps": []map[string]any{{
			"description": desc,
			"tool":        "noop",
			"args":        map[string]any{},
			"max_retries": 1,
		}},
	})
	return string(b)
}

// newGoalLoopTest 构造隔离的 Loop：临时 DB + stub provider + fake 工具（L0 放行）。
func newGoalLoopTest(t *testing.T, provider llm.Provider, rounds int) (*Loop, *fakeTool) {
	t.Helper()
	database, err := agentDB.Init(filepath.Join(t.TempDir(), "goal-loop.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	cfg.Agent.GoalLoopMaxRounds = rounds

	loop := NewLoop(database, zap.NewNop(), provider)
	ft := &fakeTool{}
	loop.RegisterTool(ft)
	// 未知工具默认 fail-closed 到 L2；测试工具显式放行到 L0
	loop.permEngine.AddPolicy(auth.Policy{Pattern: "noop", Level: auth.Level0ReadOnly})
	return loop, ft
}

func newGoalTestTask(t *testing.T, loop *Loop, goal string) *task.Task {
	t.Helper()
	now := time.Now()
	tk := &task.Task{
		ID:        fmt.Sprintf("goal-loop-%d", now.UnixNano()),
		Goal:      goal,
		Status:    task.StatusRunning,
		Priority:  5,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := loop.store.SaveTask(tk); err != nil {
		t.Fatalf("save task: %v", err)
	}
	return tk
}

func TestParseGoalVerdict(t *testing.T) {
	cases := []struct {
		verdict  string
		achieved bool
		known    bool
	}{
		{"达成：已输出 noop-ok", true, true},
		{"已达成", true, true},
		{"已完成", true, true},
		{"部分达成：还差验证", false, true},
		{"未达成：缺少第二步", false, true},
		{"未完成", false, true},
		{"没有达成", false, true},
		{"失败：工具报错", false, true},
		{"OK", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		achieved, known := parseGoalVerdict(c.verdict)
		if achieved != c.achieved || known != c.known {
			t.Errorf("parseGoalVerdict(%q) = (%v, %v), want (%v, %v)",
				c.verdict, achieved, known, c.achieved, c.known)
		}
	}
}

func TestStripThinkingBlocks(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<think>推理</think>未达成：还差一步", "未达成：还差一步"},
		{"达成：ok", "达成：ok"},
		{"<think>没有闭合的思考", ""},
		{"前缀<think>a</think>中<think>b</think>后", "前缀中后"},
	}
	for _, c := range cases {
		if got := stripThinkingBlocks(c.in); got != c.want {
			t.Errorf("stripThinkingBlocks(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 复核判定达成：只跑一轮计划，不续跑。
func TestGoalLoopStopsWhenAchieved(t *testing.T) {
	p := &stubProvider{
		plans:    []string{planJSON("step one")},
		verdicts: []string{"达成：noop-ok 已输出"},
	}
	loop, ft := newGoalLoopTest(t, p, 2)
	tk := newGoalTestTask(t, loop, "do the thing")

	if err := loop.Run(context.Background(), tk); err != nil {
		t.Fatalf("run: %v", err)
	}

	chat, review := p.counts()
	if chat != 1 || review != 1 {
		t.Fatalf("chatCalls=%d reviewCalls=%d, want 1/1 (no continuation)", chat, review)
	}
	if got := ft.count(); got != 1 {
		t.Fatalf("tool calls=%d, want 1", got)
	}
	stored, err := loop.store.GetTask(tk.ID)
	if err != nil || stored == nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != task.StatusCompleted {
		t.Fatalf("status=%s, want completed", stored.Status)
	}
}

// 复核未达成 → 续规划并继续执行，第二次复核达成后停止。
func TestGoalLoopContinuesUntilAchieved(t *testing.T) {
	p := &stubProvider{
		plans:    []string{planJSON("step one"), planJSON("step two")},
		verdicts: []string{"<think>目标是否达成？看起来还没有达成</think>未达成：还需要第二步", "达成：目标完成"},
	}
	loop, ft := newGoalLoopTest(t, p, 2)
	tk := newGoalTestTask(t, loop, "do the thing in two steps")

	if err := loop.Run(context.Background(), tk); err != nil {
		t.Fatalf("run: %v", err)
	}

	chat, review := p.counts()
	if chat != 2 || review != 2 {
		t.Fatalf("chatCalls=%d reviewCalls=%d, want 2/2", chat, review)
	}
	if got := ft.count(); got != 2 {
		t.Fatalf("tool calls=%d, want 2 (original + continuation)", got)
	}
	stored, err := loop.store.GetTask(tk.ID)
	if err != nil || stored == nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != task.StatusCompleted {
		t.Fatalf("status=%s, want completed", stored.Status)
	}
	if !strings.Contains(stored.Result, "目标复核") || !strings.Contains(stored.Result, "达成") {
		t.Fatalf("result %q should record the final goal review verdict", stored.Result)
	}
}

// 复核始终未达成：续跑轮数必须有上限，耗尽后停止并如实记录结论。
func TestGoalLoopStopsAfterBudgetExhausted(t *testing.T) {
	p := &stubProvider{
		plans:    []string{planJSON("s1"), planJSON("s2"), planJSON("s3")},
		verdicts: []string{"未达成：还差一点", "未达成：还差一点", "未达成：还差一点"},
	}
	loop, ft := newGoalLoopTest(t, p, 2)

	var events []string
	loop.SetStepCallback(func(eventType string, data map[string]any) {
		events = append(events, eventType)
	})

	tk := newGoalTestTask(t, loop, "never finishes")
	if err := loop.Run(context.Background(), tk); err != nil {
		t.Fatalf("run: %v", err)
	}

	chat, review := p.counts()
	if chat != 3 || review != 3 {
		t.Fatalf("chatCalls=%d reviewCalls=%d, want 3/3 (1 initial + 2 continuations)", chat, review)
	}
	if got := ft.count(); got != 3 {
		t.Fatalf("tool calls=%d, want 3", got)
	}
	if !containsEvent(events, "task.continue") {
		t.Fatal("expected task.continue event")
	}
	if !containsEvent(events, "task.review_exhausted") {
		t.Fatal("expected task.review_exhausted event")
	}
	stored, err := loop.store.GetTask(tk.ID)
	if err != nil || stored == nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != task.StatusCompleted {
		t.Fatalf("status=%s, want completed", stored.Status)
	}
	if !strings.Contains(stored.Result, "未达成") {
		t.Fatalf("result %q should record the unmet verdict", stored.Result)
	}
}

// 轮数为 0 时关闭闭环：复核照常执行，但不再续规划（保持旧的单轮行为）。
func TestGoalLoopDisabledByZeroRounds(t *testing.T) {
	p := &stubProvider{
		plans:    []string{planJSON("only step")},
		verdicts: []string{"未达成：还差一点"},
	}
	loop, ft := newGoalLoopTest(t, p, 0)
	tk := newGoalTestTask(t, loop, "single round")

	if err := loop.Run(context.Background(), tk); err != nil {
		t.Fatalf("run: %v", err)
	}

	chat, review := p.counts()
	if chat != 1 || review != 1 {
		t.Fatalf("chatCalls=%d reviewCalls=%d, want 1/1 (no continuation when disabled)", chat, review)
	}
	if got := ft.count(); got != 1 {
		t.Fatalf("tool calls=%d, want 1", got)
	}
}

// 复核结论无法识别时不触发续跑（避免小模型输出噪声导致无谓循环）。
func TestGoalLoopStopsOnUnparsableVerdict(t *testing.T) {
	p := &stubProvider{
		plans:    []string{planJSON("only step")},
		verdicts: []string{"OK"},
	}
	loop, ft := newGoalLoopTest(t, p, 2)
	tk := newGoalTestTask(t, loop, "noisy verdict")

	if err := loop.Run(context.Background(), tk); err != nil {
		t.Fatalf("run: %v", err)
	}

	chat, review := p.counts()
	if chat != 1 || review != 1 {
		t.Fatalf("chatCalls=%d reviewCalls=%d, want 1/1", chat, review)
	}
	if got := ft.count(); got != 1 {
		t.Fatalf("tool calls=%d, want 1", got)
	}
}

func containsEvent(events []string, want string) bool {
	for _, e := range events {
		if e == want {
			return true
		}
	}
	return false
}

// strict 档下 shell.run 经确认流批准后必须真正执行。
// 回归：此前主循环只给破坏性命令注入放行标记，普通 shell 步骤批准后
// 仍被 shell 层以 "requires confirmation under shell_profile" 拒绝。
func TestGoalLoopApprovedShellRunsUnderStrict(t *testing.T) {
	shellPlan, _ := json.Marshal(map[string]any{
		"steps": []map[string]any{{
			"description": "echo approved-shell",
			"tool":        "shell.run",
			"args":        map[string]any{"command": "echo approved-shell", "timeout": 5},
			"max_retries": 1,
		}},
	})
	p := &stubProvider{
		plans:    []string{string(shellPlan)},
		verdicts: []string{"达成：命令已执行"},
	}
	loop, _ := newGoalLoopTest(t, p, 2)
	config.Get().Permissions.ShellProfile = "strict"

	tk := newGoalTestTask(t, loop, "run an approved shell command")
	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background(), tk) }()

	var confID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pending, err := loop.confirmStore.ListPending()
		if err == nil && len(pending) > 0 {
			confID = pending[0].ID
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if confID == "" {
		t.Fatal("strict 档下 shell.run 应进入确认流")
	}
	if err := loop.confirmStore.Approve(confID, "test"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("approved shell.run should succeed, got: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("task did not finish after approval")
	}

	stored, err := loop.store.GetTask(tk.ID)
	if err != nil || stored == nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != task.StatusCompleted {
		t.Fatalf("status=%s, want completed", stored.Status)
	}
}
