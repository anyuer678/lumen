package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"agent/internal/agent"
	"agent/internal/auth"
)

// EventHandler 事件处理器
type EventHandler struct {
	eventBus *agent.EventBus
}

// NewEventHandler 创建处理器
func NewEventHandler(db *sql.DB) *EventHandler {
	return &EventHandler{eventBus: agent.NewEventBus(db)}
}

// Routes 注册路由。
// scope 门禁：读端点（GET /）由 router.go 的 RequireAnyScope(events:read, events:emit)
// 把守且此处再校验一次；写端点（POST /emit、DELETE /）在此收紧为 events:emit，
// 保证只读 token 无法发射/清理事件。
func (h *EventHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAnyScope(auth.ScopeEventsRead, auth.ScopeEventsEmit))
		r.Get("/", h.ListEvents)
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireScope(auth.ScopeEventsEmit))
		r.Post("/emit", h.EmitEvent)
		r.Delete("/", h.ClearOld)
	})
	return r
}

// ListEvents 列出事件
func (h *EventHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	events, err := h.eventBus.GetRecent(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []agent.Event{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// EmitEvent 发射事件
func (h *EventHandler) EmitEvent(w http.ResponseWriter, r *http.Request) {
	// 权限校验：伪造事件可驱动 proactive 恢复/重试，需要 L2+
	caller := auth.PrincipalFromContext(r.Context())
	if caller == nil || caller.PermLevel < 2 {
		http.Error(w, `{"error":"权限不足：发射事件需要 L2 及以上 token"}`, http.StatusForbidden)
		return
	}
	var event agent.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if event.Type == "" {
		http.Error(w, "event_type is required", http.StatusBadRequest)
		return
	}
	if err := h.eventBus.Emit(event); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "emitted"})
}

// ClearOld 清理旧事件
func (h *EventHandler) ClearOld(w http.ResponseWriter, r *http.Request) {
	// 权限校验：批量删除事件历史属于破坏性操作，需要 L3
	caller := auth.PrincipalFromContext(r.Context())
	if caller == nil || caller.PermLevel < 3 {
		http.Error(w, `{"error":"权限不足：清理事件需要 L3"}`, http.StatusForbidden)
		return
	}
	keepDays := 30
	if d := r.URL.Query().Get("keep_days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			keepDays = n
		}
	}
	n, err := h.eventBus.ClearOld(keepDays)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"deleted": n})
}
