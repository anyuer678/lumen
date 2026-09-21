package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
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

// Scope 常量：与 RequireScope / HasScope 字符串严格一致。
const (
	ScopeToolsRun      = "tools:run"
	ScopeTasksCreate   = "tasks:create"
	ScopeTasksControl  = "tasks:control"
	ScopeConfirmApprove = "confirm:approve"
	ScopeMCPRegister   = "mcp:register"
	ScopeTokenManage   = "token:manage"
	ScopeEventsEmit    = "events:emit"
	ScopeKBWrite       = "kb:write"
	ScopeSettingsWrite = "settings:write"
)

// DefaultTokenScopes 新签发 token 的最小 scopes。
const DefaultTokenScopes = ScopeToolsRun + "," + ScopeTasksCreate

// AdminTokenScopes bootstrap / L3 管理员 token 的全量 scopes。
const AdminTokenScopes = ScopeToolsRun + "," + ScopeTasksCreate + "," + ScopeTasksControl +
	"," + ScopeConfirmApprove + "," + ScopeMCPRegister + "," + ScopeTokenManage +
	"," + ScopeEventsEmit + "," + ScopeKBWrite + "," + ScopeSettingsWrite

// allowLegacyEmptyScopes 仅用于从旧版本迁移：
// 历史行为「空 scopes = 全部权限」fail-open，升级后默认拒绝。
// 迁移期设置环境变量 LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1 可暂时兼容，
// 必须在 CHANGELOG 与启动日志中警告，并在迁完后删除该变量。
func allowLegacyEmptyScopes() bool {
	v := strings.TrimSpace(os.Getenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES"))
	return v == "1" || strings.EqualFold(v, "true")
}

// HasScope 检查 principal 是否拥有指定 scope。
// 安全默认：空 scopes 字段 **不** 视为拥有全部 scope（fail-closed）。
// 仅当显式设置 LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1 时才保留旧行为（迁移期）。
func (p *TokenPrincipal) HasScope(scope string) bool {
	if p == nil {
		return false
	}
	if p.Scopes == "" {
		return allowLegacyEmptyScopes()
	}
	for _, s := range strings.Split(p.Scopes, ",") {
		if strings.TrimSpace(s) == scope {
			return true
		}
	}
	return false
}

// RequireScope 返回一个 HTTP 中间件：检查调用者是否拥有指定 scope，缺失时 403。
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

// queryTokenAllowedPath 仅允许 SSE/事件流使用 ?token=（EventSource 无法带自定义头）。
// 其它路径一律要求 Authorization / X-API-Token，避免 token 进入访问日志与浏览器历史。
func queryTokenAllowedPath(path string) bool {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "/events"),
		strings.Contains(p, "/sse"),
		strings.HasSuffix(p, "/stream"):
		return true
	default:
		return false
	}
}

// Verify 校验请求头中的 Bearer token，返回 principal。
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
	// ?token= 仅限 SSE/事件流路径
	if t := r.URL.Query().Get("token"); t != "" && queryTokenAllowedPath(r.URL.Path) {
		return strings.TrimSpace(t)
	}
	return ""
}
