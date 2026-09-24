package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"agent/internal/auth"
	"agent/internal/db"
)

// newEventbusAuthzRouter 按 router.go 的真实接线构造 eventbus 测试路由
//（外层 RequireAnyScope(events:read, events:emit) + 内层 per-route 收紧）。
func newEventbusAuthzRouter(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES", "")
	database, err := db.Init(filepath.Join(t.TempDir(), "eventbus_authz.db"))
	if err != nil {
		t.Fatalf("init temp db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	h := NewEventHandler(database)
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAnyScope(auth.ScopeEventsRead, auth.ScopeEventsEmit))
		r.Mount("/eventbus", h.Routes())
	})
	return r
}

func doEventbusReq(h http.Handler, method, target, scopes string, permLevel int) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(`{"type":"test.event"}`))
	if scopes != "" {
		req = req.WithContext(auth.WithPrincipal(req.Context(), &auth.TokenPrincipal{
			Name: "t", PermLevel: permLevel, Scopes: scopes,
		}))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// 只读 token（events:read）可读事件列表 —— events:read 的核心用途。
func TestEventbusReadScopeCanList(t *testing.T) {
	h := newEventbusAuthzRouter(t)
	w := doEventbusReq(h, http.MethodGet, "/eventbus", auth.ScopeEventsRead, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("events:read token must GET /eventbus 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "[") {
		t.Fatalf("expected JSON array body, got %q", w.Body.String())
	}
}

// emit 持有者向后兼容：仍可读。
func TestEventbusEmitScopeCanListBackcompat(t *testing.T) {
	h := newEventbusAuthzRouter(t)
	w := doEventbusReq(h, http.MethodGet, "/eventbus", auth.ScopeEventsEmit, 2)
	if w.Code != http.StatusOK {
		t.Fatalf("events:emit token must keep read access (back-compat), got %d", w.Code)
	}
}

// 只读 token 不得发射事件 —— 内层 events:emit 门禁必须兜住。
func TestEventbusReadScopeCannotEmit(t *testing.T) {
	h := newEventbusAuthzRouter(t)
	w := doEventbusReq(h, http.MethodPost, "/eventbus/emit", auth.ScopeEventsRead, 3)
	if w.Code != http.StatusForbidden {
		t.Fatalf("events:read token must NOT emit (403), got %d body=%s", w.Code, w.Body.String())
	}
}

// 只读 token 不得清理事件历史。
func TestEventbusReadScopeCannotClear(t *testing.T) {
	h := newEventbusAuthzRouter(t)
	w := doEventbusReq(h, http.MethodDelete, "/eventbus", auth.ScopeEventsRead, 3)
	if w.Code != http.StatusForbidden {
		t.Fatalf("events:read token must NOT clear (403), got %d body=%s", w.Code, w.Body.String())
	}
}

// 两个 scope 都没有 → 403；未认证 → 401。
func TestEventbusScopeGateDeny(t *testing.T) {
	h := newEventbusAuthzRouter(t)
	w := doEventbusReq(h, http.MethodGet, "/eventbus", auth.ScopeToolsRun, 1)
	if w.Code != http.StatusForbidden {
		t.Fatalf("token without read/emit must 403, got %d", w.Code)
	}
	w2 := doEventbusReq(h, http.MethodGet, "/eventbus", "", 1)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated must 401, got %d", w2.Code)
	}
}

// emit 持有者回归：发射与清理路径仍可用（perm 门槛不变）。
func TestEventbusEmitScopeStillWorks(t *testing.T) {
	h := newEventbusAuthzRouter(t)
	w := doEventbusReq(h, http.MethodPost, "/eventbus/emit", auth.ScopeEventsEmit, 2)
	if w.Code != http.StatusOK {
		t.Fatalf("events:emit L2 must still emit 200, got %d body=%s", w.Code, w.Body.String())
	}
	w2 := doEventbusReq(h, http.MethodPost, "/eventbus/emit", auth.ScopeEventsEmit, 1)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("events:emit L1 must still be denied by L2 perm gate, got %d", w2.Code)
	}
}
