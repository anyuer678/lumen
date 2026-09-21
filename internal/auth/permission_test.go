package auth

import "testing"

// 策略表对齐真实工具注册表；Sprint2：fs 写/删均为 L2。
func TestCheckActionPolicies(t *testing.T) {
	e := NewPermissionEngine()
	cases := []struct {
		tool        string
		userLevel   PermissionLevel
		allowed     bool
		needConfirm bool
	}{
		{"shell:run", Level2Dangerous, true, false},
		{"shell:run", Level1Normal, false, true}, // L2 策略：L1 需确认
		{"fs:read", Level1Normal, true, false},
		{"fs:read", Level0ReadOnly, true, false},
		// Sprint2：write/delete 均为 L2（与 FilesystemTool.ActionRequiredLevel 对齐）
		{"fs:write", Level1Normal, false, true},
		{"fs:write", Level2Dangerous, true, false},
		{"fs:mkdir", Level1Normal, false, true},
		{"fs:delete", Level1Normal, false, true},
		{"fs:delete", Level2Dangerous, true, false},
		{"fs:organize", Level1Normal, false, true},
		{"fs:unknown-action", Level1Normal, false, true}, // fs:* fail-closed L2
		{"exec:argv", Level1Normal, true, false},
		{"windows:keyboard", Level1Normal, true, false},
		{"windows:launch", Level1Normal, false, true},
		{"computer:screenshot", Level1Normal, false, true},
		{"mcp:anything", Level1Normal, false, true},
		{"safety:classify", Level0ReadOnly, true, false},
		{"future:tool", Level1Normal, false, true},
	}
	for _, c := range cases {
		got := e.Check(c.tool, c.userLevel)
		if got.Allowed != c.allowed || got.NeedConfirm != c.needConfirm {
			t.Errorf("Check(%q, L%d) = {allowed:%v needConfirm:%v}, want {allowed:%v needConfirm:%v}",
				c.tool, c.userLevel, got.Allowed, got.NeedConfirm, c.allowed, c.needConfirm)
		}
	}
}

func TestCheckLowUserDeniedWithoutConfirm(t *testing.T) {
	e := NewPermissionEngine()
	// L0 用户对 L1 工具（非 L2 策略）：直接拒绝，不可确认提权
	got := e.Check("windows:keyboard", Level0ReadOnly)
	if got.Allowed || got.NeedConfirm {
		t.Errorf("L0 user on windows:keyboard should be denied outright, got %+v", got)
	}
}

// 空 scopes fail-closed：即使 PermLevel 很高，没有 tools:run 也不能调用工具。
func TestEmptyScopesFailClosedHighPerm(t *testing.T) {
	p := &TokenPrincipal{Name: "legacy", PermLevel: 3, Scopes: ""}
	if p.HasScope(ScopeToolsRun) {
		t.Fatal("L3 empty-scope token must NOT inherit tools:run")
	}
	if p.HasScope(ScopeTokenManage) {
		t.Fatal("L3 empty-scope token must NOT inherit token:manage")
	}
	if p.HasScope(ScopeConfirmApprove) {
		t.Fatal("L3 empty-scope token must NOT inherit confirm:approve")
	}
}

func TestMissingToolsRunScope(t *testing.T) {
	p := &TokenPrincipal{Name: "narrow", PermLevel: 2, Scopes: "tasks:create,events:emit"}
	if p.HasScope(ScopeToolsRun) {
		t.Fatal("token without tools:run must fail HasScope(tools:run)")
	}
}
