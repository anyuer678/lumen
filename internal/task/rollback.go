package task

import (
	"database/sql"
	"fmt"
	"time"
)

// RollbackPoint 回滚点
type RollbackPoint struct {
	TaskID    string    `json:"task_id"`
	StepSeq   int       `json:"step_seq"`
	State     string    `json:"state"` // JSON 序列化的任务状态
	CreatedAt time.Time `json:"created_at"`
}

// RollbackStore 回滚存储
type RollbackStore struct {
	db *sql.DB
}

// NewRollbackStore 创建回滚存储
func NewRollbackStore(db *sql.DB) *RollbackStore {
	return &RollbackStore{db: db}
}

// SaveRollbackPoint 保存回滚点
func (s *RollbackStore) SaveRollbackPoint(taskID string, stepSeq int, state string) error {
	_, err := s.db.Exec(`
		INSERT INTO working_memory (task_id, resume_step, last_summary, interrupted_at, restore_count)
		VALUES (?, ?, ?, ?, 0)
		ON CONFLICT(task_id) DO UPDATE SET
			resume_step = excluded.resume_step,
			last_summary = excluded.last_summary,
			interrupted_at = excluded.interrupted_at`,
		taskID, stepSeq, state, time.Now())
	return err
}

// GetRollbackPoint 获取回滚点
func (s *RollbackStore) GetRollbackPoint(taskID string) (*RollbackPoint, error) {
	rp := &RollbackPoint{}
	var interruptedAt sql.NullTime
	var lastSummary sql.NullString

	err := s.db.QueryRow(`
		SELECT task_id, resume_step, last_summary, interrupted_at
		FROM working_memory WHERE task_id = ?`, taskID).Scan(
		&rp.TaskID, &rp.StepSeq, &lastSummary, &interruptedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query rollback point: %w", err)
	}

	if interruptedAt.Valid {
		rp.CreatedAt = interruptedAt.Time
	}
	if lastSummary.Valid {
		rp.State = lastSummary.String
	}

	return rp, nil
}

// DeleteRollbackPoint 删除回滚点
func (s *RollbackStore) DeleteRollbackPoint(taskID string) error {
	_, err := s.db.Exec("DELETE FROM working_memory WHERE task_id = ?", taskID)
	return err
}

// RollbackTask 回滚任务到指定步骤
func RollbackTask(db *sql.DB, taskID string, toStep int) error {
	// 1. 删除 toStep 之后的步骤
	_, err := db.Exec("DELETE FROM steps WHERE task_id = ? AND seq >= ?", taskID, toStep)
	if err != nil {
		return fmt.Errorf("delete steps: %w", err)
	}

	// 2. 更新任务状态
	_, err = db.Exec(`
		UPDATE tasks SET 
			status = 'paused',
			current_step = ?,
			progress = 0,
			pause_reason = 'rollback',
			updated_at = ?
		WHERE id = ?`,
		toStep, time.Now(), taskID)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	return nil
}
