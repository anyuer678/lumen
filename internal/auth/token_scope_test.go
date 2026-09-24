package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHasScopeEmptyScopesFailClosed(t *testing.T) {
	os.Unsetenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES")
	p := &TokenPrincipal{Scopes: ""}
	if p.HasScope("tools:run") {
		t.Fatal("empty scopes must NOT grant all scopes by default")
	}
	if p.HasScope("token:manage") {
		t.Fatal("empty scopes must not grant token:manage")
	}
	// 即使 PermLevel=3 也 fail-closed
	p3 := &TokenPrincipal{Scopes: "", PermLevel: 3}
	if p3.HasScope(ScopeToolsRun) || p3.HasScope(ScopeTokenManage) {
		t.Fatal("empty scopes must not grant scopes even for L3")
	}
}

func TestHasScopeNilPrincipal(t *testing.T) {
	var p *TokenPrincipal
	if p.HasScope(ScopeToolsRun) {
		t.Fatal("nil principal must fail HasScope")
	}
}

func TestHasScopeLegacyFlagAllowsEmpty(t *testing.T) {
	t.Setenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES", "1")
	p := &TokenPrincipal{Scopes: ""}
	if !p.HasScope("tools:run") {
		t.Fatal("legacy flag should temporarily allow empty-scope tokens")
	}
}

func TestHasScopeExplicitList(t *testing.T) {
	os.Unsetenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES")
	p := &TokenPrincipal{Scopes: "tools:run, tasks:control"}
	if !p.HasScope("tools:run") {
		t.Fatal("explicit tools:run should pass")
	}
	if p.HasScope("confirm:approve") {
		t.Fatal("confirm:approve not in list should fail")
	}
}

func TestRequireScopeDeniesMissing(t *testing.T) {
	os.Unsetenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES")
	h := RequireScope("confirm:approve")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/confirmations/x/approve", nil)
	req = req.WithContext(WithPrincipal(req.Context(), &TokenPrincipal{
		Name: "t1", PermLevel: 3, Scopes: "tools:run",
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 without confirm:approve, got %d", rec.Code)
	}
}

func TestQueryTokenOnlyForEvents(t *testing.T) {
	// 写/工具类 API：?token= 必须被忽略
	for _, path := range []string{
		"/v1/tools", "/v1/tools/run", "/v1/tools/shell.run/run",
		"/v1/confirmations/x/approve", "/v1/auth/token", "/v1/settings",
		"/v1/mcp/servers", "/v1/tasks",
	} {
		req := httptest.NewRequest(http.MethodPost, path+"?token=secret", nil)
		if extractToken(req) != "" {
			t.Fatalf("?token= must not work on write/tool path %s", path)
		}
	}
	// SSE/事件流仍允许（EventSource 无法带 Header）
	for _, path := range []string{"/v1/events", "/v1/sse", "/v1/stream"} {
		req2 := httptest.NewRequest(http.MethodGet, path+"?token=secret", nil)
		if extractToken(req2) != "secret" {
			t.Fatalf("?token= should still work on %s for EventSource", path)
		}
	}
}

func TestDefaultAndAdminScopeConstants(t *testing.T) {
	if DefaultTokenScopes != "tools:run,tasks:create" {
		t.Fatalf("DefaultTokenScopes changed: %q", DefaultTokenScopes)
	}
	admin := &TokenPrincipal{Scopes: AdminTokenScopes, PermLevel: 3}
	for _, s := range []string{
		ScopeToolsRun, ScopeTasksCreate, ScopeTasksControl, ScopeConfirmApprove,
		ScopeMCPRegister, ScopeTokenManage, ScopeEventsEmit, ScopeEventsRead, ScopeKBWrite, ScopeSettingsWrite,
	} {
		if !admin.HasScope(s) {
			t.Fatalf("AdminTokenScopes missing %s", s)
		}
	}
}

func TestRequireAnyScopeGrantsOnFirstMatch(t *testing.T) {
	os.Unsetenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES")
	h := RequireAnyScope(ScopeEventsRead, ScopeEventsEmit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, scopes := range []string{ScopeEventsRead, ScopeEventsEmit, "tools:run," + ScopeEventsRead} {
		req := httptest.NewRequest(http.MethodGet, "/eventbus", nil)
		req = req.WithContext(WithPrincipal(req.Context(), &TokenPrincipal{
			Name: "t1", PermLevel: 1, Scopes: scopes,
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("scopes=%q should pass RequireAnyScope, got %d", scopes, rec.Code)
		}
	}
}

func TestRequireAnyScopeDeniesWhenNoneMatch(t *testing.T) {
	os.Unsetenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES")
	h := RequireAnyScope(ScopeEventsRead, ScopeEventsEmit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// 无匹配 scope → 403
	req := httptest.NewRequest(http.MethodGet, "/eventbus", nil)
	req = req.WithContext(WithPrincipal(req.Context(), &TokenPrincipal{
		Name: "t1", PermLevel: 3, Scopes: ScopeToolsRun,
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 when no listed scope granted, got %d", rec.Code)
	}
	// 未认证 → 401
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/eventbus", nil))
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("nil principal must 401, got %d", rec2.Code)
	}
	// 空 scopes 即使 L3 也 fail-closed
	req3 := httptest.NewRequest(http.MethodGet, "/eventbus", nil)
	req3 = req3.WithContext(WithPrincipal(req3.Context(), &TokenPrincipal{
		Name: "t2", PermLevel: 3, Scopes: "",
	}))
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("empty scopes must fail closed, got %d", rec3.Code)
	}
}
