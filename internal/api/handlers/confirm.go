package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"agent/internal/auth"
)

// ConfirmHandler 确认处理器
type ConfirmHandler struct {
	store *auth.ConfirmStore
}

// NewConfirmHandler 创建处理器
func NewConfirmHandler(store *auth.ConfirmStore) *ConfirmHandler {
	return &ConfirmHandler{store: store}
}

// Routes 注册路由
func (h *ConfirmHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListPending)
	r.Post("/{id}/approve", h.Approve)
	r.Post("/{id}/reject", h.Reject)
	return r
}

func (h *ConfirmHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	confs, err := h.store.ListPending()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if confs == nil {
		confs = []*auth.Confirmation{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(confs)
}

func (h *ConfirmHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Approve(id, "dashboard-user"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ConfirmHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Reject(id, "dashboard-user", "rejected by user"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
