package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"agent/internal/agent"
	"agent/internal/memory"
)

// ProfileHandler 用户画像 API
type ProfileHandler struct {
	reflection *agent.ReflectionEngine
	logger     *zap.SugaredLogger
}

// NewProfileHandler 创建处理器
func NewProfileHandler(db *sql.DB, logger *zap.Logger) *ProfileHandler {
	fbStore := agent.NewFeedbackStore(db)
	reflection := agent.NewReflectionEngine(db, memory.NewStore(db), fbStore)
	_ = reflection.InitSchema()
	return &ProfileHandler{reflection: reflection, logger: logger.Sugar()}
}

// Routes 注册路由
func (h *ProfileHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/reflect", h.Reflect)
	return r
}

// List 获取所有用户画像
func (h *ProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.reflection.GetProfiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if profiles == nil {
		profiles = []*agent.UserProfile{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"value": profiles,
		"count": len(profiles),
	})
}

// Reflect 执行一次反思
func (h *ProfileHandler) Reflect(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.reflection.Reflect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if profiles == nil {
		profiles = []*agent.UserProfile{}
	}

	h.logger.Infof("reflection: generated %d profiles", len(profiles))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"profiles": profiles,
		"count":    len(profiles),
		"time":     time.Now().Format(time.RFC3339),
	})
}
