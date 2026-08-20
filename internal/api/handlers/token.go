package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TokenHandler API Token 处理器
type TokenHandler struct {
	db *sql.DB
}

// NewTokenHandler 创建处理器
func NewTokenHandler(db *sql.DB) *TokenHandler {
	return &TokenHandler{db: db}
}

// Routes 注册路由
func (h *TokenHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Delete("/{id}", h.Revoke)
	return r
}

// Token 令牌结构
type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scopes    string    `json:"scopes"`
	PermLevel int       `json:"perm_level"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (h *TokenHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, name, scopes, perm_level, enabled, created_at, expires_at FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		t := &Token{}
		var expiresAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.Scopes, &t.PermLevel, &t.Enabled, &t.CreatedAt, &expiresAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}
		tokens = append(tokens, t)
	}
	if tokens == nil {
		tokens = []*Token{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

type createTokenRequest struct {
	Name      string `json:"name"`
	Scopes    string `json:"scopes"`
	PermLevel int    `json:"perm_level"`
}

func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Scopes == "" {
		req.Scopes = "tasks:create,tasks:control,confirm:approve"
	}
	if req.PermLevel == 0 {
		req.PermLevel = 1
	}

	// 生成随机 token
	raw := make([]byte, 32)
	rand.Read(raw)
	tokenStr := "agt_" + hex.EncodeToString(raw)

	// 存储哈希
	hash := sha256.Sum256([]byte(tokenStr))
	id := "tok-" + uuid.New().String()[:8]

	_, err := h.db.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, scopes, perm_level, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?)`,
		id, req.Name, hex.EncodeToString(hash[:]), req.Scopes, req.PermLevel, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         id,
		"name":       req.Name,
		"scopes":     req.Scopes,
		"perm_level": req.PermLevel,
		"token":      tokenStr, // 仅此一次显示明文
		"warning":    "请立即保存 token，仅显示一次",
	})
}

func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.Exec(`UPDATE api_tokens SET enabled = 0 WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		http.Error(w, fmt.Sprintf("token not found: %s", id), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}
