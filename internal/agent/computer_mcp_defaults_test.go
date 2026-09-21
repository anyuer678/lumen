package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/config"
	agentDB "agent/internal/db"
	"go.uber.org/zap"
)

func loadDefaultConfig(t *testing.T) *config.Config {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	t.Cleanup(func() {
		// restore safe defaults for subsequent tests in this package
		cfg.Permissions.ComputerUseEnabled = false
		cfg.MCP.Enabled = false
		cfg.Permissions.ArgvExtraAllow = nil
	})
	return cfg
}

// Sprint3：默认配置下 computer / mcp 不可调用。
func TestDefaultConfigDisablesComputerAndMCP(t *testing.T) {
	cfg := loadDefaultConfig(t)
	if cfg.ComputerUseEnabled() {
		t.Fatal("computer_use_enabled must default to false")
	}
	if cfg.MCPEnabled() {
		t.Fatal("mcp.enabled must default to false")
	}
	if len(cfg.ArgvExtraAllow()) != 0 {
		t.Fatalf("argv_extra_allow must default empty, got %v", cfg.ArgvExtraAllow())
	}

	dbPath := filepath.Join(t.TempDir(), "lumen-test.db")
	database, err := agentDB.Init(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	loop := NewLoop(database, zap.NewNop(), nil)

	// 工具列表不得包含 computer
	for _, meta := range loop.ListTools() {
		if meta.Name == "computer" {
			t.Fatal("computer tool must not be registered under default config")
		}
	}

	ctx := context.Background()
	_, err = loop.RunTool(ctx, "computer", map[string]any{"action": "screenshot"})
	if err == nil {
		t.Fatal("default config must not allow computer tool")
	}
	if !strings.Contains(err.Error(), "computer") && !strings.Contains(err.Error(), "disabled") && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected computer denial: %v", err)
	}

	_, err = loop.RunTool(ctx, "mcp", map[string]any{"name": "evil"})
	if err == nil {
		t.Fatal("default config must not allow mcp tool")
	}

	// 即便工具被手动注册，Execute 仍须拒绝
	ct := NewComputerTool("./ws")
	if _, err := ct.Execute(ctx, map[string]any{"action": "window_list"}); err == nil {
		t.Fatal("ComputerTool.Execute must refuse when feature flag is off")
	}

	reg := NewMcpRegistry()
	err = reg.Register(ctx, McpServer{Name: "x", Command: "node", Args: []string{"-e", "1"}})
	if err == nil || (!strings.Contains(err.Error(), "mcp") && !strings.Contains(err.Error(), "disabled")) {
		t.Fatalf("McpRegistry.Register must refuse when mcp.enabled=false, got %v", err)
	}

	adapter := &McpToolAdapter{client: NewMcpClient(McpServer{Name: "x", Command: "node"}), tool: McpToolDef{Name: "x.tool"}}
	if _, err := adapter.Execute(ctx, map[string]any{}); err == nil {
		t.Fatal("McpToolAdapter.Execute must refuse when mcp.enabled=false")
	}
}

// 显式 opt-in 后 computer 可注册（仍保持 L2 权限级别）。
func TestComputerEnabledWhenExplicitlyOptedIn(t *testing.T) {
	cfg := loadDefaultConfig(t)
	cfg.Permissions.ComputerUseEnabled = true

	dbPath := filepath.Join(t.TempDir(), "lumen-test.db")
	database, err := agentDB.Init(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
		cfg.Permissions.ComputerUseEnabled = false
	})

	loop := NewLoop(database, zap.NewNop(), nil)
	found := false
	for _, meta := range loop.ListTools() {
		if meta.Name == "computer" {
			found = true
			if meta.RequiredLevel < 2 {
				t.Errorf("computer RequiredLevel=%d want >=2", meta.RequiredLevel)
			}
		}
	}
	if !found {
		t.Fatal("computer tool should be registered when computer_use_enabled=true")
	}
}

// conf/policy.yaml features 字段：computer_use / mcp_register 默认 false。
func TestPolicyYAMLFeaturesDefaultOff(t *testing.T) {
	// 代码默认策略不得包含 computer/mcp 的静默放行
	eng := NewPolicyEngine()
	for _, p := range eng.GetActivePolicies() {
		for _, tool := range p.AllowedTools {
			if tool == "computer" || tool == "mcp" || strings.HasPrefix(tool, "mcp.") {
				t.Errorf("default policy %s must not allow tool %q without explicit enable", p.Name, tool)
			}
		}
	}
	ydef := DefaultPolicyConfig()
	for _, p := range ydef.Policies {
		if p.Action.AutoExecute {
			t.Errorf("DefaultPolicyConfig policy %s auto_execute must be false", p.Name)
		}
		for _, tool := range p.Action.Tools {
			if tool == "shell.run" || tool == "computer" || tool == "mcp" {
				t.Errorf("DefaultPolicyConfig policy %s must not auto-allow %q", p.Name, tool)
			}
		}
	}
}
