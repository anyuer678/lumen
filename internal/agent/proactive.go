package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent/internal/memory"
	"agent/internal/task"
)

// ProactiveRunner 主动任务执行器
type ProactiveRunner struct {
	loop     *Loop
	eventBus *EventBus
	policy   *PolicyEngine
}

// NewProactiveRunner 创建主动任务执行器
func NewProactiveRunner(loop *Loop, eventBus *EventBus, policy *PolicyEngine) *ProactiveRunner {
	r := &ProactiveRunner{
		loop:     loop,
		eventBus: eventBus,
		policy:   policy,
	}

	// 注册事件处理器
	eventBus.Subscribe(EventFileCreated, r.onFileCreated)
	eventBus.Subscribe(EventTaskFailed, r.onTaskFailed)

	return r
}

// onFileCreated 文件创建事件处理器
func (r *ProactiveRunner) onFileCreated(event Event) error {
	// 策略检查
	shouldAct, reason, _ := r.policy.Evaluate(event)
	if !shouldAct {
		r.loop.logger.Infof("proactive: file.created ignored (%s)", reason)
		return nil
	}

	// 自动整理文件
	filePath := event.Payload
	ext := strings.ToLower(filepath.Ext(filePath))

	// 只处理特定类型
	switch ext {
	case ".pdf", ".docx", ".xlsx", ".png", ".jpg", ".zip":
		r.loop.logger.Infof("proactive: auto-organizing file %s", filePath)
		// 执行 organize
		result, err := r.loop.RunTool(context.Background(), "fs", map[string]any{
			"action": "organize",
			"path":   filepath.Dir(filePath),
		})
		if err != nil {
			r.loop.logger.Errorf("proactive: organize failed: %v", err)
			return err
		}
		r.loop.logger.Infof("proactive: organize result: %s", result.Summary)
		return nil
	}

	return nil
}

// onTaskFailed 任务失败事件处理器：分析失败原因 → 判断是否可恢复 → 自动重试
func (r *ProactiveRunner) onTaskFailed(event Event) error {
	// 策略检查
	shouldAct, reason, _ := r.policy.Evaluate(event)
	if !shouldAct {
		r.loop.logger.Infof("proactive: task.failed ignored (%s)", reason)
		return nil
	}

	taskID := event.Payload
	if taskID == "" {
		return nil
	}

	r.loop.logger.Infof("proactive: auto-recovery for task %s", taskID)

	// 1. 从 store 加载失败任务
	t, err := r.loop.store.GetTask(taskID)
	if err != nil || t == nil {
		r.loop.logger.Warnf("proactive: cannot load task %s: %v", taskID, err)
		return nil
	}

	// 2. 分析失败原因，判断是否可恢复
	errorMsg := t.Error
	if errorMsg == "" {
		errorMsg = "unknown error"
	}

	recoverable, reason := classifyFailure(errorMsg)
	if !recoverable {
		r.loop.logger.Infof("proactive: task %s failure not recoverable (%s), skipping", taskID, reason)
		return nil
	}

	// 3. 检查重试次数（防止无限循环）
	if t.RetryCount >= 2 {
		r.loop.logger.Infof("proactive: task %s already retried %d times, giving up", taskID, t.RetryCount)
		return nil
	}

	// 4. 创建新任务（带改良目标）
	newGoal := fmt.Sprintf("[自动恢复] %s（第 %d 次重试，前次失败原因: %s）", t.Goal, t.RetryCount+1, reason)
	r.loop.logger.Infof("proactive: creating recovery task for %s → %s", taskID, newGoal)

	// 通过 Agent Loop 执行
	ctx := context.Background()
	newTask := &task.Task{
		ID:         fmt.Sprintf("recovery-%s-%d", taskID, time.Now().UnixMilli()),
		Goal:       newGoal,
		Type:       "retry",
		Status:     task.StatusRunning,
		Priority:   t.Priority,
		RetryCount: t.RetryCount + 1,
	}
	if err := r.loop.store.SaveTask(newTask); err != nil {
		r.loop.logger.Errorf("proactive: failed to save recovery task: %v", err)
		return nil
	}

	// 异步执行
	go func() {
		if err := r.loop.Run(ctx, newTask); err != nil {
			r.loop.logger.Errorf("proactive: recovery task %s failed: %v", newTask.ID, err)
			// 发射失败事件（触发下一轮恢复）
			r.eventBus.EmitAsync(Event{
				Source:   "proactive",
				Type:     EventTaskFailed,
				Payload:  newTask.ID,
				Priority: 5,
			})
		} else {
			r.loop.logger.Infof("proactive: recovery task %s completed", newTask.ID)
		}
	}()

	return nil
}

