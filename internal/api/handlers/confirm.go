package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"agent/internal/auth"
	"github.com/go-chi/chi/v5"
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
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, "未认证：批准确认流需要携带有效 token", http.StatusUnauthorized)
		return
	}
	conf, err := h.store.Get(id)
	if err != nil || conf == nil {
		http.Error(w, "确认流不存在", http.StatusNotFound)
		return
	}
	// 高危确认需要足够级别的 token 批准：L0 只读 token 不能批准任何确认，
	// L2 风险的确认需要 L2 及以上（此前任何级别 token 都能批准 L3 高危确认）
	if conf.RiskLevel >= auth.Level2Dangerous && p.PermLevel < int(conf.RiskLevel) {
		http.Error(w, fmt.Sprintf("权限不足：批准 L%d 风险确认需要 L%d 及以上 token",
			int(conf.RiskLevel), int(conf.RiskLevel)), http.StatusForbidden)
		return
	}
	if err := h.store.Approve(id, p.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ConfirmHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// 拒绝是安全方向的操作，任何已认证主体都可执行；审计记录真实主体
	decidedBy := "dashboard-user"
	if p := auth.PrincipalFromContext(r.Context()); p != nil {
		decidedBy = p.Name
	}
	if err := h.store.Reject(id, decidedBy, "rejected by user"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
