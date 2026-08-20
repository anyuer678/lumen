package task

import (
	"database/sql"
	"encoding/json"
	"time"
)

// WorkflowStep 工作流步骤
type WorkflowStep struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Tool        string            `json:"tool"`
	Args        map[string]any    `json:"args"`
	DependsOn   []string          `json:"depends_on"`   // 依赖的前置步骤 ID
	Status      string            `json:"status"`       // pending/running/completed/failed
	Result      string            `json:"result"`
	Error       string            `json:"error"`
	StartedAt   *time.Time        `json:"started_at"`
	FinishedAt  *time.Time        `json:"finished_at"`
}

// Workflow 工作流定义
type Workflow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Steps       []WorkflowStep  `json:"steps"`
	Status      string          `json:"status"` // pending/running/completed/failed
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// WorkflowStore 工作流存储
type WorkflowStore struct {
	db *sql.DB
}

// NewWorkflowStore 创建存储
func NewWorkflowStore(db *sql.DB) *WorkflowStore {
	return &WorkflowStore{db: db}
}

// Save 保存工作流
func (s *WorkflowStore) Save(wf *Workflow) error {
	stepsJSON, _ := json.Marshal(wf.Steps)
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO workflows (id, name, description, steps_json, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		wf.ID, wf.Name, wf.Description, stepsJSON, wf.Status, wf.CreatedAt, wf.UpdatedAt)
	return err
}

// Get 获取工作流
func (s *WorkflowStore) Get(id string) (*Workflow, error) {
	wf := &Workflow{}
	var stepsJSON string
	err := s.db.QueryRow(`
		SELECT id, name, description, steps_json, status, created_at, updated_at
		FROM workflows WHERE id = ?`, id).Scan(
		&wf.ID, &wf.Name, &wf.Description, &stepsJSON, &wf.Status, &wf.CreatedAt, &wf.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal([]byte(stepsJSON), &wf.Steps)
	return wf, nil
}

// List 列出工作流
func (s *WorkflowStore) List(limit int) ([]*Workflow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, name, description, steps_json, status, created_at, updated_at
		FROM workflows ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []*Workflow
	for rows.Next() {
		wf := &Workflow{}
		var stepsJSON string
		rows.Scan(&wf.ID, &wf.Name, &wf.Description, &stepsJSON, &wf.Status, &wf.CreatedAt, &wf.UpdatedAt)
		json.Unmarshal([]byte(stepsJSON), &wf.Steps)
		workflows = append(workflows, wf)
	}
	return workflows, nil
}

// UpdateStatus 更新工作流状态
func (s *WorkflowStore) UpdateStatus(id string, status string) error {
	_, err := s.db.Exec("UPDATE workflows SET status = ?, updated_at = ? WHERE id = ?",
		status, time.Now(), id)
	return err
}

// UpdateStepStatus 更新步骤状态
func (s *WorkflowStore) UpdateStepStatus(workflowID string, stepID string, status string, result string, errText string) error {
	wf, err := s.Get(workflowID)
	if err != nil || wf == nil {
		return err
	}
	for i := range wf.Steps {
		if wf.Steps[i].ID == stepID {
			wf.Steps[i].Status = status
			wf.Steps[i].Result = result
			wf.Steps[i].Error = errText
			now := time.Now()
			wf.Steps[i].FinishedAt = &now
			break
		}
	}
	return s.Save(wf)
}

// Delete 删除工作流
func (s *WorkflowStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM workflows WHERE id = ?", id)
	return err
}

// GetReadySteps 获取可执行的步骤（依赖已满足）
func GetReadySteps(wf *Workflow) []WorkflowStep {
	completed := make(map[string]bool)
	for _, s := range wf.Steps {
		if s.Status == "completed" {
			completed[s.ID] = true
		}
	}

	var ready []WorkflowStep
	for _, s := range wf.Steps {
		if s.Status != "pending" {
			continue
		}
		allDepsMet := true
		for _, dep := range s.DependsOn {
			if !completed[dep] {
				allDepsMet = false
				break
			}
		}
		if allDepsMet {
			ready = append(ready, s)
		}
	}
	return ready
}
