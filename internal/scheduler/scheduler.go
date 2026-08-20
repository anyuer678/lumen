package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TriggerType 触发器类型
type TriggerType string

const (
	TriggerCron      TriggerType = "cron"
	TriggerInterval  TriggerType = "interval"
	TriggerAt        TriggerType = "at"
	TriggerFileWatch TriggerType = "file_watch"
	TriggerWebhook   TriggerType = "webhook"
)

// Job 调度任务
type Job struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	TriggerType  TriggerType `json:"trigger_type"`
	CronExpr     string      `json:"cron_expr,omitempty"`
	IntervalSecs int         `json:"interval_secs,omitempty"`
	WatchPath    string      `json:"watch_path,omitempty"`
	GoalTemplate string      `json:"goal_template"`
	Priority     int         `json:"priority"`
	Enabled      bool        `json:"enabled"`
	CatchUp      bool        `json:"catch_up"`
	Concurrency  string      `json:"concurrency"` // skip|queue
	LastRunAt    *time.Time  `json:"last_run_at,omitempty"`
	NextRunAt    *time.Time  `json:"next_run_at,omitempty"`
	LastStatus   string      `json:"last_status,omitempty"`
	MissCount    int         `json:"miss_count"`
	CreatedAt    time.Time   `json:"created_at"`
}

// TaskFactory 任务工厂接口
type TaskFactory interface {
	CreateTask(goal string, priority int) error
}

// Scheduler 调度器
type Scheduler struct {
	store   *Store
	factory TaskFactory
	logger  *zap.SugaredLogger
	ctx     context.Context
	cancel  context.CancelFunc
	jobs    map[string]*Job
	mu      sync.RWMutex
}

// NewScheduler 创建调度器
func NewScheduler(db *sql.DB, factory TaskFactory, logger *zap.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		store:   NewStore(db),
		factory: factory,
		logger:  logger.Sugar(),
		ctx:     ctx,
		cancel:  cancel,
		jobs:    make(map[string]*Job),
	}
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.logger.Info("scheduler started")

	// 加载所有启用的任务
	jobs, err := s.store.ListEnabledJobs()
	if err != nil {
		s.logger.Errorf("failed to load jobs: %v", err)
		return
	}

	for _, job := range jobs {
		s.jobs[job.ID] = job
	}

	// 启动主循环
	go s.run()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.cancel()
	s.logger.Info("scheduler stopped")
}

// run 主循环
func (s *Scheduler) run() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// tick 执行一次检查
func (s *Scheduler) tick() {
	now := time.Now()
	s.mu.RLock()
	var toFire []*Job
	for _, job := range s.jobs {
		if job.Enabled && job.NextRunAt != nil && now.After(*job.NextRunAt) {
			toFire = append(toFire, job)
		}
	}
	s.mu.RUnlock() // 先释放锁再启动 goroutine

	for _, job := range toFire {
		go s.fire(job)
	}
}

// fire 触发任务（支持 payload）
func (s *Scheduler) fire(job *Job, payload ...map[string]interface{}) {
	s.logger.Infof("firing job: %s", job.Name)

	// 渲染目标模板
	var data map[string]interface{}
	if len(payload) > 0 {
		data = payload[0]
	}
	goal := s.renderTemplate(job.GoalTemplate, data)

	// 创建任务
	var status string
	if err := s.factory.CreateTask(goal, job.Priority); err != nil {
		s.logger.Errorf("failed to create task for job %s: %v", job.ID, err)
		status = "error"
	} else {
		status = "fired"
	}

	// 更新状态（持写锁，避免与 tick/ListJobs 并发读冲突）
	now := time.Now()
	s.mu.Lock()
	job.LastStatus = status
	job.LastRunAt = &now
	job.NextRunAt = s.computeNextRun(job)
	s.mu.Unlock()
	s.store.UpdateJob(job)
}

// renderTemplate 渲染模板
func (s *Scheduler) renderTemplate(template string, data map[string]interface{}) string {
	// 简化版：替换 {{date}} 变量
	result := template
	result = replaceAll(result, "{{date}}", time.Now().Format("2006-01-02"))
	result = replaceAll(result, "{{datetime}}", time.Now().Format("2006-01-02T15:04:05"))

	// 替换任意 payload 字段 {{field}}
	if data != nil {
		for k, v := range data {
			result = replaceAll(result, "{{"+k+"}}", fmt.Sprintf("%v", v))
		}
	}
	return result
}

// computeNextRun 计算下次运行时间
func (s *Scheduler) computeNextRun(job *Job) *time.Time {
	now := time.Now()

	switch job.TriggerType {
	case TriggerCron:
		// 简化版：每天运行一次
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 8, 0, 0, 0, now.Location())
		return &next
	case TriggerInterval:
		next := now.Add(time.Duration(job.IntervalSecs) * time.Second)
		return &next
	case TriggerAt:
		return nil // 一次性任务
	default:
		return nil
	}
}

// CreateJob 创建任务
func (s *Scheduler) CreateJob(job *Job) error {
	if err := s.store.SaveJob(job); err != nil {
		return err
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return nil
}

// TriggerByWebhook 通过 webhook 触发指定 job
func (s *Scheduler) TriggerByWebhook(jobID string, payload map[string]interface{}) (bool, string) {
	s.mu.RLock()
	job, ok := s.jobs[jobID]
	s.mu.RUnlock()

	if !ok {
		return false, "job not found"
	}
	if job.TriggerType != TriggerWebhook {
		return false, "job is not webhook type"
	}
	if !job.Enabled {
		return false, "job is disabled"
	}

	go s.fire(job, payload)
	return true, "triggered"
}

// AddFileWatch 为 file_watch 类型的 job 注册监听目录
// 返回需要监听的目录集合
func (s *Scheduler) FileWatchTargets() map[string][]*Job {
	targets := make(map[string][]*Job)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, job := range s.jobs {
		if !job.Enabled || job.TriggerType != TriggerFileWatch || job.WatchPath == "" {
			continue
		}
		targets[job.WatchPath] = append(targets[job.WatchPath], job)
	}
	return targets
}

// DeleteJob 删除任务
func (s *Scheduler) DeleteJob(id string) error {
	if err := s.store.DeleteJob(id); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()

	return nil
}

// ListJobs 列出任务
func (s *Scheduler) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jobs []*Job
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// replaceAll 字符串替换
func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

// indexOf 查找子串位置
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
