package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentDB "agent/internal/db"
	"go.uber.org/zap"
)

// 回归测试：RunTool 不得直通执行。
// 破坏性命令必须先出现在确认流中（此前 /v1/tools/run 与 chat tool_call
// 经 RunTool 直通 tool.Execute，完全绕过权限与确认）；批准后才执行。
func TestRunToolDestructiveRequiresConfirmation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "lumen-test.db")
	database, err := agentDB.Init(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer database.Close()

	loop := NewLoop(database, zap.NewNop(), nil)

	// rmdir 被 ClassifyCommand 判定为破坏性，但不触碰 shell.go 硬黑名单；
	// 命令本身可成功执行（先创建待删目录）
	dir := filepath.Join(t.TempDir(), "tobe-removed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cmd := "rmdir " + dir
	done := make(chan error, 1)
	go func() {
		_, err := loop.RunTool(context.Background(), "shell.run", map[string]any{
			"command": cmd, "timeout": 10,
		})
		done <- err
	}()

	// 门控生效的标志：确认流被创建并进入 pending（修复前 RunTool 直通执行，
	// 不会有任何 pending 确认）
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
		t.Fatal("RunTool 未创建确认流：破坏性命令被直通执行（安全门失效）")
	}

	// 模拟 dashboard 批准
	if err := loop.confirmStore.Approve(confID, "test-approver"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("批准后执行失败: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("批准后 RunTool 未返回")
	}
}

// adhoc 确认流使用合成任务 ID，不应污染任务表
func TestRunToolAdhocTaskNotPersisted(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "lumen-test.db")
	database, err := agentDB.Init(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer database.Close()

	loop := NewLoop(database, zap.NewNop(), nil)

	done := make(chan error, 1)
	go func() {
		_, err := loop.RunTool(context.Background(), "shell.run", map[string]any{
			"command": "rmdir /tmp/lumen-adhoc-persist-test", "timeout": 10,
		})
		done <- err
	}()

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
		t.Fatal("RunTool 未创建确认流")
	}
	loop.confirmStore.Reject(confID, "test-approver", "test")

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("被拒绝的破坏性命令不应执行成功")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("拒绝后 RunTool 未返回")
	}

	// adhoc 上下文不该写入任务表
	tasks, _, _ := loop.store.ListTasks("", 1, 100)
	for _, tk := range tasks {
		if len(tk.ID) > 6 && tk.ID[:6] == "adhoc-" {
			t.Fatalf("adhoc 任务 %s 不应持久化到任务表", tk.ID)
		}
	}
}
