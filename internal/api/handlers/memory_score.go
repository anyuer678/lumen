package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"agent/internal/agent"
)

// MemoryScoreHandler 记忆质量评分 API
type MemoryScoreHandler struct {
	scorer *agent.MemoryScorer
	logger *zap.SugaredLogger
}

// NewMemoryScoreHandler 创建处理器
func NewMemoryScoreHandler(db *sql.DB, logger *zap.Logger) *MemoryScoreHandler {
	scorer := agent.NewMemoryScorer(db)
	_ = scorer.InitSchema()
	return &MemoryScoreHandler{scorer: scorer, logger: logger.Sugar()}
}

// Routes 注册路由
func (h *MemoryScoreHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/recalc", h.Recalc)
	r.Get("/top", h.GetTop)
	r.Get("/low", h.GetLow)
	return r
}

// Recalc 重新计算所有记忆评分
func (h *MemoryScoreHandler) Recalc(w http.ResponseWriter, r *http.Request) {
	updated, err := h.scorer.ScoreAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"updated": updated,
		"status":  "ok",
	})
}

// GetTop 获取高分记忆
func (h *MemoryScoreHandler) GetTop(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	mems, err := h.scorer.GetTopQualityMemories(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondScores(w, mems)
}

// GetLow 获取低分记忆（可归档）
func (h *MemoryScoreHandler) GetLow(w http.ResponseWriter, r *http.Request) {
	threshold := 0.3
	limit := 20
	if v := r.URL.Query().Get("threshold"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	mems, err := h.scorer.GetLowQualityMemories(threshold, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondScores(w, mems)
}

func respondScores(w http.ResponseWriter, mems []agent.MemoryScore) {
	if mems == nil {
		mems = []agent.MemoryScore{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"value": mems,
		"count": len(mems),
	})
}
