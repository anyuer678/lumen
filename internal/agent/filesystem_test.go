package agent

import "testing"
func TestCheckSandbox_EscapeViaPrefix(t *testing.T) {
	// workspace-evil 以 workspace 为前缀但不是子路径——HasPrefix 会误放行
	tool := NewFilesystemTool("./workspace", true)
	err := tool.checkSandbox("/tmp/workspace-evil/payload")
	if err == nil {
		t.Fatal("expected denied for /tmp/workspace-evil but got through")
	}
}

func TestCheckSandbox_Accepted(t *testing.T) {
	tool := NewFilesystemTool("./workspace", true)
	// 正常子路径
	err := tool.checkSandbox("./workspace/safe/file.txt")
	if err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}
}
