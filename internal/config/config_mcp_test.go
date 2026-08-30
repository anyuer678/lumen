package config

import (
	"testing"
)

func TestMcpServerConfig(t *testing.T) {
	cfg := &Config{}
	cfg.MCP.Servers = []McpServerConfig{
		{Name: "pgi", Command: "python", Args: []string{"-m", "pgi", "mcp"}, Transport: "stdio"},
	}

	if len(cfg.MCP.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.MCP.Servers))
	}
	srv := cfg.MCP.Servers[0]
	if srv.Name != "pgi" {
		t.Errorf("name = %q, want pgi", srv.Name)
	}
	if srv.Command != "python" {
		t.Errorf("command = %q, want python", srv.Command)
	}
	if srv.Transport != "stdio" {
		t.Errorf("transport = %q, want stdio", srv.Transport)
	}
}

func TestMcpServerConfigEmpty(t *testing.T) {
	cfg := &Config{}
	if len(cfg.MCP.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(cfg.MCP.Servers))
	}
}
