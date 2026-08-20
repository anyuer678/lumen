package auth

import (
	"database/sql"
	"time"
)

// AuditEntry 审计日志条目
type AuditEntry struct {
	ID        int64     `json:"id"`
	TS        time.Time `json:"ts"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Result    string    `json:"result,omitempty"`
	IP        string    `json:"ip,omitempty"`
}

// AuditStore 审计存储
type AuditStore struct {
	db *sql.DB
}

// NewAuditStore 创建审计存储
func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// Record 记录审计日志
func (s *AuditStore) Record(entry *AuditEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO audit_logs (ts, actor, action, target, detail, result, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TS, entry.Actor, entry.Action, entry.Target, entry.Detail, entry.Result, entry.IP,
	)
	return err
}

// List 列出审计日志
func (s *AuditStore) List(limit int) ([]*AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, ts, actor, action, target, detail, result, ip
		FROM audit_logs ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		entry := &AuditEntry{}
		if err := rows.Scan(
			&entry.ID, &entry.TS, &entry.Actor, &entry.Action, &entry.Target, &entry.Detail, &entry.Result, &entry.IP,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ListByAction 按动作列出
func (s *AuditStore) ListByAction(action string, limit int) ([]*AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, ts, actor, action, target, detail, result, ip
		FROM audit_logs WHERE action = ? ORDER BY ts DESC LIMIT ?`, action, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		entry := &AuditEntry{}
		if err := rows.Scan(
			&entry.ID, &entry.TS, &entry.Actor, &entry.Action, &entry.Target, &entry.Detail, &entry.Result, &entry.IP,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Count 统计数量
func (s *AuditStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	return count, err
}
