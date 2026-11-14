package observability

import (
	"database/sql"
	"fmt"
	"strings"
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

// List 列出审计日志（不限条件，兼容旧调用）
func (l *AuditLogger) List(limit int) ([]*AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	return l.Query(limit, "", "", "")
}

// Query 按条件查询审计/决策日志（DESIGN Phase C：决策日志可查询）。
// actor/action 为空表示不过滤；since 支持 "2006-01-02" 或 "2006-01-02 15:04:05"，
// 与库内 UTC 时间戳做字典序比较（datetime('now') 存 UTC）。
func (l *AuditLogger) Query(limit int, actor, action, since string) ([]*AuditEntry, error) {
	if l.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	sqlStr := `SELECT ts, actor, action, target, detail, result, ip
		FROM audit_logs WHERE 1=1`
	args := []any{}
	if actor != "" {
		sqlStr += " AND actor=?"
		args = append(args, actor)
	}
	if action != "" {
		sqlStr += " AND action=?"
		args = append(args, action)
	}
	if since != "" {
		norm, err := NormalizeSince(since)
		if err != nil {
			return nil, err
		}
		sqlStr += " AND ts >= ?"
		args = append(args, norm)
	}
	sqlStr += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := l.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []*AuditEntry{}
	for rows.Next() {
		entry := &AuditEntry{}
		if err := rows.Scan(&entry.TS, &entry.Actor, &entry.Action, &entry.Target, &entry.Detail, &entry.Result, &entry.IP); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// NormalizeSince 把 --since 取值规范化为 "2006-01-02 15:04:05"（按 UTC 比较）。
func NormalizeSince(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", fmt.Errorf("--since 格式须为 YYYY-MM-DD 或 YYYY-MM-DD HH:MM:SS（按 UTC 比较）：%q", s)
}
