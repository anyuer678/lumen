package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Store 任务存储
type Store struct {
	db *sql.DB
}

// NewStore 创建存储
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveTask 保存任务
func (s *Store) SaveTask(t *Task) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO tasks 
		(id, type, goal, status, progress, current_step, priority, perm_level, owner, session_id, 
		 retry_count, max_retries, result, error, plan_json, pause_reason, created_at, started_at, finished_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Type, t.Goal, t.Status, t.Progress, t.CurrentStep,
		t.Priority, t.PermLevel, t.Owner, t.SessionID,
		t.RetryCount, t.MaxRetries, t.Result, t.Error, t.PlanJSON, t.PauseReason,
		t.CreatedAt, t.StartedAt, t.FinishedAt, t.UpdatedAt,
	)
	return err
}

// GetTask 获取任务
func (s *Store) GetTask(id string) (*Task, error) {
	t := &Task{}
	var startedAt, finishedAt sql.NullTime
	var result, errStr, pauseReason, planJSON, sessionID sql.NullString

	err := s.db.QueryRow(`
		SELECT id, type, goal, status, progress, current_step, priority, perm_level, owner, session_id,
		       retry_count, max_retries, result, error, plan_json, pause_reason, 
		       created_at, started_at, finished_at, updated_at
		FROM tasks WHERE id = ?`, id).Scan(
		&t.ID, &t.Type, &t.Goal, &t.Status, &t.Progress, &t.CurrentStep,
		&t.Priority, &t.PermLevel, &t.Owner, &sessionID,
		&t.RetryCount, &t.MaxRetries, &result, &errStr, &planJSON, &pauseReason,
		&t.CreatedAt, &startedAt, &finishedAt, &t.UpdatedAt,
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
	if result.Valid {
		t.Result = result.String
	}
	if errStr.Valid {
		t.Error = errStr.String
	}
	if pauseReason.Valid {
		t.PauseReason = pauseReason.String
	}
	if planJSON.Valid {
		t.PlanJSON = planJSON.String
	}
	if sessionID.Valid {
		t.SessionID = sessionID.String
	}

	return t, nil
}

// ListTasks 列出任务
func (s *Store) ListTasks(status string, page, limit int) ([]*Task, int, error) {
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
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	querySQL := "SELECT id, type, goal, status, progress, current_step, priority, created_at FROM tasks"
	if status != "" {
		querySQL += " WHERE status = ?"
	}
	querySQL += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, (page-1)*limit)

	rows, err := s.db.Query(querySQL, args...)
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
func (s *Store) TransitionStatus(id string, newStatus TaskStatus, reason string) error {
	t, err := s.GetTask(id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("task not found: %s", id)
	}

	// 检查合法转移
	allowed := validTransitions[t.Status]
	valid := false
	for _, st := range allowed {
		if st == newStatus {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s -> %s", t.Status, newStatus)
	}

	now := time.Now()
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

	_, err = s.db.Exec(query, args...)
	return err
}

// UpdateProgress 更新进度
func (s *Store) UpdateProgress(id string, progress float64, currentStep int) error {
	_, err := s.db.Exec("UPDATE tasks SET progress = ?, current_step = ?, updated_at = ? WHERE id = ?",
		progress, currentStep, time.Now(), id)
	return err
}

// SetResult 设置结果
func (s *Store) SetResult(id, result, errText string) error {
	_, err := s.db.Exec("UPDATE tasks SET result = ?, error = ?, updated_at = ? WHERE id = ?",
		result, errText, time.Now(), id)
	return err
}

// SetStatus 直接设置任务状态
func (s *Store) SetStatus(id string, status TaskStatus) error {
	_, err := s.db.Exec("UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
		status, time.Now(), id)
	return err
}

// SaveStep 保存步骤
func (s *Store) SaveStep(step *Step) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO steps 
		(id, task_id, seq, description, status, tool, args_json, result, summary, retries, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.TaskID, step.Seq, step.Description, step.Status,
		step.Tool, step.ArgsJSON, step.Result, step.Summary, step.Retries,
		step.StartedAt, step.FinishedAt,
	)
	return err
}

// GetSteps 获取任务的所有步骤
func (s *Store) GetSteps(taskID string) ([]Step, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, seq, description, status, tool, args_json, result, summary, retries, started_at, finished_at
		FROM steps WHERE task_id = ? ORDER BY seq`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []Step
	for rows.Next() {
		var step Step
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(&step.ID, &step.TaskID, &step.Seq, &step.Description, &step.Status,
			&step.Tool, &step.ArgsJSON, &step.Result, &step.Summary, &step.Retries,
			&startedAt, &finishedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			step.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			step.FinishedAt = &finishedAt.Time
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// SavePlan 保存计划
func (s *Store) SavePlan(taskID string, plan interface{}) error {
	b, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE tasks SET plan_json = ?, updated_at = ? WHERE id = ?", string(b), time.Now(), taskID)
	return err
}

// LoadCheckpoint 读取任务断点（resume_step）
func (s *Store) LoadCheckpoint(taskID string) (int, error) {
	var step int
	err := s.db.QueryRow(`SELECT resume_step FROM working_memory WHERE task_id = ?`, taskID).Scan(&step)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return step, nil
}

// RollbackTaskToStep 回滚任务到指定步骤（删除该步之后的步骤并置为 paused）
func (s *Store) RollbackTaskToStep(taskID string, toStep int) error {
	if err := RollbackTask(s.db, taskID, toStep); err != nil {
		return err
	}
	// 清理断点记录（下次 resume 从头开始该步）
	_, _ = s.db.Exec("DELETE FROM working_memory WHERE task_id = ?", taskID)
	return nil
}

// GetPlan 获取计划
func (s *Store) GetPlan(taskID string) ([]byte, error) {
	var planJSON sql.NullString
	err := s.db.QueryRow("SELECT plan_json FROM tasks WHERE id = ?", taskID).Scan(&planJSON)
	if err != nil {
		return nil, err
	}
	if !planJSON.Valid {
		return nil, nil
	}
	return []byte(planJSON.String), nil
}

// RecoverTasks 进程重启恢复
func (s *Store) RecoverTasks() (int, error) {
	result, err := s.db.Exec(`
		UPDATE tasks SET status = 'paused', pause_reason = 'process_restarted', updated_at = ?
		WHERE status = 'running'`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("recover tasks: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// GetTasksByStatus 按状态获取任务
func (s *Store) GetTasksByStatus(status string) ([]*Task, error) {
	rows, err := s.db.Query(`
		SELECT id, type, goal, status, progress, current_step, priority, created_at, updated_at
		FROM tasks WHERE status = ? ORDER BY created_at`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t := &Task{}
		if err := rows.Scan(&t.ID, &t.Type, &t.Goal, &t.Status, &t.Progress, &t.CurrentStep, &t.Priority, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ClearTasks 清理任务（保留进行中的任务）
func (s *Store) ClearTasks(keepRunning bool) (int, error) {
	var query string
	var args []interface{}

	if keepRunning {
		// 只清理终态任务（completed/failed/cancelled）
		query = `DELETE FROM tasks WHERE status IN ('completed', 'failed', 'cancelled')`
	} else {
		query = `DELETE FROM tasks`
	}

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("clear tasks: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
