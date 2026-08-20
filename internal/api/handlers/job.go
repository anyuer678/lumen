package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"agent/internal/scheduler"
)

// JobHandler 定时任务处理器
type JobHandler struct {
	scheduler *scheduler.Scheduler
}

// NewJobHandler 创建处理器
func NewJobHandler(s *scheduler.Scheduler) *JobHandler {
	return &JobHandler{scheduler: s}
}

// Routes 注册路由
func (h *JobHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListJobs)
	r.Post("/", h.CreateJob)
	r.Delete("/{id}", h.DeleteJob)
	return r
}

type createJobRequest struct {
	Name         string `json:"name"`
	TriggerType  string `json:"trigger_type"`
	CronExpr     string `json:"cron_expr"`
	IntervalSecs int    `json:"interval_secs"`
	WatchPath    string `json:"watch_path"`
	GoalTemplate string `json:"goal_template"`
	Priority     int    `json:"priority"`
}

func (h *JobHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := h.scheduler.ListJobs()
	if jobs == nil {
		jobs = []*scheduler.Job{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (h *JobHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.GoalTemplate == "" {
		http.Error(w, "name and goal_template are required", http.StatusBadRequest)
		return
	}

	if req.TriggerType == "" {
		req.TriggerType = "cron"
	}

	job := &scheduler.Job{
		ID:           "j-" + uuid.New().String()[:8],
		Name:         req.Name,
		TriggerType:  scheduler.TriggerType(req.TriggerType),
		CronExpr:     req.CronExpr,
		IntervalSecs: req.IntervalSecs,
		WatchPath:    req.WatchPath,
		GoalTemplate: req.GoalTemplate,
		Priority:     req.Priority,
		Enabled:      true,
		CreatedAt:    time.Now(),
	}

	if job.Priority == 0 {
		job.Priority = 5
	}

	// 计算首次运行时间
	now := time.Now()
	if job.TriggerType == scheduler.TriggerCron && job.CronExpr != "" {
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 8, 0, 0, 0, now.Location())
		job.NextRunAt = &next
	} else if job.TriggerType == scheduler.TriggerInterval && job.IntervalSecs > 0 {
		next := now.Add(time.Duration(job.IntervalSecs) * time.Second)
		job.NextRunAt = &next
	}

	if err := h.scheduler.CreateJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(job)
}

func (h *JobHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.scheduler.DeleteJob(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
