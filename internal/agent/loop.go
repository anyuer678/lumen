package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"sync/atomic"

	"agent/internal/auth"
	"agent/internal/config"
	"agent/internal/contextmgr"
	"agent/internal/llm"
	"agent/internal/memory"
	"agent/internal/task"
	"agent/internal/vision"

	"go.uber.org/zap"
)

// Loop Agent 主循环
type Loop struct {
	store        *task.Store
	logger       *zap.SugaredLogger
	tools        map[string]Tool
	provider     llm.Provider
	router       *llm.Router // 智能路由器
	planner      *LLMPlanner
	evaluator    *LLMEvaluator
	replanner    *LLMReplanner
	permEngine   *auth.PermissionEngine
	confirmStore *auth.ConfirmStore
	auditLog     *sql.DB
	memoryStore  *memory.Store
	kb           *memory.KBStore
	onStep       func(eventType string, data map[string]any) // 步骤事件回调
	ctxMgr       *contextmgr.Manager                         // 上下文管理器
	eventBus     *EventBus                                   // 事件总线（主动任务恢复用）
	feedback     *FeedbackCollector                          // 反馈收集器
	traceDir     string                                      // 追踪记录目录

	// 子代理并发控制
	subagentSem     chan struct{} // 并发信号量（默认容量 4）
	subagentRunning atomic.Int32  // 当前运行中的子代理数（用于统计/日志）
}

// SetKnowledgeBase 注入知识库（供 Agent 规划检索）
func (l *Loop) SetKnowledgeBase(kb *memory.KBStore) {
	l.kb = kb
}

// retrieveKnowledge 检索与任务目标相关的知识
func (l *Loop) retrieveKnowledge(goal string) string {
	if l.kb == nil || goal == "" {
		return ""
	}
	items, err := l.kb.Search(goal, 3)
	if err != nil || len(items) == 0 {
		return ""
	}
	var parts []string
	for _, k := range items {
		parts = append(parts, fmt.Sprintf("[知识:%s] %s", k.Title, k.Content))
	}
	return stringListJoin(parts, "\n")
}

// SetStepCallback 设置步骤事件回调（用于 SSE 推送）
func (l *Loop) SetStepCallback(cb func(eventType string, data map[string]any)) {
	l.onStep = cb
}

// SetRouter 设置智能路由器
func (l *Loop) SetRouter(r *llm.Router) {
	l.router = r

	// 构建回退链：默认 → 简单 → 复杂 → 任意可用
	if r != nil && l.provider != nil {
		chain := []llm.Provider{l.provider} // 主 provider（从 config 来）
		seen := map[string]bool{l.provider.Name(): true}

		// 按路由优先级追加备用
		priorities := []string{r.Config().Default, r.Config().Simple, r.Config().Complex, r.Config().Vision}
		for _, name := range priorities {
			if name == "" || seen[name] {
				continue
			}
			if p := r.GetProvider(name); p != nil {
				chain = append(chain, p)
				seen[name] = true
			}
		}
		// 兜底：任意未使用的 provider
		for name, p := range r.GetAllProviders() {
			if !seen[name] {
				chain = append(chain, p)
				seen[name] = true
			}
		}

		if len(chain) > 1 {
			fallback := llm.NewFallbackProvider(chain...)
			l.planner = NewLLMPlanner(fallback, l.tools)
			l.evaluator = NewLLMEvaluator(fallback)
			l.replanner = NewLLMReplanner(fallback, l.tools)
			l.logger.Infof("agent loop: fallback chain built: %d providers", len(chain))
		}
	}
}

// SetVisionAnalyzer 将视觉分析器注入 ComputerTool（LLM provider 可用后调用）
func (l *Loop) SetVisionAnalyzer(a *vision.Analyzer) {
	if ct, ok := l.tools["computer"].(*ComputerTool); ok {
		ct.SetVisionAnalyzer(a)
		l.logger.Infof("vision analyzer injected into computer tool")
	}
}

// SetEventBus 注入事件总线（用于任务失败时触发主动恢复）
func (l *Loop) SetEventBus(bus *EventBus) {
	l.eventBus = bus
}

// SetFeedbackCollector 注入反馈收集器
func (l *Loop) SetFeedbackCollector(fc *FeedbackCollector) {
	l.feedback = fc
}

// SetTraceDir 设置追踪记录目录
func (l *Loop) SetTraceDir(dir string) {
	l.traceDir = dir
}

