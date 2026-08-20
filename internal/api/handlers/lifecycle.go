package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"agent/internal/agent"
)

// LifecycleHandler 记忆生命周期 API
type LifecycleHandler struct {
	lifecycle *agent.MemoryLifecycle
	logger    *zap.SugaredLogger
}

// NewLifecycleHandler 创建处理器
func NewLifecycleHandler(db *sql.DB, logger *zap.Logger) *LifecycleHandler {
	lc := agent.NewMemoryLifecycle(db)
	_ = lc.InitSchema()
	return &LifecycleHandler{lifecycle: lc, logger: logger.Sugar()}
}

// Routes 注册路由
func (h *LifecycleHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/stats", h.Stats)
	r.Post("/run", h.Run)
	return r
}

// Stats 获取生命周期统计
func (h *LifecycleHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.lifecycle.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Run 执行一次生命周期检查
func (h *LifecycleHandler) Run(w http.ResponseWriter, r *http.Request) {
	changed, err := h.lifecycle.RunLifecycle()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"changed": changed,
		"status":  "ok",
	})
}
