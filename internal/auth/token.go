package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// principalKey 用于在 context 中存取已认证的 TokenPrincipal
type principalKey struct{}

// WithPrincipal 把已认证的主体存入 context（供认证中间件使用）
func WithPrincipal(ctx context.Context, p *TokenPrincipal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext 从 context 取出已认证主体（未认证返回 nil）
func PrincipalFromContext(ctx context.Context) *TokenPrincipal {
	if v := ctx.Value(principalKey{}); v != nil {
		if p, ok := v.(*TokenPrincipal); ok {
			return p
		}
	}
	return nil
}

// TokenVerifier 校验 HTTP 请求中的 Bearer token。
type TokenVerifier struct {
	db *sql.DB
}

// NewTokenVerifier 创建校验器
func NewTokenVerifier(db *sql.DB) *TokenVerifier {
	return &TokenVerifier{db: db}
}

// ErrUnauthorized 未授权
var ErrUnauthorized = errors.New("unauthorized")

// ErrScopeDenied scope 不足
var ErrScopeDenied = errors.New("scope denied")

// TokenPrincipal 解析出的调用方身份
type TokenPrincipal struct {
	ID        string
	Name      string
	PermLevel int
	Scopes    string
}

// HasScope 检查 principal 是否拥有指定 scope。
// 空 scopes 字段视为拥有所有 scope（向后兼容未设 scope 的旧 token）。
func (p *TokenPrincipal) HasScope(scope string) bool {
	if p.Scopes == "" {
		return true
	}
	for _, s := range strings.Split(p.Scopes, ",") {
		if strings.TrimSpace(s) == scope {
			return true
		}
	}
	return false
}

// RequireScope 返回一个 HTTP 中间件：检查调用者是否拥有指定 scope，
// 缺失时返回 403。若 token 无 scopes 字段（旧 token），自动放行。
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFromContext(r.Context())
			if p == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if !p.HasScope(scope) {
				http.Error(w, fmt.Sprintf(`{"error":"scope %q required but not granted"}`, scope), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Verify 校验请求头中的 Bearer token，返回 principal。
// 支持三种来源：Authorization: Bearer <token>、X-API-Token: <token>、?token=<token>。
func (v *TokenVerifier) Verify(r *http.Request) (*TokenPrincipal, error) {
	token := extractToken(r)
	if token == "" {
		return nil, ErrUnauthorized
	}

	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])

	var p TokenPrincipal
	var enabled bool
	var expiresAt sql.NullTime
	err := v.db.QueryRow(
		`SELECT id, name, perm_level, scopes, enabled, expires_at FROM api_tokens WHERE token_hash = ?`,
		hashHex).Scan(&p.ID, &p.Name, &p.PermLevel, &p.Scopes, &enabled, &expiresAt)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if !enabled {
		return nil, ErrUnauthorized
	}
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		return nil, ErrUnauthorized
	}
	return &p, nil
}

func extractToken(r *http.Request) string {
	// Authorization: Bearer xxx
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	// X-API-Token: xxx
	if t := r.Header.Get("X-API-Token"); t != "" {
		return strings.TrimSpace(t)
	}
	// SSE 的 EventSource 无法携带自定义头，允许 ?token= 查询参数（仅前端事件流使用）
	if t := r.URL.Query().Get("token"); t != "" {
		return strings.TrimSpace(t)
	}
	return ""
}
