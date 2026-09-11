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
	"agent/internal/auth"
	"agent/internal/config"
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
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Scopes    string     `json:"scopes"`
	PermLevel int        `json:"perm_level"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (h *TokenHandler) List(w http.ResponseWriter, r *http.Request) {
	// 权限校验：列出全部 token 元数据需要 L3（防止低权限 token 枚举）
	caller := auth.PrincipalFromContext(r.Context())
	if caller == nil || caller.PermLevel < 3 {
		http.Error(w, `{"error":"权限不足：列出 token 需要 L3"}`, http.StatusForbidden)
		return
	}
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

// parseTTL 解析 TTL 字符串（如 "365d"、"24h"、"30m"）为 time.Duration。
// 支持后缀：d(天)、h(小时)、m(分钟)、s(秒)。无后缀默认为天。
func parseTTL(ttl string) (time.Duration, error) {
	if ttl == "" {
		return 0, fmt.Errorf("empty TTL")
	}
	// 尝试标准 Go duration
	if d, err := time.ParseDuration(ttl); err == nil {
		return d, nil
	}
	// 自定义后缀：NNd = N天
	last := ttl[len(ttl)-1]
	numeric := ttl[:len(ttl)-1]
	var n int
	if _, err := fmt.Sscanf(numeric, "%d", &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid TTL: %s", ttl)
	}
	switch last {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 's':
		return time.Duration(n) * time.Second, nil
	default:
		return 0, fmt.Errorf("unknown TTL suffix: %c", last)
	}
}

func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	// 权限校验：调用者级别必须 ≥ 被签发级别（L0 只读不能签发任何 token）
	caller := auth.PrincipalFromContext(r.Context())
	if caller == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

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
	// 防止提权：调用者不能签发比自己更高级别的 token
	if req.PermLevel > caller.PermLevel {
		http.Error(w, fmt.Sprintf(`{"error":"权限不足：调用者级别 L%d 不能签发 L%d token"}`, caller.PermLevel, req.PermLevel), http.StatusForbidden)
		return
	}

	// 生成随机 token
	raw := make([]byte, 32)
	rand.Read(raw)
	tokenStr := "agt_" + hex.EncodeToString(raw)

	// 存储哈希
	hash := sha256.Sum256([]byte(tokenStr))
	id := "tok-" + uuid.New().String()[:8]

	// 计算过期时间：从配置读取 TTL（默认 365 天），新 token 均设置 expires_at
	var expiresAt time.Time
	cfg := config.Get()
	ttlStr := "365d" // 默认值
	if cfg != nil && cfg.Server.APITokenTTL != "" {
		ttlStr = cfg.Server.APITokenTTL
	}
	if ttl, err := parseTTL(ttlStr); err == nil {
		expiresAt = time.Now().Add(ttl)
	} else {
		// TTL 解析失败时仍设置合理默认值（365 天），避免 token 永不过期
		expiresAt = time.Now().Add(365 * 24 * time.Hour)
	}

	_, err := h.db.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, scopes, perm_level, enabled, created_at, expires_at)
	 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		id, req.Name, hex.EncodeToString(hash[:]), req.Scopes, req.PermLevel, time.Now(), expiresAt)
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
		"expires_at": expiresAt,
		"warning":    "请立即保存 token，仅显示一次",
	})
}

func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	// 权限校验：吊销 token 需要 L3（防止 L0/L1 拒绝服务或清除审计）
	caller := auth.PrincipalFromContext(r.Context())
	if caller == nil || caller.PermLevel < 3 {
		http.Error(w, `{"error":"权限不足：吊销 token 需要 L3"}`, http.StatusForbidden)
		return
	}
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
