package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"agent/internal/agent"
)

// ToolHandler 工具处理器
type ToolHandler struct {
	loop *agent.Loop
}

// NewToolHandler 创建处理器
func NewToolHandler(loop *agent.Loop) *ToolHandler {
	return &ToolHandler{loop: loop}
}

// Routes 注册路由
func (h *ToolHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListTools)
	r.Post("/{name}/run", h.RunTool)
	return r
}

func (h *ToolHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	tools := h.loop.ListTools()
	if tools == nil {
		tools = []agent.ToolMeta{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tools)
}

type runToolRequest struct {
	Args map[string]any `json:"args"`
}

func (h *ToolHandler) RunTool(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req runToolRequest
	json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := h.loop.RunTool(ctx, name, req.Args)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		raw := ""
		if result != nil {
			raw = result.Raw
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
			"raw":     raw,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"raw":     result.Raw,
		"summary": result.Summary,
		"kind":    result.Kind,
	})
}
