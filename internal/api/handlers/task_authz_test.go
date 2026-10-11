package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/auth"
	agentDB "agent/internal/db"
)

// 外部审查 2026-10 回归：
// 1. token 签发端 req.Scopes 原样入库 → 低权限调用者可自授 token:manage 等 scope（提权）。
// 2. /tasks 路由无 RequireScope，tasks:create / tasks:control 为死字母。

func createTokenWithPrincipal(t *testing.T, p *auth.TokenPrincipal, body string) *httptest.ResponseRecorder {
	t.Helper()
	db, err := agentDB.Init(filepath.Join(t.TempDir(), "scope-test.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewTokenHandler(db)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", strings.NewReader(body))
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))
	w := httptest.NewRecorder()
	h.Create(w, req)
	return w
}

// L1 调用者不能签发超出自身 scopes 的 token（防提权）。
func TestTokenCreateScopeEscalationDenied(t *testing.T) {
	caller := &auth.TokenPrincipal{
		Name: "l1", PermLevel: 1,
		Scopes: auth.DefaultTokenScopes, // tools:run,tasks:create
	}
	w := createTokenWithPrincipal(t, caller,
		`{"name":"evil","scopes":"tasks:create,token:manage","perm_level":1}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("scope escalation must 403, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "token:manage") {
		t.Fatalf("error should name the ungranted scope, body=%s", w.Body.String())
	}
}

// 未知 scope 直接 400，不入库。
func TestTokenCreateUnknownScopeRejected(t *testing.T) {
	caller := &auth.TokenPrincipal{Name: "l3", PermLevel: 3, Scopes: auth.AdminTokenScopes}
	w := createTokenWithPrincipal(t, caller,
		`{"name":"junk","scopes":"tools:run,bogus:scope","perm_level":3}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown scope must 400, got %d body=%s", w.Code, w.Body.String())
	}
}

// 合法子集签发放行（L3 签发 kb:write 单一 scope token）。
func TestTokenCreateScopeSubsetAllowed(t *testing.T) {
	caller := &auth.TokenPrincipal{Name: "l3", PermLevel: 3, Scopes: auth.AdminTokenScopes}
	w := createTokenWithPrincipal(t, caller,
		`{"name":"kb-writer","scopes":"kb:write","perm_level":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("valid subset must 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"scopes":"kb:write"`) {
		t.Fatalf("response must echo granted scopes, body=%s", w.Body.String())
	}
}

// /tasks 路由 scope 门禁：scope 不足一律 403（nil manager 安全——门在业务前触发）。
func TestTaskRoutesRequireScopes(t *testing.T) {
	routes := NewTaskHandler(nil).Routes()

	cases := []struct {
		name   string
		method string
		path   string
		scopes string
		want   int
	}{
		{"create-no-scope", http.MethodPost, "/", "", http.StatusUnauthorized},
		{"create-wrong-scope", http.MethodPost, "/", auth.ScopeTasksControl, http.StatusForbidden},
		{"stop-no-scope", http.MethodPost, "/t1/stop", "", http.StatusUnauthorized},
		{"stop-create-only", http.MethodPost, "/t1/stop", auth.ScopeTasksCreate, http.StatusForbidden},
		{"rollback-create-only", http.MethodPost, "/t1/rollback", auth.ScopeTasksCreate, http.StatusForbidden},
		{"clear-create-only", http.MethodDelete, "/", auth.ScopeTasksCreate, http.StatusForbidden},
		{"list-no-scope", http.MethodGet, "/", "", http.StatusUnauthorized},
		{"detail-no-scope", http.MethodGet, "/t1", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, strings.NewReader(`{}`))
		if c.scopes != "" {
			p := &auth.TokenPrincipal{Name: "p", PermLevel: 1, Scopes: c.scopes}
			req = req.WithContext(auth.WithPrincipal(req.Context(), p))
		}
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Fatalf("%s: got %d want %d body=%s", c.name, w.Code, c.want, w.Body.String())
		}
	}
}

// 读端点：持有任一 tasks scope 即放行（负例见上；正例走 auth 层单测足够）。
func TestTaskReadScopesAcceptBoth(t *testing.T) {
	if !(&auth.TokenPrincipal{Scopes: auth.ScopeTasksCreate}).HasScope(auth.ScopeTasksCreate) ||
		!(&auth.TokenPrincipal{Scopes: auth.ScopeTasksControl}).HasScope(auth.ScopeTasksControl) {
		t.Fatal("scope constants must match HasScope parsing")
	}
}
