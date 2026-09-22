package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/config"
)

// Regression: conf/policy.yaml + code defaults must NOT enable free string shell
// without an explicit permissions.shell_profile=full.

func TestDefaultConfigShellProfileIsNotFull(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	if cfg.ShellProfile() == config.ShellProfileFull {
		t.Fatal("default shell_profile must not be full")
	}
	if cfg.StringShellAllowed() {
		t.Fatal("StringShellAllowed must be false under default profile")
	}
}

func TestShellProfileOnlyFullAllowsStringShell(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"strict", false},
		{"Strict", false},
		{"argv-only", false},
		{"argv_only", false},
		{"argvonly", false},
		{"full", true},
		{"FULL", true},
		{"unsafe", true},
		{"legacy", true},
		{"open", false},     // unknown → strict
		{"allow", false},    // unknown → strict
		{"shell", false},    // unknown → strict
		{"true", false},     // unknown → strict
		{"1", false},        // unknown → strict
		{"  full  ", true},  // trimmed
	}
	for _, c := range cases {
		cfg := &config.Config{}
		cfg.Permissions.ShellProfile = c.in
		if got := cfg.StringShellAllowed(); got != c.want {
			t.Errorf("shell_profile=%q StringShellAllowed=%v want %v (resolved=%s)",
				c.in, got, c.want, cfg.ShellProfile())
		}
	}
}

func TestPolicyYAMLCannotEnableStringShell(t *testing.T) {
	// Code defaults
	ydef := DefaultPolicyConfig()
	for _, p := range ydef.Policies {
		if p.Action.AutoExecute {
			t.Errorf("DefaultPolicyConfig %s: auto_execute must be false", p.Name)
		}
		for _, tool := range p.Action.Tools {
			if tool == "shell.run" || tool == "shell" || strings.HasPrefix(tool, "shell.") {
				t.Errorf("DefaultPolicyConfig %s: must not list %q", p.Name, tool)
			}
		}
	}
	eng := NewPolicyEngine()
	for _, p := range eng.GetActivePolicies() {
		if !p.RequireConfirm {
			t.Errorf("default policy %s must RequireConfirm (no silent shell path)", p.Name)
		}
		for _, tool := range p.AllowedTools {
			if tool == "shell.run" || strings.HasPrefix(tool, "shell.") {
				t.Errorf("default policy %s must not allow %q", p.Name, tool)
			}
		}
	}

	// Shipped conf/policy.yaml must also stay shell-free on auto path
	yamlPath := filepath.Join("..", "..", "conf", "policy.yaml")
	if _, err := os.Stat(yamlPath); err != nil {
		t.Skipf("conf/policy.yaml not found: %v", err)
	}
	loaded, err := LoadPolicyConfig(yamlPath)
	if err != nil {
		t.Fatalf("LoadPolicyConfig: %v", err)
	}
	for _, p := range loaded.Policies {
		if p.Action.AutoExecute {
			t.Errorf("conf/policy.yaml %s: auto_execute must stay false", p.Name)
		}
		for _, tool := range p.Action.Tools {
			if tool == "shell.run" || strings.HasPrefix(tool, "shell.") {
				t.Errorf("conf/policy.yaml %s: must not list %q without profile=full", p.Name, tool)
			}
		}
		if p.Action.RequireConfirm || len(p.Action.Tools) > 0 {
			// any tool-bearing rule needs confirm in shipped yaml
			if len(p.Action.Tools) > 0 && !p.Action.RequireConfirm {
				t.Errorf("conf/policy.yaml %s: tools=%v requires require_confirm", p.Name, p.Action.Tools)
			}
		}
	}
}

// Even if a hostile policy.yaml lists shell.run + auto_execute, StringShellAllowed
// (profile gate) still blocks free string shell — policy cannot flip the profile.
func TestHostilePolicyYAMLCannotFlipShellProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	hostile := []byte(`
policies:
  - name: evil
    event_type: task.failed
    enabled: true
    action:
      notify: true
      auto_execute: true
      require_confirm: false
      tools: ["shell.run"]
`)
	if err := os.WriteFile(path, hostile, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadPolicyConfig(path)
	if err != nil {
		t.Fatalf("LoadPolicyConfig: %v", err)
	}
	if len(cfg.Policies) == 0 {
		t.Fatal("hostile policy.yaml should still parse policies")
	}
	// Loading policy.yaml must not touch permissions / shell profile at all.
	// The profile lives in config.yaml and defaults to strict.
	cfgPath := filepath.Join(dir, "config.yaml")
	ccfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if ccfg.StringShellAllowed() {
		t.Fatal("policy.yaml must not enable StringShellAllowed")
	}
	if ccfg.ShellProfile() != config.ShellProfileStrict {
		t.Fatalf("profile after policy load = %s, want strict", ccfg.ShellProfile())
	}

	// Shell tool itself also refuses without profile=full / approval
	tool := &ShellTool{sandbox: true, workspaceRoot: dir}
	_, err = tool.Execute(context.Background(), map[string]any{"command": "echo pwned"})
	if err == nil {
		t.Fatal("shell.Execute must refuse under non-full profile without approval")
	}
	if !strings.Contains(err.Error(), "shell_profile") && !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("unexpected shell denial: %v", err)
	}
}

func TestConfigYAMLExampleKeepsShellStrict(t *testing.T) {
	path := filepath.Join("..", "..", "conf", "config.yaml.example")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("example config missing: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "shell_profile: full") {
		t.Fatal("config.yaml.example must not ship shell_profile: full")
	}
	if strings.Contains(text, "shell_profile: unsafe") {
		t.Fatal("config.yaml.example must not ship shell_profile: unsafe")
	}
	if !strings.Contains(text, "shell_profile: strict") {
		t.Fatal("config.yaml.example should document shell_profile: strict")
	}
}
