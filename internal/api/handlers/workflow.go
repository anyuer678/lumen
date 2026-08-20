package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"agent/internal/task"
)

// WorkflowHandler 工作流处理器
type WorkflowHandler struct {
	store *task.WorkflowStore
}

// NewWorkflowHandler 创建处理器
func NewWorkflowHandler(db *sql.DB) *WorkflowHandler {
	return &WorkflowHandler{store: task.NewWorkflowStore(db)}
}

// Routes 注册路由
func (h *WorkflowHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Delete("/{id}", h.Delete)
	return r
}

// List 列出工作流
func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.store.List(20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if wfs == nil {
		wfs = []*task.Workflow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wfs)
}

// Create 创建工作流
func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Steps       []struct {
			ID          string         `json:"id"`
			Description string         `json:"description"`
			Tool        string         `json:"tool"`
			Args        map[string]any `json:"args"`
			DependsOn   []string       `json:"depends_on"`
		} `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	wf := &task.Workflow{
		ID:          fmt.Sprintf("wf-%d", time.Now().UnixNano()),
		Name:        req.Name,
		Description: req.Description,
		Status:      "pending",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	for _, s := range req.Steps {
		wf.Steps = append(wf.Steps, task.WorkflowStep{
			ID:          s.ID,
			Description: s.Description,
			Tool:        s.Tool,
			Args:        s.Args,
			DependsOn:   s.DependsOn,
			Status:      "pending",
		})
	}

	if err := h.store.Save(wf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wf)
}

// Get 获取工作流
func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wf, err := h.store.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if wf == nil {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wf)
}

// Delete 删除工作流
func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