// classifyFailure 分类失败原因，判断是否可恢复
// 返回 (可恢复, 供人类阅读的原因)
func classifyFailure(errorMsg string) (bool, string) {
	lower := strings.ToLower(errorMsg)

	// 永久性错误（不可恢复）
	permanent := []string{
		"permission denied", "access denied", "access violation",
		"安全拦截", "blocked", "denied",
		"no such file or directory", "文件不存在",
		"unknown tool", "tool not found",
		"subagents cannot nest",
	}
	for _, kw := range permanent {
		if strings.Contains(lower, kw) {
			return false, "permanent: " + kw
		}
	}

	// 可恢复错误（网络/超时/临时不可用）
	recoverable := []string{
		"timeout", "deadline exceeded", "connection refused",
		"context canceled", "i/o timeout",
		"exit status 1", "exit status 2",
		"retry", "temporary",
		"network", "unreachable",
	}
	for _, kw := range recoverable {
		if strings.Contains(lower, kw) {
			return true, "transient: " + kw
		}
	}

	// 未知错误：默认可恢复（给一次机会）
	return true, "unknown error"
}

// RunDailyResearch 定时研究任务
func (r *ProactiveRunner) RunDailyResearch(query string) error {
	// 策略检查
	event := Event{
		Source:   "scheduler",
		Type:     EventScheduleTrigger,
		Payload:  "daily_research",
		Priority: 5,
	}
	shouldAct, reason, _ := r.policy.Evaluate(event)
	if !shouldAct {
		r.loop.logger.Infof("proactive: daily research skipped (%s)", reason)
		return nil
	}

	r.loop.logger.Infof("proactive: running daily research: %s", query)

	// 执行搜索
	result, err := r.loop.RunTool(context.Background(), "browser", map[string]any{
		"action": "research",
		"query":  query,
	})
	if err != nil {
		return fmt.Errorf("research failed: %w", err)
	}

	// 保存到知识库
	if r.loop.kb != nil {
		kb := &memory.Knowledge{
			ID:        fmt.Sprintf("research-%d", time.Now().UnixNano()),
			Title:     "Daily Research: " + query,
			Content:   result.Raw,
			Tags:      "research,daily",
			Source:    "proactive",
			CreatedAt: time.Now(),
		}
		r.loop.kb.Add(kb)
	}

	return nil
}

// GenerateDailyDigest 生成每日摘要
func (r *ProactiveRunner) GenerateDailyDigest() (string, error) {
	if r.loop.memoryStore == nil {
		return "", fmt.Errorf("memory store not initialized")
	}

	// 获取今天完成的任务
	today := time.Now().Format("2006-01-02")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s Daily Digest\n\n", today))

	// 获取最近的记忆
	mems, _ := r.loop.memoryStore.GetRecent(20)
	sb.WriteString(fmt.Sprintf("### 最近记忆 (%d 条)\n", len(mems)))
	for _, m := range mems {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Type, truncateString(m.Content, 60)))
	}

	// 获取事件
	events, _ := r.eventBus.GetRecent(10)
	sb.WriteString(fmt.Sprintf("\n### 最近事件 (%d 条)\n", len(events)))
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", e.Type, e.Source, truncateString(e.Payload, 60)))
	}

	// 获取统计
	memCount, _ := r.loop.memoryStore.GetCount()
	sb.WriteString(fmt.Sprintf("\n### 统计\n- 总记忆数: %d\n", memCount))

	digest := sb.String()

	// 保存到知识库
	if r.loop.kb != nil {
		kb := &memory.Knowledge{
			ID:        fmt.Sprintf("digest-%s", today),
			Title:     fmt.Sprintf("Daily Digest: %s", today),
			Content:   digest,
			Tags:      "digest,daily",
			Source:    "proactive",
			CreatedAt: time.Now(),
		}
		r.loop.kb.Add(kb)
	}

	return digest, nil
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
