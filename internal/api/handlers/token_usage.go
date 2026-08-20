package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"agent/internal/llm"
)

// TokenUsageHandler LLM Token 用量追踪处理器
type TokenUsageHandler struct {
	db      *sql.DB
	tracker *llm.TokenTracker
}

// NewTokenUsageHandler 创建处理器
func NewTokenUsageHandler(db *sql.DB) *TokenUsageHandler {
	return &TokenUsageHandler{db: db, tracker: llm.NewTokenTracker(db)}
}

// Routes 注册路由
func (h *TokenUsageHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetSummary)
	r.Get("/providers", h.GetByProvider)
	r.Get("/daily", h.GetByDay)
	r.Get("/recent", h.GetRecent)
	return r
}

func (h *TokenUsageHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	since := parseSince(r.URL.Query().Get("since"))
	summary, err := h.tracker.GetSummary(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (h *TokenUsageHandler) GetByProvider(w http.ResponseWriter, r *http.Request) {
	since := parseSince(r.URL.Query().Get("since"))
	stats, err := h.tracker.GetByProvider(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if stats == nil {
		stats = []llm.ProviderStats{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *TokenUsageHandler) GetByDay(w http.ResponseWriter, r *http.Request) {
	since := parseSince(r.URL.Query().Get("since"))
	stats, err := h.tracker.GetByDay(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if stats == nil {
		stats = []llm.DailyStats{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *TokenUsageHandler) GetRecent(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	usages, err := h.tracker.GetRecent(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if usages == nil {
		usages = []llm.TokenUsage{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(usages)
}

func parseSince(s string) time.Time {
	switch s {
	case "today":
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "week":
		return time.Now().AddDate(0, 0, -7)
	case "month":
		return time.Now().AddDate(0, -1, 0)
	default:
		return time.Now().AddDate(0, -1, 0)
	}
}
