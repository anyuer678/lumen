package task

import (
	"container/heap"
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Executor 任务执行器接口
type Executor interface {
	Run(ctx context.Context, task *Task) error
}

// EventPublisher 事件发布器接口
type EventPublisher interface {
	Publish(eventType string, data interface{})
}

// Manager 任务管理器
type Manager struct {
	store      *Store
	queue      *PriorityQueue
	workerCh   chan struct{}
	maxWorkers int
	logger     *zap.SugaredLogger
	ctx        context.Context
	cancel     context.CancelFunc
	running    map[string]context.CancelFunc
	mu         sync.RWMutex
	executor   Executor
	publisher  EventPublisher
}

// NewManager 创建管理器
func NewManager(db *sql.DB, maxWorkers int, logger *zap.Logger, executor Executor, publisher EventPublisher) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		store:      NewStore(db),
		queue:      &PriorityQueue{},
		maxWorkers: maxWorkers,
		logger:     logger.Sugar(),
		ctx:        ctx,
		cancel:     cancel,
		running:    make(map[string]context.CancelFunc),
		executor:   executor,
		publisher:  publisher,
	}
	heap.Init(m.queue)
	return m
}

// CreateTask 创建任务并入队
func (m *Manager) CreateTask(goal string, opts ...Option) (*Task, error) {
	t, err := CreateTask(m.store.db, goal, opts...)
	if err != nil {
		return nil, err
	}

	// 转入 queued 状态
	if err := m.store.TransitionStatus(t.ID, StatusQueued, ""); err != nil {
		return nil, fmt.Errorf("transition to queued: %w", err)
	}
	t.Status = StatusQueued

	// 入队
	heap.Push(m.queue, t)

	return t, nil
}

// CreateTaskFromScheduler 供调度器调用的工厂方法（实现 scheduler.TaskFactory）
func (m *Manager) CreateTaskFromScheduler(goal string, priority int) error {
	_, err := m.CreateTask(goal, WithPriority(priority), WithType("scheduled"))
	return err
}

// Start 启动工作循环
func (m *Manager) Start() {
	m.workerCh = make(chan struct{}, m.maxWorkers)

	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			default:
				if m.queue.Len() == 0 {
					time.Sleep(100 * time.Millisecond)
					continue
				}

				// 获取最高优先级任务
				m.mu.Lock()
				if m.queue.Len() == 0 {
					m.mu.Unlock()
					continue
				}
				t := heap.Pop(m.queue).(*Task)
				m.mu.Unlock()

				// 并发控制
				m.workerCh <- struct{}{}
				go func(task *Task) {
					defer func() { <-m.workerCh }()
					m.executeTask(task)
				}(t)
			}
		}
	}()
}

// Stop 停止所有任务
func (m *Manager) Stop() {
	m.cancel()

	// 停止所有运行中的任务
	m.mu.RLock()
	for id, cancel := range m.running {
		cancel()
		m.logger.Infof("stopping task: %s", id)
	}
	m.mu.RUnlock()
}

// PauseTask 暂停任务
func (m *Manager) PauseTask(id string, reason string) error {
	m.mu.Lock()
	if cancel, ok := m.running[id]; ok {
		cancel()
		delete(m.running, id)
	}
	m.mu.Unlock()

	return m.store.TransitionStatus(id, StatusPaused, reason)
}

// ResumeTask 恢复任务
func (m *Manager) ResumeTask(id string) error {
	t, err := m.store.GetTask(id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", id)
	}
	if t.Status != StatusPaused {
		return fmt.Errorf("task is not paused: %s", t.Status)
	}

	// 转入 queued 状态
	if err := m.store.TransitionStatus(id, StatusQueued, ""); err != nil {
		return err
	}

	// 重新入队
	heap.Push(m.queue, t)
	return nil
}

// StopTask 终止任务
func (m *Manager) StopTask(id string) error {
	m.mu.Lock()
	if cancel, ok := m.running[id]; ok {
		cancel()
		delete(m.running, id)
	}
	m.mu.Unlock()

	return m.store.TransitionStatus(id, StatusCancelled, "")
}

