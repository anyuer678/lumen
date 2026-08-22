package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
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

// TokenPrincipal 解析出的调用方身份
type TokenPrincipal struct {
	ID        string
	Name      string
	PermLevel int
	Scopes    string
}

// Verify 校验请求头中的 Bearer token，返回 principal。
// 支持两种来源：Authorization: Bearer <token>，或 X-API-Token: <token>。
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
	return ""
}
