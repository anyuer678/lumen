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
// 委托 agent.EffectiveRequiredLevel，避免 handler 与 RunTool 两套逻辑漂移。
func actionRequiredLevel(meta agent.ToolMeta, args map[string]any) int {
	// 构造仅带 Name/RequiredLevel 的轻量 Tool 适配，复用统一计算
	lvl := agent.EffectiveRequiredLevel(metaTool{meta}, meta.Name, args)
	return lvl
}

// metaTool 把 ToolMeta 适配为 EffectiveRequiredLevel 所需的最小接口。
type metaTool struct{ m agent.ToolMeta }

func (t metaTool) Name() string                   { return t.m.Name }
func (t metaTool) Description() string            { return "" }
func (t metaTool) RequiredLevel() int             { return t.m.RequiredLevel }
func (t metaTool) Execute(context.Context, map[string]any) (*agent.ToolResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// fsActionLeveler 供 EffectiveRequiredLevel 识别 fs action 下限。
// （FilesystemTool 本体在 agent 包；handler 侧用相同映射。）
func (t metaTool) ActionRequiredLevel(action string) int {
	if t.m.Name != "fs" {
		return t.m.RequiredLevel
	}
	switch strings.ToLower(action) {
	case "delete", "organize", "write", "mkdir":
		return 2
	case "read", "list", "exists":
		return 0
	default:
		return 2
	}
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
