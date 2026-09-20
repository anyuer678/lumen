package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent/internal/agent"
	"agent/internal/auth"
	"github.com/go-chi/chi/v5"
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

// actionRequiredLevel 工具元信息 RequiredLevel 可能是「工具级下限」，
// 对单工具多 action（fs）必须按 action 再收紧。
func actionRequiredLevel(meta agent.ToolMeta, args map[string]any) int {
	lvl := meta.RequiredLevel
	if meta.Name == "fs" {
		if action, ok := args["action"].(string); ok {
			switch strings.ToLower(action) {
			case "delete", "organize":
				if lvl < 2 {
					lvl = 2
				}
			case "write", "mkdir":
				if lvl < 1 {
					lvl = 1
				}
			case "read", "list", "exists":
				// 保持工具级下限
			}
		}
	}
	if meta.Name == "shell.run" && lvl < 2 {
		lvl = 2
	}
	if meta.Name == "computer" && lvl < 2 {
		lvl = 2
	}
	if meta.Name == "mcp" && lvl < 2 {
		lvl = 2
	}
	return lvl
}

func (h *ToolHandler) RunTool(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req runToolRequest
	json.NewDecoder(r.Body).Decode(&req)

	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	// Scope 必须显式授予（空 scopes 默认 fail-closed，见 auth.HasScope）
	if !p.HasScope("tools:run") {
		http.Error(w, `{"error":"scope \"tools:run\" required but not granted"}`, http.StatusForbidden)
		return
	}

	meta, ok := findToolMeta(h.loop.ListTools(), name)
	need := 0
	if ok {
		need = actionRequiredLevel(meta, req.Args)
	}
	if need > p.PermLevel {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("权限不足：工具 %s 需要 L%d，你的 token 级别是 L%d", name, need, p.PermLevel),
		})
		return
	}

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

func findToolMeta(metas []agent.ToolMeta, name string) (agent.ToolMeta, bool) {
	for _, m := range metas {
		if m.Name == name {
			return m, true
		}
	}
	return agent.ToolMeta{}, false
}
