package toolrepair

import (
	"testing"
)

func TestRepairJSON_TrailingComma(t *testing.T) {
	input := `{"steps": [{"tool": "shell.run", "args": {"command": "ls",},},],}`
	result := RepairJSON(input)
	if !result.Fixed {
		t.Errorf("RepairJSON should fix trailing commas, got fixes: %v", result.Fixes)
	}
}

func TestRepairJSON_MarkdownBlock(t *testing.T) {
	input := "Here is the plan:\n```json\n{\"steps\": [{\"tool\": \"shell.run\"}]}\n```\nDone."
	result := RepairJSON(input)
	if !result.Fixed {
		t.Errorf("RepairJSON should extract from markdown, got fixes: %v", result.Fixes)
	}
}

func TestRepairJSON_SurroundingText(t *testing.T) {
	input := "Sure! Here's the plan: {\"steps\": [{\"tool\": \"shell.run\"}]} and that's it."
	result := RepairJSON(input)
	if !result.Fixed {
		t.Errorf("RepairJSON should extract from surrounding text, got fixes: %v", result.Fixes)
	}
}

func TestRepairJSON_Truncated(t *testing.T) {
	input := `{"steps": [{"tool": "shell.run", "args": {"command": "ls"}`
	result := RepairJSON(input)
	if !result.Fixed {
		t.Errorf("RepairJSON should fix truncated JSON, got fixes: %v", result.Fixes)
	}
}

func TestRepairJSON_AlreadyValid(t *testing.T) {
	input := `{"steps": [{"tool": "shell.run"}]}`
	result := RepairJSON(input)
	if !result.Fixed {
		t.Errorf("RepairJSON should accept valid JSON")
	}
}

func TestRepairJSON_Empty(t *testing.T) {
	result := RepairJSON("")
	if result.Fixed {
		t.Errorf("RepairJSON should not fix empty string")
	}
}

func TestRepairToolName_Dotted(t *testing.T) {
	tools := map[string]bool{"shell.run": true, "fs": true, "browser": true, "system": true, "windows": true}
	tests := []struct{ input, want string }{
		{"windows.powershell", "windows"},
		{"system.disk", "system"},
		{"shell.run", "shell.run"},
		{"fs", "fs"},
	}
	for _, tt := range tests {
		got := RepairToolName(tt.input, tools)
		if got != tt.want {
			t.Errorf("RepairToolName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractToolCallsFromText(t *testing.T) {
	text := `I'll help you with that.

<tool_call>
{"name": "shell.run", "arguments": {"command": "echo hello"}}
</tool_call>

Let me know if you need more help.`

	calls := ExtractToolCallsFromText(text)
	if len(calls) != 1 {
		t.Errorf("ExtractToolCallsFromText should find 1 call, got %d", len(calls))
	}
}

func TestRepairToolArgs_TimeoutString(t *testing.T) {
	args := map[string]any{"command": "ls", "timeout": "30"}
	fixed := RepairToolArgs("shell.run", args, nil)
	if _, ok := fixed["timeout"].(int); !ok {
		t.Errorf("timeout should be int after repair, got %T", fixed["timeout"])
	}
}

func TestRepairToolArgs_DefaultAction(t *testing.T) {
	args := map[string]any{"path": "."}
	fixed := RepairToolArgs("fs", args, nil)
	if fixed["action"] != "list" {
		t.Errorf("fs default action should be 'list', got %v", fixed["action"])
	}
}

func BenchmarkRepairJSON(b *testing.B) {
	input := `{"steps": [{"description": "test", "tool": "shell.run", "args": {"command": "ls -la"}}]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RepairJSON(input)
	}
}
