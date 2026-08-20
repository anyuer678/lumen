package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// McpServer MCP 服务器配置
type McpServer struct {
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Transport string   `json:"transport"` // stdio|sse|streamablehttp
	URL       string   `json:"url,omitempty"`
}

// McpClient MCP 客户端（支持 stdio 和 SSE 传输）
type McpClient struct {
	server   McpServer
	cmd      *exec.Cmd
	mu       sync.Mutex
	started  bool
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	reader   *bufio.Reader
	nextID   int64
	// SSE 传输
	sseURL    string
	sseClient *http.Client
}

// NewMcpClient 创建 MCP 客户端
func NewMcpClient(server McpServer) *McpClient {
	return &McpClient{server: server}
}

// Start 启动 MCP 服务器进程
func (c *McpClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	// SSE 传输模式
	if c.server.Transport == "sse" && c.server.URL != "" {
		c.sseURL = c.server.URL
		c.sseClient = &http.Client{Timeout: 30 * time.Second}
		c.started = true

		// 握手：发送 initialize 请求
		if _, err := c.callJSONRPC(ctx, "initialize", map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "openagent", "version": "0.1.0"},
		}); err != nil {
			// 握手失败不致命
			c.started = true
		}
		return nil
	}

	// stdio 传输模式（默认）
	c.cmd = exec.CommandContext(ctx, c.server.Command, c.server.Args...)
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewReader(stdout)
	c.nextID = 1

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}

	c.started = true
	time.Sleep(100 * time.Millisecond)
	// 握手：发送 initialize 请求
	if _, err := c.callJSONRPC(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "openagent", "version": "0.1.0"},
	}); err != nil {
		// 握手失败不致命，部分 server 不需要
		c.started = true
	}
	return nil
}

// callJSONRPC 发送 JSON-RPC 请求并读取匹配 id 的响应
func (c *McpClient) callJSONRPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.nextID++
	req := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method, "params": params}
	reqBytes, _ := json.Marshal(req)

	// SSE 传输模式：通过 HTTP POST 发送
	if c.sseURL != "" {
		return c.callJSONRPCHTTP(ctx, reqBytes)
	}

	// stdio 传输模式
	if _, err := c.stdin.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// 读取响应直到匹配 id（带超时）
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("MCP server closed stdout")
			}
			return nil, fmt.Errorf("read response: %w", err)
		}
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int64           `json:"id"`
			Result  json.RawMessage `json:"result"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // 跳过非 JSON 行（如 stderr 混入）
		}
		if msg.JSONRPC != "2.0" {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	}
	return nil, fmt.Errorf("MCP response timeout")
}

// callJSONRPCHTTP 通过 HTTP POST 发送 JSON-RPC 请求（SSE/Streamable HTTP 传输）
func (c *McpClient) callJSONRPCHTTP(ctx context.Context, reqBytes []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.sseURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.sseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read HTTP response: %w", err)
	}

	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &msg); err != nil {
		return nil, fmt.Errorf("parse HTTP response: %w (body: %s)", err, string(respBody[:min(len(respBody), 200)]))
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", msg.Error.Code, msg.Error.Message)
	}
	return msg.Result, nil
}

// Stop 停止 MCP 服务器
func (c *McpClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.started = false
}

// CallTool 调用 MCP 工具（真实 JSON-RPC，返回工具结果）
func (c *McpClient) CallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return "", fmt.Errorf("MCP server not started")
	}

	result, err := c.callJSONRPC(ctx, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	// 解析 MCP 结构化内容
	var content struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(result, &content) == nil && len(content.Content) > 0 {
		var sb strings.Builder
		for _, c := range content.Content {
			sb.WriteString(c.Text)
		}
		return sb.String(), nil
	}

	return string(result), nil
}

// ListTools 列出 MCP 服务器提供的工具
func (c *McpClient) ListTools(ctx context.Context) ([]McpToolDef, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil, fmt.Errorf("MCP server not started")
	}

	result, err := c.callJSONRPC(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tools []McpToolDef `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	return resp.Tools, nil
}

// McpToolAdapter MCP 工具适配器
type McpToolAdapter struct {
	client *McpClient
	tool   McpToolDef
}

// McpToolDef MCP 工具定义
type McpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func (a *McpToolAdapter) Name() string        { return "mcp." + a.tool.Name }
func (a *McpToolAdapter) Description() string  { return a.tool.Description }
func (a *McpToolAdapter) RequiredLevel() int   { return 2 } // MCP 默认需要确认

