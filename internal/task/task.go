package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	StatusPending         TaskStatus = "pending"
	StatusQueued          TaskStatus = "queued"
	StatusRunning         TaskStatus = "running"
	StatusPaused          TaskStatus = "paused"
	StatusWaitingConfirm  TaskStatus = "waiting_confirm"
	StatusCompleted       TaskStatus = "completed"
	StatusFailed          TaskStatus = "failed"
	StatusCancelled       TaskStatus = "cancelled"
)

// 合法状态转移
var validTransitions = map[TaskStatus][]TaskStatus{
	StatusPending:        {StatusQueued, StatusCancelled},
	StatusQueued:         {StatusRunning, StatusCancelled},
	StatusRunning:        {StatusPaused, StatusWaitingConfirm, StatusCompleted, StatusFailed, StatusCancelled},
	StatusPaused:         {StatusRunning, StatusCancelled},
	StatusWaitingConfirm: {StatusRunning, StatusFailed, StatusCancelled},
	StatusCompleted:      {},
	StatusFailed:         {StatusQueued},
	StatusCancelled:      {},
}

// Task 任务模型
type Task struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`         // manual|scheduled|webhook|file_trigger|retry
	Goal        string     `json:"goal"`
	Status      TaskStatus `json:"status"`
	Progress    float64    `json:"progress"`
	CurrentStep int        `json:"current_step"`
	Priority    int        `json:"priority"`
	PermLevel   int        `json:"perm_level"`
	Owner       string     `json:"owner"`
	SessionID   string     `json:"session_id,omitempty"`
	RetryCount  int        `json:"retry_count"`
	MaxRetries  int        `json:"max_retries"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	PlanJSON    string     `json:"-"`
	PauseReason string     `json:"pause_reason,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Steps       []Step     `json:"steps,omitempty"`
}

// Step 步骤模型
type Step struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"task_id"`
	Seq         int        `json:"seq"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Tool        string     `json:"tool,omitempty"`
	ArgsJSON    string     `json:"-"`
	Result      string     `json:"result,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	Retries     int        `json:"retries"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// CreateTask 创建任务
