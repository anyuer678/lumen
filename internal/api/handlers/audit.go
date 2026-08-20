package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// AuditHandler 审计日志处理器
type AuditHandler struct {
	db *sql.DB
}

// NewAuditHandler 创建处理器
func NewAuditHandler(db *sql.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

// Routes 注册路由
func (h *AuditHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	return r
}

// AuditEntry 审计条目
type AuditEntry struct {
	ID     int64     `json:"id"`
	TS     time.Time `json:"ts"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	Detail string    `json:"detail"`
	Result string    `json:"result"`
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	action := r.URL.Query().Get("action")

	var rows *sql.Rows
	var err error

	if action != "" {
		rows, err = h.db.Query(
			`SELECT id, ts, actor, action, target, detail, result FROM audit_logs WHERE action = ? ORDER BY ts DESC LIMIT ?`,
			action, limit)
	} else {
		rows, err = h.db.Query(
			`SELECT id, ts, actor, action, target, detail, result FROM audit_logs ORDER BY ts DESC LIMIT ?`, limit)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.Result); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []*AuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}
