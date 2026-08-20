package task

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Checkpoint 断点
type Checkpoint struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	StepSeq   int       `json:"step_seq"`
	PlanJSON  string    `json:"plan_json"`
	StateJSON string    `json:"state_json"`
	CreatedAt time.Time `json:"created_at"`
}

// CheckpointStore 断点存储
type CheckpointStore struct {
	db *sql.DB
}

// NewCheckpointStore 创建存储
func NewCheckpointStore(db *sql.DB) *CheckpointStore {
	return &CheckpointStore{db: db}
}

// Save 保存断点
func (s *CheckpointStore) Save(taskID string, stepSeq int, plan any, state any) error {
	planJSON, _ := json.Marshal(plan)

	// 使用 working_memory 表存储断点
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO working_memory (task_id, resume_step, last_summary, interrupted_at, restore_count)
		VALUES (?, ?, ?, ?, 0)`,
		taskID, stepSeq, string(planJSON), time.Now())
	return err
}

// Load 加载断点
func (s *CheckpointStore) Load(taskID string) (*Checkpoint, error) {
	cp := &Checkpoint{TaskID: taskID}
	var summary string
	var interruptedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT task_id, resume_step, last_summary, interrupted_at
		FROM working_memory WHERE task_id = ?`, taskID).Scan(
		&cp.TaskID, &cp.StepSeq, &summary, &interruptedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	cp.PlanJSON = summary
	if interruptedAt.Valid {
		cp.CreatedAt = interruptedAt.Time
	}
	return cp, nil
}

// Delete 删除断点（任务完成后）
func (s *CheckpointStore) Delete(taskID string) error {
	_, err := s.db.Exec(`DELETE FROM working_memory WHERE task_id = ?`, taskID)
	return err
}

// SaveStepCheckpoint 保存步骤级断点（每步执行完保存）
func SaveStepCheckpoint(db *sql.DB, taskID string, stepSeq int) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO working_memory (task_id, resume_step, last_summary, interrupted_at, restore_count)
		VALUES (?, ?, ?, ?, 0)`,
		taskID, stepSeq, "", time.Now())
	return err
}

// RecoverFromCheckpoint 从断点恢复任务
func RecoverFromCheckpoint(db *sql.DB, taskID string) (int, bool, error) {
	var stepSeq int
	var summary string
	err := db.QueryRow(`
		SELECT resume_step, last_summary FROM working_memory WHERE task_id = ?`, taskID).Scan(&stepSeq, &summary)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, err
	}

	// 增加恢复次数
	db.Exec(`UPDATE working_memory SET restore_count = restore_count + 1 WHERE task_id = ?`, taskID)

	return stepSeq, true, nil
}

// 接入 Agent Loop：在每个步骤执行完后保存 checkpoint
func SaveStepAfterExecute(db *sql.DB, taskID string, stepSeq int) {
	SaveStepCheckpoint(db, taskID, stepSeq)
}
