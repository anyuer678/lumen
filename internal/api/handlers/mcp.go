package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"agent/internal/agent"
)

// McpHandler MCP 服务器管理处理器
type McpHandler struct {
	registry *agent.McpRegistry
}

// NewMcpHandler 创建处理器
func NewMcpHandler(registry *agent.McpRegistry) *McpHandler {
	return &McpHandler{registry: registry}
}

// Routes 注册路由
func (h *McpHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Register)
	r.Post("/{name}/test", h.Test)
	r.Delete("/{name}", h.Unregister)
	return r
}

func (h *McpHandler) List(w http.ResponseWriter, r *http.Request) {
	servers := h.registry.List()
	if servers == nil {
		servers = []*agent.McpServer{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (h *McpHandler) Register(w http.ResponseWriter, r *http.Request) {
	var server agent.McpServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if server.Name == "" || server.Command == "" {
		http.Error(w, "name and command are required", http.StatusBadRequest)
		return
	}
	if server.Transport == "" {
		server.Transport = "stdio"
	}
	// 安全校验：只允许白名单内的启动命令，防止 RCE
	if err := validateMcpCommand(server.Command); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.registry.Register(context.Background(), server); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(server)
}

// validateMcpCommand 限制 MCP server 只能用常见的解释器/运行时启动，
// 防止通过 HTTP 注册恶意 command 实现 RCE。
func validateMcpCommand(command string) error {
	allowed := map[string]bool{
		"node": true, "python": true, "python3": true, "npx": true,
		"uvx": true, "uv": true, "bun": true, "deno": true,
		"ruby": true, "go": true, "cargo": true, "docker": true,
	}
	base := filepath.Base(command)
	if !allowed[base] {
		return fmt.Errorf("MCP 启动命令 %q 不在白名单内（仅允许 node/python/npx/uvx/bun/deno/go 等）", base)
	}
	return nil
}

func (h *McpHandler) Test(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.registry.TestServer(context.Background(), name); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "error", "message": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "message": "connection successful"})
}

func (h *McpHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	h.registry.Unregister(name)
	w.WriteHeader(http.StatusOK)
}
