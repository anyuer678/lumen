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
	req := httptest.NewRequest(http.MethodGet, "/v1/tools?token=secret", nil)
	if extractToken(req) != "" {
		t.Fatal("?token= must not work on /v1/tools")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/v1/events?token=secret", nil)
	if extractToken(req2) != "secret" {
		t.Fatal("?token= should still work on /v1/events for EventSource")
	}
}
