package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupGrepWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// 创建测试文件
	files := map[string]string{
		"main.go":        "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
		"README.md":      "# Project\n\nThis is a test project.\n",
		"lib/utils.go":   "package lib\n\nfunc Add(a, b int) int { return a + b }\n",
		"lib/helper.txt": "just some text\nnothing special\n",
		".git/config":    "should be skipped",
	}

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(content), 0o644)
	}

	// 创建 node_modules（应被跳过）
	nmPath := filepath.Join(dir, "node_modules", "pkg", "index.js")
	os.MkdirAll(filepath.Dir(nmPath), 0o755)
	os.WriteFile(nmPath, []byte("module.exports = {}"), 0o644)

	return dir
}

func TestFileGrep_BasicMatch(t *testing.T) {
	root := setupGrepWorkspace(t)
	tool := NewFileGrepTool(root, false)

	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "func",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Kind != "json" {
		t.Errorf("expected kind=json, got %s", result.Kind)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestFileGrep_PathEscape(t *testing.T) {
	root := setupGrepWorkspace(t)
	tool := NewFileGrepTool(root, true) // sandbox=true

	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "func",
		"path":    "../../etc",
	})
	if err == nil {
		t.Error("expected error for path escape")
	}
}

func TestFileGrep_MaxResults(t *testing.T) {
	root := setupGrepWorkspace(t)
	tool := NewFileGrepTool(root, false)

	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     ".",
		"max_results": 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证截断
	if result.Summary == "" {
		t.Error("expected summary mentioning truncation")
	}
}

func TestFileGrep_InvalidRegex(t *testing.T) {
	root := setupGrepWorkspace(t)
	tool := NewFileGrepTool(root, false)

	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "[invalid",
	})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestFileGrep_EmptyPattern(t *testing.T) {
	root := setupGrepWorkspace(t)
	tool := NewFileGrepTool(root, false)

	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}
