package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"agent/internal/task"
)

// TaskHandler 任务处理器
type TaskHandler struct {
	manager *task.Manager
}

// NewTaskHandler 创建处理器
func NewTaskHandler(manager *task.Manager) *TaskHandler {
	return &TaskHandler{manager: manager}
}

// Routes 注册路由
func (h *TaskHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.CreateTask)
	r.Get("/", h.ListTasks)
	r.Get("/{id}", h.GetTask)
	r.Post("/{id}/pause", h.PauseTask)
	r.Post("/{id}/resume", h.ResumeTask)
	r.Post("/{id}/stop", h.StopTask)
	r.Post("/{id}/retry", h.RetryTask)
	r.Post("/{id}/rollback", h.RollbackTask)
	r.Get("/{id}/steps", h.GetSteps)
	r.Delete("/", h.ClearTasks)
	return r
}

type createTaskRequest struct {
	Goal     string `json:"goal"`
	Priority int    `json:"priority"`
	Type     string `json:"type"`
}

func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Goal == "" {
		http.Error(w, "goal is required", http.StatusBadRequest)
		return
	}

	if req.Priority == 0 {
		req.Priority = 5
	}

	opts := []task.Option{task.WithPriority(req.Priority)}
	if req.Type != "" {
		opts = append(opts, task.WithType(req.Type))
	}

	t, err := h.manager.CreateTask(req.Goal, opts...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
}

func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	tasks, total, err := h.manager.ListTasks(status, page, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": tasks,
		"total": total,
		"page":  page,
	})
}

func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.manager.GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

func (h *TaskHandler) PauseTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.manager.PauseTask(id, "user_pause"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *TaskHandler) ResumeTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.manager.ResumeTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *TaskHandler) StopTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.manager.StopTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *TaskHandler) RetryTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.manager.RetryTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// RollbackTask 回滚任务到指定步骤
func (h *TaskHandler) RollbackTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Step int `json:"step"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	if body.Step < 0 {
		http.Error(w, "step must be >= 0", http.StatusBadRequest)
		return
	}
	if err := h.manager.RollbackTaskToStep(id, body.Step); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "rolled_back",
		"task":   id,
		"to_step": body.Step,
	})
}

func (h *TaskHandler) GetSteps(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	steps, err := h.manager.GetSteps(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": steps,
	})
}

func (h *TaskHandler) ClearTasks(w http.ResponseWriter, r *http.Request) {
	keepRunning := r.URL.Query().Get("keep_running") == "true"
	count, err := h.manager.ClearTasks(keepRunning)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted": count,
	})
}
