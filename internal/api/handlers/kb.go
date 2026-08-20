package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"agent/internal/memory"
)

// KBHandler 知识库处理器
type KBHandler struct {
	store *memory.KBStore
}

// NewKBHandler 创建处理器
func NewKBHandler(db *sql.DB) *KBHandler {
	return &KBHandler{store: memory.NewKBStore(db)}
}

// Routes 注册路由
func (h *KBHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Add)
	r.Post("/search", h.Search)
	r.Delete("/{id}", h.Delete)
	return r
}

func (h *KBHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.store.List(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

type addKBRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Tags    string `json:"tags"`
	Source  string `json:"source"`
}

func (h *KBHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req addKBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Content == "" {
		http.Error(w, "title and content are required", http.StatusBadRequest)
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}

	k := &memory.Knowledge{
		ID:        "kb-" + uuid.New().String()[:8],
		Title:     req.Title,
		Content:   req.Content,
		Tags:      req.Tags,
		Source:    req.Source,
		CreatedAt: time.Now(),
	}

	if err := h.store.Add(k); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(k)
}

func (h *KBHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	items, err := h.store.Search(req.Query, req.Limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *KBHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
