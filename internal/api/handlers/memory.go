package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"agent/internal/memory"
)

// MemoryHandler 记忆处理器
type MemoryHandler struct {
	store *memory.Store
}

// NewMemoryHandler 创建处理器
func NewMemoryHandler(s *memory.Store) *MemoryHandler {
	return &MemoryHandler{store: s}
}

// Routes 注册路由
func (h *MemoryHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListMemories)
	r.Post("/{id}/confirm", h.Confirm)
	r.Delete("/{id}", h.Delete)
	return r
}

func (h *MemoryHandler) ListMemories(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind") // long_term / short_term / working
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	var mems []*memory.Memory
	var err error

	if kind != "" {
		mems, err = h.store.ListByType(memory.MemoryType(kind), limit)
	} else {
		// 列出所有（按 kind 分别查询合并）
		mems = []*memory.Memory{}
		for _, k := range []memory.MemoryType{memory.MemoryLongTerm, memory.MemoryWorking, memory.MemoryShortTerm} {
			items, e := h.store.ListByType(k, limit)
			if e == nil {
				mems = append(mems, items...)
			}
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if mems == nil {
		mems = []*memory.Memory{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mems)
}

func (h *MemoryHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Confirm(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *MemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
