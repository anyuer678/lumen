package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"agent/internal/config"
	"agent/internal/trajectory"
)

// TrajectoryHandler 提供任务轨迹的列出与回放读取。
type TrajectoryHandler struct{}

// NewTrajectoryHandler 创建处理器
func NewTrajectoryHandler() *TrajectoryHandler {
	return &TrajectoryHandler{}
}

func (h *TrajectoryHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Get("/{taskID}", h.Get)
	return r
}

func (h *TrajectoryHandler) dir() string {
	return filepath.Join(config.Get().Workspace.Root, "trajectories")
}

// List 列出所有有轨迹的任务
func (h *TrajectoryHandler) List(w http.ResponseWriter, r *http.Request) {
	names, err := trajectory.List(h.dir())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"count": len(names), "tasks": names})
}

// Get 返回某任务的完整轨迹（回放）
func (h *TrajectoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	recs, err := trajectory.Load(h.dir(), taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recs == nil {
		recs = []trajectory.Record{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"task_id": taskID, "count": len(recs), "events": recs})
}
