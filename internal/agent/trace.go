package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TraceStage 追踪阶段
type TraceStage string

const (
	StageIntent       TraceStage = "intent"
	StagePlanner      TraceStage = "planner"
	StageContext      TraceStage = "context"
	StageToolSelect   TraceStage = "tool_select"
	StagePermission   TraceStage = "permission"
	StageExecution    TraceStage = "execution"
	StageRepair       TraceStage = "repair"
	StageEvaluator    TraceStage = "evaluator"
	StageReplanner    TraceStage = "replanner"
	StageMemory       TraceStage = "memory"
	StageFeedback     TraceStage = "feedback"
	StageEvent        TraceStage = "event"
	StageProactive    TraceStage = "proactive"
)

// TraceSpan 一个追踪段
type TraceSpan struct {
	TaskID    string     `json:"task_id"`
	Stage     TraceStage `json:"stage"`
	Input     string     `json:"input,omitempty"`     // 输入摘要
	Output    string     `json:"output,omitempty"`    // 输出摘要
	LatencyMs int64      `json:"latency_ms"`          // 耗时（毫秒）
	Success   bool       `json:"success"`
	Error     string     `json:"error,omitempty"`
	TS        int64      `json:"ts"`                  // 时间戳（unix ms）
	Seq       int        `json:"seq"`                 // 序号
}

// TraceRecorder 追踪记录器
type TraceRecorder struct {
	mu     sync.Mutex
	spans  []TraceSpan
	seq    int
	taskID string
	dir    string
}

// NewTraceRecorder 创建追踪记录器
func NewTraceRecorder(dir, taskID string) *TraceRecorder {
	return &TraceRecorder{
		dir:    dir,
		taskID: taskID,
	}
}

// Start 开始一个追踪段，返回结束函数
func (r *TraceRecorder) Start(stage TraceStage, input string) func(success bool, output string, err error) {
	start := time.Now()
	r.mu.Lock()
	r.seq++
	seq := r.seq
	r.mu.Unlock()

	return func(success bool, output string, err error) {
		latency := time.Since(start).Milliseconds()
		span := TraceSpan{
			TaskID:    r.taskID,
			Stage:     stage,
			Input:     truncateStr(input, 200),
			Output:    truncateStr(output, 200),
			LatencyMs: latency,
			Success:   success,
			Error:     errStr(err),
			TS:        start.UnixMilli(),
			Seq:       seq,
		}

		r.mu.Lock()
		r.spans = append(r.spans, span)
		r.mu.Unlock()
	}
}

// Spans 获取所有追踪段
func (r *TraceRecorder) Spans() []TraceSpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]TraceSpan, len(r.spans))
	copy(result, r.spans)
	return result
}

// Save 保存追踪记录到 JSONL 文件
func (r *TraceRecorder) Save() error {
	if r.dir == "" {
		return nil
	}

	if err := os.MkdirAll(r.dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(r.dir, fmt.Sprintf("trace-%s.jsonl", r.taskID))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, span := range r.Spans() {
		data, err := json.Marshal(span)
		if err != nil {
			continue // 跳过序列化失败的 span
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

// Summary 生成追踪摘要
func (r *TraceRecorder) Summary() string {
	spans := r.Spans()
	if len(spans) == 0 {
		return "无追踪数据"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task %s: %d stages\n", r.taskID, len(spans)))

	totalLatency := int64(0)
	for _, s := range spans {
		totalLatency += s.LatencyMs
		status := "✅"
		if !s.Success {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("  %s [%s] %dms %s\n", status, s.Stage, s.LatencyMs, truncateStr(s.Output, 60)))
	}

	sb.WriteString(fmt.Sprintf("Total: %dms\n", totalLatency))
	return sb.String()
}

// SummaryJSON 生成 JSON 格式的追踪摘要
func (r *TraceRecorder) SummaryJSON() map[string]any {
	spans := r.Spans()
	totalLatency := int64(0)
	successCount := 0
	failCount := 0

	for _, s := range spans {
		totalLatency += s.LatencyMs
		if s.Success {
			successCount++
		} else {
			failCount++
		}
	}

	return map[string]any{
		"task_id":     r.taskID,
		"total_spans": len(spans),
		"total_ms":    totalLatency,
		"success":     successCount,
		"failed":      failCount,
		"spans":       spans,
	}
}

func truncateStr(s string, maxLen int) string {
	// 按 rune 计数，避免切割 UTF-8 多字节字符
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
