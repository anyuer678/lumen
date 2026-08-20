package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"agent/internal/agent"
)

// EventHandler 事件处理器
type EventHandler struct {
	eventBus *agent.EventBus
}

// NewEventHandler 创建处理器
func NewEventHandler(db *sql.DB) *EventHandler {
	return &EventHandler{eventBus: agent.NewEventBus(db)}
}

// Routes 注册路由
func (h *EventHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListEvents)
	r.Post("/emit", h.EmitEvent)
	r.Delete("/", h.ClearOld)
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
