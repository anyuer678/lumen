package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// WorkingMemory 工作记忆（恢复点）
type WorkingMemory struct {
	TaskID        string     `json:"task_id"`
	ResumeStep    int        `json:"resume_step"`
	LastSummary   string     `json:"last_summary,omitempty"`
	InterruptedAt *time.Time `json:"interrupted_at,omitempty"`
	RestoreCount  int        `json:"restore_count"`
}

// WorkingStore 工作记忆存储
type WorkingStore struct {
	db *sql.DB
}

// NewWorkingStore 创建工作记忆存储
func NewWorkingStore(db *sql.DB) *WorkingStore {
	return &WorkingStore{db: db}
}

// Save 保存工作记忆
func (s *WorkingStore) Save(wm *WorkingMemory) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO working_memory 
		(task_id, resume_step, last_summary, interrupted_at, restore_count)
		VALUES (?, ?, ?, ?, ?)`,
		wm.TaskID, wm.ResumeStep, wm.LastSummary, wm.InterruptedAt, wm.RestoreCount,
	)
	return err
}

// Get 获取工作记忆
func (s *WorkingStore) Get(taskID string) (*WorkingMemory, error) {
	wm := &WorkingMemory{}
	var interruptedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT task_id, resume_step, last_summary, interrupted_at, restore_count
		FROM working_memory WHERE task_id = ?`, taskID).Scan(
		&wm.TaskID, &wm.ResumeStep, &wm.LastSummary, &interruptedAt, &wm.RestoreCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query working memory: %w", err)
	}

	if interruptedAt.Valid {
		wm.InterruptedAt = &interruptedAt.Time
	}

	return wm, nil
}

// Delete 删除工作记忆
func (s *WorkingStore) Delete(taskID string) error {
	_, err := s.db.Exec("DELETE FROM working_memory WHERE task_id = ?", taskID)
	return err
}

// ListPending 列出待恢复的任务
func (s *WorkingStore) ListPending() ([]*WorkingMemory, error) {
	rows, err := s.db.Query(`
		SELECT task_id, resume_step, last_summary, interrupted_at, restore_count
		FROM working_memory ORDER BY interrupted_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*WorkingMemory
	for rows.Next() {
		wm := &WorkingMemory{}
		var interruptedAt sql.NullTime

		if err := rows.Scan(
			&wm.TaskID, &wm.ResumeStep, &wm.LastSummary, &interruptedAt, &wm.RestoreCount,
		); err != nil {
			return nil, err
		}

		if interruptedAt.Valid {
			wm.InterruptedAt = &interruptedAt.Time
		}

		memories = append(memories, wm)
	}
	return memories, nil
}
