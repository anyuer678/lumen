package agent

import (
	"testing"
)

func TestGetTestSuiteV2(t *testing.T) {
	tasks := GetTestSuiteV2()
	if len(tasks) < 25 {
		t.Errorf("GetTestSuiteV2() returned %d tasks, want >= 25", len(tasks))
	}

	// 检查每个任务有必要的字段
	seen := map[string]bool{}
	for _, bt := range tasks {
		if bt.ID == "" {
			t.Errorf("task missing ID: %+v", bt)
		}
		if seen[bt.ID] {
			t.Errorf("duplicate task ID: %s", bt.ID)
		}
		seen[bt.ID] = true
		if bt.Category == "" {
			t.Errorf("task %s missing category", bt.ID)
		}
		if bt.Difficulty == "" {
			t.Errorf("task %s missing difficulty", bt.ID)
		}
		if bt.Goal == "" {
			t.Errorf("task %s missing goal", bt.ID)
		}
		if bt.MaxSteps <= 0 {
			t.Errorf("task %s has invalid max_steps: %d", bt.ID, bt.MaxSteps)
		}
	}
}

func TestIsToolCorrect(t *testing.T) {
	tests := []struct {
		selected, expected string
		want               bool
	}{
		{"shell.run", "shell.run", true},
		{"windows", "windows", true},
		{"windows.powershell", "windows", true},
		{"system.disk", "system", true},
		{"shell.run", "fs", false},
		{"browser", "shell.run", false},
	}
	for _, tt := range tests {
		got := isToolCorrect(tt.selected, tt.expected)
		if got != tt.want {
			t.Errorf("isToolCorrect(%q, %q) = %v, want %v", tt.selected, tt.expected, got, tt.want)
		}
	}
}

func TestComputeMetrics(t *testing.T) {
	report := &BenchReportV2{
		Total:  3,
		Passed: 2,
		Failed: 1,
		Results: []BenchResultV2{
			{Status: "pass", ToolSelected: "shell.run", ToolCorrect: true, Duration: 1.0, TokensUsed: 100},
			{Status: "pass", ToolSelected: "browser", ToolCorrect: true, Duration: 2.0, TokensUsed: 200},
			{Status: "fail", ToolSelected: "shell.run", ToolCorrect: false, Duration: 3.0, TokensUsed: 150},
		},
	}
	computeMetrics(report)

	if report.Metrics.OverallSuccess != 66.66666666666667 && report.Metrics.OverallSuccess != 66.7 {
		// 2/3 = 66.67%
		if report.Metrics.OverallSuccess < 66 || report.Metrics.OverallSuccess > 67 {
			t.Errorf("OverallSuccess = %.1f, want ~66.7", report.Metrics.OverallSuccess)
		}
	}
	if report.Metrics.ToolSelection != 66.66666666666667 && report.Metrics.ToolSelection != 66.7 {
		if report.Metrics.ToolSelection < 66 || report.Metrics.ToolSelection > 67 {
			t.Errorf("ToolSelection = %.1f, want ~66.7", report.Metrics.ToolSelection)
		}
	}
	if report.Metrics.TotalTokens != 450 {
		t.Errorf("TotalTokens = %d, want 450", report.Metrics.TotalTokens)
	}
}

func BenchmarkGetTestSuiteV2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetTestSuiteV2()
	}
}
