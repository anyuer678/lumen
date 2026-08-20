package observability

import (
	"database/sql"
	"time"
)

// AuditEntry 审计日志条目
type AuditEntry struct {
	TS     time.Time `json:"ts"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target,omitempty"`
	Detail string    `json:"detail,omitempty"`
	Result string    `json:"result,omitempty"`
	IP     string    `json:"ip,omitempty"`
}

// AuditLogger 审计日志记录器
type AuditLogger struct {
	db *sql.DB
}

// NewAuditLogger 创建审计日志记录器
func NewAuditLogger(db *sql.DB) *AuditLogger {
	return &AuditLogger{db: db}
}

// Record 记录审计日志
func (l *AuditLogger) Record(entry *AuditEntry) error {
	if l.db == nil {
		return nil
	}

	_, err := l.db.Exec(`
		INSERT INTO audit_logs (ts, actor, action, target, detail, result, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TS, entry.Actor, entry.Action, entry.Target, entry.Detail, entry.Result, entry.IP,
	)
	return err
}

// List 列出审计日志
func (l *AuditLogger) List(limit int) ([]*AuditEntry, error) {
	if l.db == nil {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	rows, err := l.db.Query(`
		SELECT ts, actor, action, target, detail, result, ip
		FROM audit_logs ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		entry := &AuditEntry{}
		if err := rows.Scan(&entry.TS, &entry.Actor, &entry.Action, &entry.Target, &entry.Detail, &entry.Result, &entry.IP); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
