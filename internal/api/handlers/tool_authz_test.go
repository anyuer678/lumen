package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent/internal/agent"
	"agent/internal/auth"
)

func ctxWithPrincipal(p *auth.TokenPrincipal) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/tools/x/run", strings.NewReader(`{"args":{}}`))
	return req.WithContext(auth.WithPrincipal(context.Background(), p))
}

// ToolHandler.RunTool：缺少 tools:run scope 必须 403。
func TestRunToolMissingToolsRunScope(t *testing.T) {
	osUnsetLegacy := func() { t.Setenv("LUMEN_ALLOW_LEGACY_EMPTY_SCOPES", "") }
	osUnsetLegacy()

	h := &ToolHandler{} // loop 为 nil：scope 门应在触达 loop 前触发
	req := ctxWithPrincipal(&auth.TokenPrincipal{
		Name: "no-scope", PermLevel: 3, Scopes: "tasks:create",
	})
	w := httptest.NewRecorder()
	h.RunTool(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing tools:run must 403, got %d body=%s", w.Code, w.Body.String())
	}

	// 空 scopes 即使 L3 也 403
	req2 := ctxWithPrincipal(&auth.TokenPrincipal{Name: "empty", PermLevel: 3, Scopes: ""})
	w2 := httptest.NewRecorder()
	h.RunTool(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("empty scopes must 403, got %d", w2.Code)
	}

	// 未认证 principal → 401
	req3 := httptest.NewRequest(http.MethodPost, "/v1/tools/x/run", strings.NewReader(`{}`))
	w3 := httptest.NewRecorder()
	h.RunTool(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("nil principal must 401, got %d", w3.Code)
	}
}

// actionRequiredLevel：fs delete/write 必须 L2；read 保持 L0。
func TestActionRequiredLevelFs(t *testing.T) {
	fsMeta := agent.ToolMeta{Name: "fs", RequiredLevel: 0}
	shellMeta := agent.ToolMeta{Name: "shell.run", RequiredLevel: 2}

	cases := []struct {
		meta agent.ToolMeta
		args map[string]any
		want int
	}{
		{fsMeta, map[string]any{"action": "read"}, 0},
		{fsMeta, map[string]any{"action": "list"}, 0},
		{fsMeta, map[string]any{"action": "exists"}, 0},
		{fsMeta, map[string]any{"action": "write"}, 2},
		{fsMeta, map[string]any{"action": "mkdir"}, 2},
		{fsMeta, map[string]any{"action": "delete"}, 2},
		{fsMeta, map[string]any{"action": "organize"}, 2},
		{fsMeta, map[string]any{}, 0}, // 无 action：工具级下限（handler 再交 RunTool 策略）
		{shellMeta, map[string]any{"command": "echo"}, 2},
		{shellMeta, map[string]any{}, 2},
	}
	for _, c := range cases {
		got := actionRequiredLevel(c.meta, c.args)
		if got != c.want {
			t.Errorf("actionRequiredLevel(%s, %v) = %d, want %d", c.meta.Name, c.args, got, c.want)
		}
	}
}

// L1 token 调 fs delete：即使有 tools:run，也因 L2 需求被拒（无 loop 时先过 scope，
// 此处直接测 actionRequiredLevel > PermLevel 的判定逻辑）。
func TestFsDeleteRequiresL2Token(t *testing.T) {
	p := &auth.TokenPrincipal{Name: "l1", PermLevel: 1, Scopes: auth.ScopeToolsRun}
	if !p.HasScope(auth.ScopeToolsRun) {
		t.Fatal("setup: L1 with tools:run should have scope")
	}
	need := actionRequiredLevel(agent.ToolMeta{Name: "fs", RequiredLevel: 0},
		map[string]any{"action": "delete"})
	if need <= p.PermLevel {
		t.Fatalf("fs delete need L%d must exceed L1 token", need)
	}
}
