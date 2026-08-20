package auth

import (
	"database/sql"
	"fmt"
	"time"
)

// ConfirmationStatus 确认状态
type ConfirmationStatus string

const (
	ConfirmPending   ConfirmationStatus = "pending"
	ConfirmApproved  ConfirmationStatus = "approved"
	ConfirmRejected  ConfirmationStatus = "rejected"
	ConfirmTimeout   ConfirmationStatus = "timeout"
	ConfirmCancelled ConfirmationStatus = "cancelled"
)

// Confirmation 人工确认
type Confirmation struct {
	ID           string              `json:"id"`
	TaskID       string              `json:"task_id"`
	StepSeq      int                 `json:"step_seq"`
	Operation    string              `json:"operation"`
	Tool         string              `json:"tool"`
	ArgsJSON     string              `json:"args_json,omitempty"`
	RiskLevel    PermissionLevel     `json:"risk_level"`
	Reason       string              `json:"reason,omitempty"`
	Status       ConfirmationStatus  `json:"status"`
	Requester    string              `json:"requester"`
	CreatedAt    time.Time           `json:"created_at"`
	DecidedAt    *time.Time          `json:"decided_at,omitempty"`
	DecidedBy    string              `json:"decided_by,omitempty"`
	TimeoutSecs  int                 `json:"timeout_secs"`
}

// ConfirmStore 确认存储
type ConfirmStore struct {
	db *sql.DB
}

// NewConfirmStore 创建确认存储
func NewConfirmStore(db *sql.DB) *ConfirmStore {
	return &ConfirmStore{db: db}
}

// Create 创建确认请求
func (s *ConfirmStore) Create(conf *Confirmation) error {
	_, err := s.db.Exec(`
		INSERT INTO confirmations 
		(id, task_id, step_seq, operation, tool, args_json, risk_level, reason, status, requester, created_at, timeout_secs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conf.ID, conf.TaskID, conf.StepSeq, conf.Operation, conf.Tool, conf.ArgsJSON,
		conf.RiskLevel, conf.Reason, conf.Status, conf.Requester, conf.CreatedAt, conf.TimeoutSecs,
	)
	return err
}

// Get 获取确认
func (s *ConfirmStore) Get(id string) (*Confirmation, error) {
	conf := &Confirmation{}
	var decidedAt sql.NullTime
	var decidedBy sql.NullString

	err := s.db.QueryRow(`
		SELECT id, task_id, step_seq, operation, tool, args_json, risk_level, reason, status, requester, created_at, decided_at, decided_by, timeout_secs
		FROM confirmations WHERE id = ?`, id).Scan(
		&conf.ID, &conf.TaskID, &conf.StepSeq, &conf.Operation, &conf.Tool, &conf.ArgsJSON,
		&conf.RiskLevel, &conf.Reason, &conf.Status, &conf.Requester, &conf.CreatedAt, &decidedAt, &decidedBy, &conf.TimeoutSecs,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query confirmation: %w", err)
	}

	if decidedAt.Valid {
		conf.DecidedAt = &decidedAt.Time
	}
	if decidedBy.Valid {
		conf.DecidedBy = decidedBy.String
	}

	return conf, nil
}

// Approve 批准
func (s *ConfirmStore) Approve(id, decidedBy string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE confirmations SET status = 'approved', decided_at = ?, decided_by = ?
		WHERE id = ? AND status = 'pending'`, now, decidedBy, id)
	return err
}

// Reject 拒绝
func (s *ConfirmStore) Reject(id, decidedBy, reason string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE confirmations SET status = 'rejected', decided_at = ?, decided_by = ?, reason = ?
		WHERE id = ? AND status = 'pending'`, now, decidedBy, reason, id)
	return err
}

// ListPending 列出待确认
func (s *ConfirmStore) ListPending() ([]*Confirmation, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, step_seq, operation, tool, args_json, risk_level, reason, status, requester, created_at, timeout_secs
		FROM confirmations WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var confs []*Confirmation
	for rows.Next() {
		conf := &Confirmation{}
		if err := rows.Scan(
			&conf.ID, &conf.TaskID, &conf.StepSeq, &conf.Operation, &conf.Tool, &conf.ArgsJSON,
			&conf.RiskLevel, &conf.Reason, &conf.Status, &conf.Requester, &conf.CreatedAt, &conf.TimeoutSecs,
		); err != nil {
			return nil, err
		}
		confs = append(confs, conf)
	}
	return confs, nil
}

// TimeoutExpired 检查超时
func (s *ConfirmStore) TimeoutExpired(id string) (bool, error) {
	conf, err := s.Get(id)
	if err != nil {
		return false, err
	}
	if conf == nil || conf.Status != ConfirmPending {
		return false, nil
	}

	elapsed := time.Since(conf.CreatedAt)
	return elapsed > time.Duration(conf.TimeoutSecs)*time.Second, nil
}
