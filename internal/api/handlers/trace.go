package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"agent/internal/agent"
)

// TraceHandler 追踪 API
type TraceHandler struct {
	logger *zap.SugaredLogger
}

// NewTraceHandler 创建处理器
func NewTraceHandler(logger *zap.Logger) *TraceHandler {
	return &TraceHandler{logger: logger.Sugar()}
}

// Routes 注册路由
func (h *TraceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{taskID}", h.Get)
	return r
}

// Get 获取任务的追踪记录
func (h *TraceHandler) Get(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// 沙箱检查：防止路径遍历
	if strings.Contains(taskID, "..") || strings.ContainsAny(taskID, "/\\") {
		http.Error(w, "invalid task_id", http.StatusBadRequest)
		return
	}

	// 从 trace 目录读取 JSONL 文件
	tracePath := filepath.Join("./data/workspace/trajectories", taskID+".jsonl")
	data, err := os.ReadFile(tracePath)
	if err != nil {
		// 文件不存在时返回空
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"task_id": taskID, "spans": []agent.TraceSpan{}})
		return
	}

	// 解析 JSONL
	var spans []agent.TraceSpan
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var span agent.TraceSpan
		if err := json.Unmarshal([]byte(line), &span); err == nil {
			spans = append(spans, span)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"task_id": taskID,
		"spans":   spans,
		"count":   len(spans),
	})
}