func CreateTask(db *sql.DB, goal string, opts ...Option) (*Task, error) {
	t := &Task{
		ID:         "t-" + uuid.New().String()[:8],
		Type:       "manual",
		Goal:       goal,
		Status:     StatusPending,
		Priority:   5,
		MaxRetries: 3,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	for _, opt := range opts {
		opt(t)
	}

	_, err := db.Exec(`
		INSERT INTO tasks (id, type, goal, status, progress, current_step, priority, perm_level, owner, retry_count, max_retries, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Type, t.Goal, t.Status, t.Progress, t.CurrentStep,
		t.Priority, t.PermLevel, t.Owner, t.RetryCount, t.MaxRetries,
		t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	return t, nil
}

// Option 任务选项
type Option func(*Task)

func WithType(t string) Option  { return func(task *Task) { task.Type = t } }
func WithPriority(p int) Option { return func(task *Task) { task.Priority = p } }
func WithOwner(o string) Option { return func(task *Task) { task.Owner = o } }
func WithMaxRetries(n int) Option { return func(task *Task) { task.MaxRetries = n } }

// GetTask 获取任务
func GetTask(db *sql.DB, id string) (*Task, error) {
	t := &Task{}
	var startedAt, finishedAt sql.NullTime

	err := db.QueryRow(`
		SELECT id, type, goal, status, progress, current_step, priority, perm_level, owner,
		       retry_count, max_retries, result, error, pause_reason, created_at, started_at, finished_at, updated_at
		FROM tasks WHERE id = ?`, id).Scan(
		&t.ID, &t.Type, &t.Goal, &t.Status, &t.Progress, &t.CurrentStep,
		&t.Priority, &t.PermLevel, &t.Owner, &t.RetryCount, &t.MaxRetries,
		&t.Result, &t.Error, &t.PauseReason, &t.CreatedAt, &startedAt, &finishedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query task: %w", err)
	}

	if startedAt.Valid {
		t.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		t.FinishedAt = &finishedAt.Time
	}

	return t, nil
}

// ListTasks 列出任务
func ListTasks(db *sql.DB, status string, page, limit int) ([]*Task, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var total int
	countSQL := "SELECT COUNT(*) FROM tasks"
	args := []interface{}{}
	if status != "" {
		countSQL += " WHERE status = ?"
		args = append(args, status)
	}
	if err := db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	querySQL := "SELECT id, type, goal, status, progress, current_step, priority, created_at FROM tasks"
	if status != "" {
		querySQL += " WHERE status = ?"
	}
	querySQL += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, (page-1)*limit)

	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(&t.ID, &t.Type, &t.Goal, &t.Status, &t.Progress, &t.CurrentStep, &t.Priority, &t.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}

	return tasks, total, nil
}

// TransitionStatus 状态转移
func TransitionStatus(db *sql.DB, id string, newStatus TaskStatus, reason string) error {
	t, err := GetTask(db, id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", id)
	}

	// 检查合法转移
	allowed := validTransitions[t.Status]
	valid := false
	for _, s := range allowed {
		if s == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s -> %s", t.Status, newStatus)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":     newStatus,
		"updated_at": now,
	}

	if newStatus == StatusRunning && t.StartedAt == nil {
		updates["started_at"] = now
	}
	if newStatus == StatusPaused {
		updates["pause_reason"] = reason
	}
	if newStatus == StatusCompleted || newStatus == StatusFailed || newStatus == StatusCancelled {
		updates["finished_at"] = now
	}

	// 构建 UPDATE
	query := "UPDATE tasks SET status = ?, updated_at = ?"
	args := []interface{}{newStatus, now}
	if newStatus == StatusRunning && t.StartedAt == nil {
		query += ", started_at = ?"
		args = append(args, now)
	}
	if newStatus == StatusPaused && reason != "" {
		query += ", pause_reason = ?"
		args = append(args, reason)
	}
	if newStatus == StatusCompleted || newStatus == StatusFailed || newStatus == StatusCancelled {
		query += ", finished_at = ?"
		args = append(args, now)
	}
	query += " WHERE id = ?"
	args = append(args, id)

	_, err = db.Exec(query, args...)
	return err
}

// UpdateProgress 更新进度
func UpdateProgress(db *sql.DB, id string, progress float64, currentStep int) error {
	_, err := db.Exec("UPDATE tasks SET progress = ?, current_step = ?, updated_at = ? WHERE id = ?",
		progress, currentStep, time.Now(), id)
	return err
}

// SetResult 设置结果
func SetResult(db *sql.DB, id string, result, errText string) error {
	_, err := db.Exec("UPDATE tasks SET result = ?, error = ?, updated_at = ? WHERE id = ?",
		result, errText, time.Now(), id)
	return err
}

// RecoverTasks 进程重启恢复
func RecoverTasks(db *sql.DB) (int, error) {
	result, err := db.Exec(`
		UPDATE tasks SET status = 'paused', pause_reason = 'process_restarted', updated_at = ?
		WHERE status = 'running'`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("recover tasks: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ToJSON 序列化计划
func (t *Task) PlanJSONBytes() []byte {
	if t.PlanJSON == "" {
		return nil
	}
	return []byte(t.PlanJSON)
}

// GetPlan 获取任务计划
func GetPlan(db *sql.DB, id string) ([]byte, error) {
	var planJSON sql.NullString
	err := db.QueryRow("SELECT plan_json FROM tasks WHERE id = ?", id).Scan(&planJSON)
	if err != nil {
		return nil, err
	}
	if !planJSON.Valid {
		return nil, nil
	}
	return []byte(planJSON.String), nil
}

// SavePlan 保存计划
func SavePlan(db *sql.DB, id string, plan interface{}) error {
	b, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE tasks SET plan_json = ?, updated_at = ? WHERE id = ?", string(b), time.Now(), id)
	return err
}
