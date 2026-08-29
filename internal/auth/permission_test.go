package auth

import "testing"

// 策略表对齐真实工具注册表：fs/windows 单工具多 action，
// 键为 tool:action；未命中默认 fail-closed（L2 需确认）。
func TestCheckActionPolicies(t *testing.T) {
	e := NewPermissionEngine()
	cases := []struct {
		tool       string
		userLevel  PermissionLevel
		allowed    bool
		needConfirm bool
	}{
		{"shell:run", Level1Normal, true, false},
		{"fs:read", Level1Normal, true, false},
		{"fs:write", Level1Normal, true, false},
		{"fs:delete", Level1Normal, false, true},  // os.RemoveAll 必须确认
		{"fs:delete", Level2Dangerous, true, false},
		{"windows:keyboard", Level1Normal, true, false},
		{"windows:launch", Level1Normal, false, true}, // 启动外部程序必须确认
		{"computer:screenshot", Level1Normal, false, true},
		{"mcp:anything", Level1Normal, false, true},
		{"safety:classify", Level0ReadOnly, true, false},
		{"future:tool", Level1Normal, false, true}, // 未命中策略 fail-closed
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
	// L0 用户对 L1 工具：低于策略等级但策略 < L2，直接拒绝（不可确认提权）
	got := e.Check("shell:run", Level0ReadOnly)
	if got.Allowed || got.NeedConfirm {
		t.Errorf("L0 user on shell:run should be denied outright, got %+v", got)
	}
}