// RetryTask 重试任务
func (m *Manager) RetryTask(id string) error {
	t, err := m.store.GetTask(id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", id)
	}
	if t.Status != StatusFailed {
		return fmt.Errorf("task is not failed: %s", t.Status)
	}

	// 重置状态
	t.RetryCount++
	t.CurrentStep = 0
	t.Progress = 0
	t.Result = ""
	t.Error = ""

	// 更新数据库
	if err := m.store.SaveTask(t); err != nil {
		return err
	}

	// 转入 queued 状态
	if err := m.store.TransitionStatus(id, StatusQueued, ""); err != nil {
		return err
	}

	// 重新入队
	heap.Push(m.queue, t)
	return nil
}

// executeTask 执行任务
func (m *Manager) executeTask(t *Task) {
	taskCtx, cancel := context.WithCancel(m.ctx)
	m.mu.Lock()
	m.running[t.ID] = cancel
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.running, t.ID)
		m.mu.Unlock()
	}()

	// 转入 running 状态
	if err := m.store.TransitionStatus(t.ID, StatusRunning, ""); err != nil {
		m.logger.Errorf("failed to transition task %s to running: %v", t.ID, err)
		return
	}

	// 广播任务开始事件
	if m.publisher != nil {
		m.publisher.Publish("task.started", map[string]interface{}{
			"task_id": t.ID,
			"goal":    t.Goal,
		})
	}

	// 调用 Agent Loop
	if err := m.executor.Run(taskCtx, t); err != nil {
		m.logger.Errorf("agent loop failed for task %s: %v", t.ID, err)
		m.store.SetResult(t.ID, "", err.Error())
		m.store.TransitionStatus(t.ID, StatusFailed, "")
		// 广播任务失败事件
		if m.publisher != nil {
			m.publisher.Publish("task.failed", map[string]interface{}{
				"task_id": t.ID,
				"error":   err.Error(),
			})
		}
		return
	}

	// 标记完成
	if err := m.store.TransitionStatus(t.ID, StatusCompleted, ""); err != nil {
		m.logger.Errorf("failed to transition task %s to completed: %v", t.ID, err)
	}
	// 广播任务完成事件
	if m.publisher != nil {
		m.publisher.Publish("task.completed", map[string]interface{}{
			"task_id": t.ID,
		})
	}
}

// GetTask 获取任务
func (m *Manager) GetTask(id string) (*Task, error) {
	return m.store.GetTask(id)
}

// ListTasks 列出任务
func (m *Manager) ListTasks(status string, page, limit int) ([]*Task, int, error) {
	return m.store.ListTasks(status, page, limit)
}

// GetSteps 获取任务步骤
func (m *Manager) GetSteps(taskID string) ([]Step, error) {
	return m.store.GetSteps(taskID)
}

// RecoverTasks 进程重启恢复
func (m *Manager) RecoverTasks() (int, error) {
	return m.store.RecoverTasks()
}

// RollbackTaskToStep 回滚任务到指定步骤
func (m *Manager) RollbackTaskToStep(id string, toStep int) error {
	t, err := m.store.GetTask(id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", id)
	}
	// 只能回滚已结束或已暂停的任务
	if t.Status != StatusFailed && t.Status != StatusCompleted && t.Status != StatusPaused && t.Status != StatusCancelled {
		return fmt.Errorf("cannot rollback task in status %s", t.Status)
	}
	return m.store.RollbackTaskToStep(id, toStep)
}

// SaveCheckpoint 保存检查点
func (s *Store) SaveCheckpoint(taskID string, stepSeq int) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO working_memory (task_id, resume_step, last_summary, interrupted_at, restore_count)
		VALUES (?, ?, '', ?, 0)`,
		taskID, stepSeq, time.Now())
	return err
}

// ClearTasks 清理任务
func (m *Manager) ClearTasks(keepRunning bool) (int, error) {
	return m.store.ClearTasks(keepRunning)
}

// QueueDepth 队列深度
func (m *Manager) QueueDepth() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.queue.Len()
}

// RunningCount 运行中任务数
func (m *Manager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.running)
}
