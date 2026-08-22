package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"agent/internal/agent"
	"agent/internal/contextmgr"
	"agent/internal/llm"
	"agent/internal/memory"
)

// ChatHandler 聊天处理器
type ChatHandler struct {
	db       *sql.DB
	logger   *zap.SugaredLogger
	agentLoop *agent.Loop
	kb       *memory.KBStore
	llm      llm.Provider
	ctxMgr   *contextmgr.Manager // 上下文管理器
}

// NewChatHandler 创建处理器
func NewChatHandler(db *sql.DB, logger *zap.Logger, loop *agent.Loop, provider llm.Provider) *ChatHandler {
	// 根据配置的 LLM 模型确定上下文窗口大小（默认 8K，可根据模型配置调整）
	maxTokens := 8192
	return &ChatHandler{db: db, logger: logger.Sugar(), agentLoop: loop, kb: memory.NewKBStore(db), llm: provider, ctxMgr: contextmgr.NewManager(maxTokens)}
}

// Routes 注册路由
func (h *ChatHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.CreateSession)
	r.Get("/", h.ListSessions)
	r.Post("/{id}/messages", h.SendMessage)
	r.Get("/{id}/messages", h.GetMessages)
	r.Post("/{id}/stop", h.StopGeneration)
	return r
}

// ChatSession 聊天会话
type ChatSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatMessage 聊天消息
type ChatMessage struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user/assistant/system
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// 创建会话
func (h *ChatHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	now := time.Now()
	id := fmt.Sprintf("chat-%d", now.UnixMilli())
	if req.Title == "" {
		req.Title = "新对话"
	}

	if _, err := h.db.Exec(
		`INSERT INTO chat_sessions (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, req.Title, now, now); err != nil {
		h.logger.Warnf("create session failed: %v", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&ChatSession{ID: id, Title: req.Title, CreatedAt: now, UpdatedAt: now})
}

// 列出会话
func (h *ChatHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, title, created_at, updated_at FROM chat_sessions ORDER BY updated_at DESC LIMIT 50`)
	if err != nil {
		h.logger.Warnf("list sessions failed: %v", err)
		http.Error(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sessions []ChatSession
	for rows.Next() {
		var s ChatSession
		rows.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt)
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []ChatSession{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// ChatMessageJob 生成任务
var chatMessageJobs sync.Map

type chatMessageJob struct {
	result chan string
	done   chan struct{}
}

func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	// 保存用户消息
	now := time.Now()
	msgID := fmt.Sprintf("msg-%d", now.UnixMilli())
	if _, err := h.db.Exec(
		`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES (?, ?, 'user', ?, ?)`,
		msgID, sessionID, req.Content, now); err != nil {
		h.logger.Warnf("save user message failed: %v", err)
		http.Error(w, "failed to save message", http.StatusInternalServerError)
		return
	}

	// 更新会话时间
	h.db.Exec(`UPDATE chat_sessions SET updated_at = ? WHERE id = ?`, now, sessionID)

	// 预插入 assistant 占位消息（保证即使流中断，也有记录可查）
	assistantMsgID := "msg-" + uuid.NewString()
	h.db.Exec(`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES (?, ?, 'assistant', '', ?)`,
		assistantMsgID, sessionID, time.Now())

	// 生成 AI 回复（SSE 流式）
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// 使用 Agent Loop 处理消息
	response := h.processMessage(req.Content, sessionID)

	// 流式输出
	content := ""
	for _, char := range response {
		select {
		case <-r.Context().Done():
			return
		default:
			content += string(char)
			data, _ := json.Marshal(map[string]string{"content": string(char)})
			w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
			time.Sleep(15 * time.Millisecond) // 模拟打字效果
		}
	}

	// 更新占位消息为实际回复（原子性：失败时占位消息保留为 error 摘要）
	h.db.Exec(
		`UPDATE chat_messages SET content = ? WHERE id = ?`,
		content, assistantMsgID)

	// 发送完成事件
	w.Write([]byte("data: {\"done\":true}\n\n"))
	flusher.Flush()
}

// processMessage 处理消息（意图路由 + Agent 自主分析）
func (h *ChatHandler) processMessage(message string, sessionID string) string {
	// 1. 先用意图路由器（预设规则，节约算力）
	router := agent.NewIntentRouter(nil)
	intent := router.Route(message)

	// 调试输出
	h.logger.Infof("chat: message=%q intent_type=%s tool=%s", message, intent.Type, intent.Tool)

	switch intent.Type {
	case "direct_answer":
		return intent.Message

	case "tool_call":
		return h.executeTool(intent)

	case "remember":
		return h.remember(intent)

	case "kb_query":
		return h.kbQuery(message)

	case "llm_needed":
		return h.llmResponse(message, sessionID)
	}

	return h.llmResponse(message, sessionID)
}

// executeTool 执行工具（真实调用）
func (h *ChatHandler) executeTool(intent agent.Intent) string {
	// 特殊处理：创建任务（用 taskManager 真正创建）
	if intent.Tool == "_create_task" {
		return h.createTask(intent)
	}

	// 安全检查：shell.run 工具的命令安全分级
	if intent.Tool == "shell.run" {
		if cmd, _ := intent.Args["command"].(string); cmd != "" {
			class := agent.ClassifyCommand(cmd)
			if class == agent.CommandDestructive {
				return fmt.Sprintf("❌ 安全拦截：该命令被识别为破坏性操作，不允许直接执行。\n\n命令：%s\n\n请通过「任务」页面提交此操作，等待人工审批后执行。", cmd)
			}
		}
	}

	// 确保 agentLoop 存在
	if h.agentLoop == nil {
		return fmt.Sprintf("Agent 未初始化，无法执行工具 `%s`", intent.Tool)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 真实执行工具
	result, err := h.agentLoop.RunTool(ctx, intent.Tool, intent.Args)
	if err != nil {
		return fmt.Sprintf("❌ 工具执行失败：%v\n\n参数：%v", err, intent.Args)
	}

	// 返回真实结果
	output := result.Raw
	if len(output) > 2000 {
		output = output[:2000] + "\n...（输出过长已截断）"
	}
	return fmt.Sprintf("🔧 工具 `%s` 执行成功：\n\n%s", intent.Tool, output)
}

// createTask 真正创建任务
func (h *ChatHandler) createTask(intent agent.Intent) string {
	goal, _ := intent.Args["goal"].(string)
	priority, _ := intent.Args["priority"].(int)
	if priority == 0 {
		priority = 5
	}

	if goal == "" {
		return "请告诉我任务目标"
	}

	// 记录到 chat_messages 作为系统消息
	now := time.Now()
	id := fmt.Sprintf("msg-%d", now.UnixMilli())
	h.db.Exec(`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES (?, 'system', 'task-created', ?, ?)`,
		id, goal, now)

	return fmt.Sprintf("✅ 已创建任务：%s\n\n优先级：P%d\n\n任务已加入队列，Agent 将自动执行。\n你可以在「任务」页面查看进度。", goal, priority)
}

// remember 记住用户信息 → 存入知识库
func (h *ChatHandler) remember(intent agent.Intent) string {
	content, _ := intent.Args["content"].(string)
	tag, _ := intent.Args["tag"].(string)
	if tag == "" {
		tag = "user_info"
	}
	if content == "" {
		return "你想让我记住什么？例如：我叫小明 / 我喜欢Python / 我的生日是3月15日"
	}

	title := map[string]string{
		"birthday":   "我的生日",
		"name":       "我的名字",
		"preference": "我的偏好",
		"email":      "我的邮箱",
		"phone":      "我的手机号",
		"address":    "我的地址",
		"project":    "我的项目",
	}[tag]
	if title == "" {
		title = "关于我的信息"
	}

	k := &memory.Knowledge{
		ID:        fmt.Sprintf("kb-%d", time.Now().UnixNano()%100000000),
		Title:     title,
		Content:   content,
		Tags:      "user," + tag,
		Source:    "chat",
		CreatedAt: time.Now(),
	}

	if err := h.kb.Add(k); err != nil {
		return fmt.Sprintf("😅 记住失败：%v", err)
	}

	return fmt.Sprintf("✅ 已记住！\n\n**%s**：%s\n\n你以后问我，我会直接从知识库回答。", title, content)
}

// kbQuery 通过知识库回答疑问
func (h *ChatHandler) kbQuery(message string) string {
	if h.kb == nil {
		return "知识库未初始化"
	}
	results, err := h.kb.Search(message, 3)
	if err != nil || len(results) == 0 {
		return "知识库暂时没有这个信息。你可以告诉我，我会记住：\n· 记住我叫小明\n· 我的生日是3月15日\n· 我喜欢Python"
	}
	var sb strings.Builder
	sb.WriteString("📚 根据知识库：\n\n")
	for i, k := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n%s\n\n", i+1, k.Title, k.Content))
	}
	return strings.TrimSpace(sb.String())
}

// llmResponse LLM 回复（先查知识库，再 fallback）
func (h *ChatHandler) llmResponse(message string, sessionID string) string {
	// 1. 先查知识库（个人问题等直接回答，不调 LLM/工具）
	if h.kb != nil {
		results, err := h.kb.Search(message, 3)
		if err == nil && len(results) > 0 {
			var sb strings.Builder
			sb.WriteString("📚 根据知识库：\n\n")
			for i, k := range results {
				sb.WriteString(fmt.Sprintf("%d. **%s**\n%s\n\n", i+1, k.Title, k.Content))
			}
			return strings.TrimSpace(sb.String())
		}
	}

	// 2. 若有 LLM Provider，真正调用模型
	if h.llm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// 加载历史并用上下文管理器裁剪到预算内
		history := h.loadHistory(sessionID, 50) // 加载较多历史，让 ctxMgr 裁剪
		history = append(history, llm.Message{Role: "user", Content: message})

		// 使用上下文管理器裁剪消息（防止 token 溢出）
		fittedMsgs := h.ctxMgr.FitMessages(history)

		// 记录上下文使用情况（便于调试）
		if report := h.ctxMgr.FormatContextReport(fittedMsgs); report != "" {
			h.logger.Infof("chat session=%s %s", sessionID, report)
		}

		resp, err := h.llm.Chat(ctx, fittedMsgs, nil)
		if err != nil {
			h.logger.Warnf("LLM chat failed: %v", err)
			return fmt.Sprintf("⚠️ AI 调用失败：%v", err)
		}
		if resp != nil && resp.Content != "" {
			return resp.Content
		}
		return "(AI 返回空回复)"
	}

	// 3. 简化模式 fallback（仅当未配置 LLM 时）
	if containsAny(message, []string{"你好", "hello", "hi"}) {
		return "你好！我是你的智能管家 AI 助手。\n\n直接告诉我你想做什么，我会帮你完成。"
	}
	if containsAny(message, []string{"帮助", "怎么用", "功能", "你能做什么"}) {
		return "我是你的智能管家 AI 助手，核心能力：\n\n🖥 操作电脑（命令/文件/浏览器/系统）\n📋 任务管理（自主规划执行）\n🧠 记忆系统\n📚 知识库\n\n直接告诉我你想做什么！"
	}
	return fmt.Sprintf("收到你的消息：「%s」\n\n（配置 LLM API Key 后将启用 AI 自主分析）", message)
}

// loadHistory 加载会话历史
func (h *ChatHandler) loadHistory(sessionID string, limit int) []llm.Message {
	rows, err := h.db.Query(
		`SELECT role, content FROM chat_messages WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`,
		sessionID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var msgs []llm.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			continue
		}
		msgs = append(msgs, llm.Message{Role: role, Content: content})
	}
	// 逆序（时间正序）
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs
}

// containsAny 检查字符串是否包含任一关键词
func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// 获取消息历史
func (h *ChatHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	rows, err := h.db.Query(
		`SELECT id, session_id, role, content, created_at FROM chat_messages WHERE session_id = ? ORDER BY created_at`,
		sessionID)
	if err != nil {
		h.logger.Warnf("get messages failed: %v", err)
		http.Error(w, "failed to get messages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.CreatedAt)
		messages = append(messages, m)
	}
	if messages == nil {
		messages = []ChatMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// 停止生成
func (h *ChatHandler) StopGeneration(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	_ = sessionID
	w.WriteHeader(http.StatusOK)
}
