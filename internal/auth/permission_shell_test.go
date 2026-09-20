package auth

import "testing"

func TestCheckShellRunRequiresConfirmByDefault(t *testing.T) {
	e := NewPermissionEngine()
	// 改造后：shell:run 默认 L2 → L1 用户 NeedConfirm，不可静默执行
	got := e.Check("shell:run", Level1Normal)
	if got.Allowed {
		t.Fatalf("L1 user must not silently run shell after hardening, got %+v", got)
	}
	if !got.NeedConfirm {
		t.Fatalf("L1 user shell.run should need confirm, got %+v", got)
	}
	// L2 在策略层可过，仍可能被 ClassifyCommand 破坏性门二次确认
	got2 := e.Check("shell:run", Level2Dangerous)
	if !got2.Allowed {
		t.Fatalf("L2 should pass policy for shell:run, got %+v", got2)
	}
}

func TestCheckFsDeleteStillNeedsConfirm(t *testing.T) {
	e := NewPermissionEngine()
	got := e.Check("fs:delete", Level1Normal)
	if got.Allowed || !got.NeedConfirm {
		t.Fatalf("fs:delete L1 should need confirm, got %+v", got)
	}
}

func TestCheckFsReadAllowedL0(t *testing.T) {
	e := NewPermissionEngine()
	got := e.Check("fs:read", Level0ReadOnly)
	if !got.Allowed {
		t.Fatalf("fs:read should be allowed for L0, got %+v", got)
	}
}
