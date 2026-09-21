package agent

import (
	"context"
	"strings"
	"testing"
)

// Sprint3：默认 argv 白名单不得包含高副作用解释器/包管理器/网络下载器。
func TestArgvDefaultAllowlistExcludesPowerfulBins(t *testing.T) {
	tool := NewArgvToolWithAllow("./ws", true, nil)
	allow := map[string]bool{}
	for _, b := range tool.ArgvAllowlist() {
		allow[b] = true
	}
	for _, dangerous := range []string{"python", "python3", "node", "npm", "npx", "git", "go", "curl", "wget", "powershell", "cmd", "bash", "certutil", "bitsadmin"} {
		if allow[dangerous] {
			t.Errorf("default argv allowlist must NOT include %q", dangerous)
		}
	}
	// 默认应保留只读/诊断类
	for _, safe := range []string{"echo", "ls", "dir", "cat", "type", "ping", "ipconfig", "whoami", "pwd", "hostname", "date", "true", "false"} {
		if !allow[safe] {
			t.Errorf("default argv allowlist should include safe bin %q", safe)
		}
	}
}

// python/node/git 等默认必须被拒绝；显式 extra_allow 后才允许。
func TestArgvDefaultDeniesPythonNodeGit(t *testing.T) {
	ctx := context.Background()
	tool := NewArgvToolWithAllow("./ws", true, nil)
	for _, bin := range []string{"python", "python3", "node", "git", "go", "npm", "curl"} {
		_, err := tool.Execute(ctx, map[string]any{"bin": bin, "args": []any{"--version"}})
		if err == nil {
			t.Errorf("default argv must deny %q", bin)
		}
	}

	// opt-in extra_allow：仅放行配置中的 basename
	optIn := NewArgvToolWithAllow("./ws", true, []string{"python", "git"})
	if _, ok := optIn.binaryAllowed("python"); !ok {
		t.Fatal("python should be allowed when listed in argv extra_allow")
	}
	if _, ok := optIn.binaryAllowed("node"); ok {
		t.Fatal("node must still be denied when not in extra_allow")
	}
	// 路径形式 extra 必须被忽略
	pathy := NewArgvToolWithAllow("./ws", true, []string{`C:\evil\python.exe`, "../python"})
	if _, ok := pathy.binaryAllowed("python"); ok {
		t.Fatal("path-form extra_allow entries must be ignored")
	}
}

// 参数校验：拒绝绝对路径、路径分隔符、环境变量样式。
func TestArgvRejectsPathsAndEnvStyleArgs(t *testing.T) {
	ctx := context.Background()
	tool := NewArgvToolWithAllow("./ws", true, nil)
	cases := []struct {
		name string
		args []any
	}{
		{"unix abs", []any{"/etc/passwd"}},
		{"windows abs", []any{`C:\Windows\System32\config\SAM`}},
		{"unc", []any{`\\evil\share\payload`}},
		{"rel path sep", []any{"../../etc/passwd"}},
		{"env dollar", []any{"$PATH"}},
		{"env percent", []any{"%PATH%"}},
		{"env home", []any{"$HOME"}},
	}
	for _, c := range cases {
		_, err := tool.Execute(ctx, map[string]any{"bin": "echo", "args": c.args})
		if err == nil {
			t.Errorf("case %q: expected argv arg rejection, got success", c.name)
		}
		if err != nil && !strings.Contains(err.Error(), "argv") && !strings.Contains(err.Error(), "路径") && !strings.Contains(err.Error(), "环境变量") {
			t.Logf("case %q rejected with: %v", c.name, err)
		}
	}

	// 白名单外二进制仍拒绝（路径形式 bin）
	if _, err := tool.Execute(ctx, map[string]any{"bin": `/bin/ls`, "args": []any{}}); err == nil {
		t.Fatal("absolute path bin must be denied")
	}
}

// NewArgvTool（经 config）在未配置 extra 时也只含默认安全列表。
func TestArgvToolFromConfigUsesSafeDefaults(t *testing.T) {
	tool := NewArgvTool("./ws", true)
	allow := map[string]bool{}
	for _, b := range tool.ArgvAllowlist() {
		allow[b] = true
	}
	if allow["python"] || allow["node"] || allow["git"] {
		t.Fatalf("NewArgvTool without config extra must not include powerful bins, got %v", tool.ArgvAllowlist())
	}
}