// emitStep 广播步骤事件
func (l *Loop) emitStep(eventType string, data map[string]any) {
	if l.onStep != nil {
		l.onStep(eventType, data)
	}
}

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	RequiredLevel() int
	Execute(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// ToolResult 工具执行结果
type ToolResult struct {
	Raw     string `json:"raw"`
	Kind    string `json:"kind"`
	Summary string `json:"summary,omitempty"`
}

// Plan 计划
type Plan struct {
	Steps []PlanStep `json:"steps"`
}

// PlanStep 计划步骤
type PlanStep struct {
	Description string         `json:"description"`
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"args"`
	MaxRetries  int            `json:"max_retries"`

	// destructiveApproved 内部标志：本步破坏性命令已获人工确认（不序列化）
	destructiveApproved bool `json:"-"`
}

// NewLoop 创建 Agent Loop
func NewLoop(db *sql.DB, logger *zap.Logger, provider llm.Provider) *Loop {
	l := &Loop{
		store:        task.NewStore(db),
		logger:       logger.Sugar(),
		tools:        make(map[string]Tool),
		provider:     provider,
		permEngine:   auth.NewPermissionEngine(),
		confirmStore: auth.NewConfirmStore(db),
		auditLog:     db,
		memoryStore:  memory.NewStore(db),
		subagentSem:  make(chan struct{}, 4),
		ctxMgr:       contextmgr.NewManager(8192), // 默认 8K 上下文窗口
	}
	l.registerBuiltinTools()

	// 初始化 LLM 组件（provider 为 nil 时使用模拟模式）
	if provider != nil {
		l.planner = NewLLMPlanner(provider, l.tools)
		l.evaluator = NewLLMEvaluator(provider)
		l.replanner = NewLLMReplanner(provider, l.tools)
	} else {
		l.planner = nil
		l.evaluator = nil
		l.replanner = nil
		l.logger.Warnf("LLM provider not configured, using simplified mode")
	}
	return l
}

// RegisterTool 注册工具
func (l *Loop) RegisterTool(t Tool) {
	l.tools[t.Name()] = t
}

// ToolMeta 工具元信息
type ToolMeta struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	RequiredLevel int    `json:"required_level"`
}

// ListTools 列出所有已注册工具
func (l *Loop) ListTools() []ToolMeta {
	var metas []ToolMeta
	for name, t := range l.tools {
		metas = append(metas, ToolMeta{
			Name:          name,
			Description:   t.Description(),
			RequiredLevel: t.RequiredLevel(),
		})
	}
	return metas
}

// stringListJoin 拼接字符串列表
func stringListJoin(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// registerBuiltinTools 注册内置工具
func (l *Loop) registerBuiltinTools() {
	sandbox := true // 默认启用沙箱
	workspaceRoot := "./data/workspace"
	if cfg := config.Get(); cfg != nil {
		sandbox = cfg.Workspace.Sandbox
		if cfg.Workspace.Root != "" {
			workspaceRoot = cfg.Workspace.Root
		}
	}
	// argv 白名单工具：默认推荐的命令执行面（无 shell 解释；默认仅只读诊断命令）
	l.RegisterTool(NewArgvTool(workspaceRoot, sandbox))
	// 字符串 shell：始终注册以便确认流可调用；实际是否允许由 shell_profile 决定
	// （argv-only 档 RunTool 直接拒绝；strict 档每次确认；full 需显式 opt-in）
	l.RegisterTool(&ShellTool{sandbox: sandbox, workspaceRoot: workspaceRoot})
	l.RegisterTool(NewFilesystemTool(workspaceRoot, sandbox))
	l.RegisterTool(NewFileGrepTool("./data/workspace", sandbox))
	l.RegisterTool(NewGitHubTool()) // GitHub 集成（只读）
	l.RegisterTool(NewBrowserTool("./data/browser-profile", false))
	l.RegisterTool(NewSystemTool())
	l.RegisterTool(&delegateTool{l: l}) // 子代理委派
	l.RegisterTool(&safetyTool{})       // 命令安全分类
	// Sprint3：Computer Use 默认关闭，仅显式 permissions.computer_use_enabled=true 时注册
	if config.ComputerUseEnabled() {
		l.RegisterTool(NewComputerTool("./data/workspace"))
	}
	if runtime.GOOS == "windows" {
		l.RegisterTool(NewWindowsTool())
	}
}

// DecodePlan 解码计划
func DecodePlan(data []byte) (*Plan, error) {
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}