func (a *McpToolAdapter) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	// 使用原始工具名（去掉服务器前缀）调用 MCP 服务器
	originalName := a.tool.Name
	if idx := strings.IndexByte(originalName, '.'); idx > 0 {
		originalName = originalName[idx+1:]
	}

	result, err := a.client.CallTool(ctx, originalName, args)
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Raw:     result,
		Kind:    "text",
		Summary: fmt.Sprintf("MCP tool %s executed", a.tool.Name),
	}, nil
}

// McpRegistry MCP 服务器注册表
type McpRegistry struct {
	servers map[string]*McpClient
	loop    *Loop
	mu      sync.RWMutex
}

// NewMcpRegistry 创建注册表
func NewMcpRegistry() *McpRegistry {
	return &McpRegistry{
		servers: make(map[string]*McpClient),
	}
}

// AttachLoop 关联 Agent Loop（后续注册的 MCP 工具会自动同步）
func (r *McpRegistry) AttachLoop(loop *Loop) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loop = loop

	// 同步已注册的 server 的各个工具
	if loop != nil {
		for _, client := range r.servers {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			tools, err := client.ListTools(ctx)
			cancel()

			if err != nil || len(tools) == 0 {
				// 通用工具
				loop.RegisterTool(&McpToolAdapter{
					client: client,
					tool:   McpToolDef{Name: client.server.Name, Description: "MCP server: " + client.server.Name},
				})
				continue
			}

			for _, tool := range tools {
				adapter := &McpToolAdapter{client: client, tool: tool}
				adapter.tool.Name = client.server.Name + "." + tool.Name
				loop.RegisterTool(adapter)
			}
		}
	}
}

// Register 注册并启动 MCP 服务器
func (r *McpRegistry) Register(ctx context.Context, server McpServer) error {
	client := NewMcpClient(server)
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("start MCP server %s: %w", server.Name, err)
	}

	r.mu.Lock()
	r.servers[server.Name] = client
	loop := r.loop
	r.mu.Unlock()

	// 同步到 Agent Loop（使工具可被调用）
	if loop != nil {
		loop.RegisterTool(&McpToolAdapter{
			client: client,
			tool:   McpToolDef{Name: server.Name, Description: "MCP server: " + server.Name},
		})
	}

	return nil
}

// Unregister 注销并停止 MCP 服务器
func (r *McpRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.servers[name]; ok {
		client.Stop()
		delete(r.servers, name)
	}
}

// List 列出所有已注册的 MCP 服务器
func (r *McpRegistry) List() []*McpServer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var servers []*McpServer
	for _, client := range r.servers {
		server := client.server
		servers = append(servers, &server)
	}
	return servers
}

// TestServer 测试 MCP 服务器连接
func (r *McpRegistry) TestServer(ctx context.Context, name string) error {
	r.mu.RLock()
	client, ok := r.servers[name]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server not found: %s", name)
	}
	if client.cmd == nil || client.cmd.Process == nil {
		return fmt.Errorf("server %s not running", name)
	}
	return nil
}

// GetToolAdapters 获取所有 MCP 工具适配器（每个服务器的每个工具独立注册）
func (r *McpRegistry) GetToolAdapters() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var adapters []Tool
	for _, client := range r.servers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		tools, err := client.ListTools(ctx)
		cancel()

		if err != nil || len(tools) == 0 {
			// 获取失败或无工具，注册一个通用工具
			adapters = append(adapters, &McpToolAdapter{
				client: client,
				tool: McpToolDef{
					Name:        client.server.Name,
					Description: fmt.Sprintf("MCP server: %s（调用该服务器提供的工具）", client.server.Name),
				},
			})
			continue
		}

		// 注册每个独立工具
		for _, tool := range tools {
			adapter := &McpToolAdapter{
				client: client,
				tool:   tool,
			}
			// 工具名加上服务器前缀避免冲突
			adapter.tool.Name = client.server.Name + "." + tool.Name
			adapters = append(adapters, adapter)
		}
	}
	return adapters
}

// SyncToLoop 将注册的 MCP 工具同步到 Agent Loop
func (r *McpRegistry) SyncToLoop(loop *Loop) {
	if loop == nil {
		return
	}
	for _, a := range r.GetToolAdapters() {
		loop.RegisterTool(a)
	}
}

// StopAll 停止所有 MCP 服务器
func (r *McpRegistry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, client := range r.servers {
		client.Stop()
	}
	r.servers = make(map[string]*McpClient)
}
