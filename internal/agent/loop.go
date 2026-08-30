package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"agent/internal/auth"
	"agent/internal/config"
	"agent/internal/contextmgr"
	"agent/internal/llm"
	"agent/internal/memory"
	"agent/internal/task"
	"agent/internal/vision"
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
	subagentRunning atomic.Int32 // 当前运行中的子代理数（用于统计/日志）
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
	Raw      string `json:"raw"`
	Kind     string `json:"kind"`
	Summary  string `json:"summary,omitempty"`
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

// extractFilePath 从中文描述中提取文件路径
func extractFilePath(goal string) string {
	// 方法1：找反斜杠或正斜杠分割的路径
	for i := 0; i < len(goal); i++ {
		if goal[i] == '/' || goal[i] == '\\' {
			start := i
			for start > 0 {
				c := goal[start-1]
				if c == ' ' || c == ',' || c == '.' || c > 127 {
					break
				}
				start--
			}
			end := i
			for end < len(goal) {
				c := goal[end]
				if c == ' ' || c == ',' || c == '.' || c == '\n' || c > 127 {
					break
				}
				end++
			}
			path := goal[start:end]
			if strings.Contains(path, ".") || strings.Contains(path, "/") || strings.Contains(path, "\\") {
				return strings.TrimSpace(path)
			}
		}
	}

	// 方法2：匹配常见文件名模式
	filePatterns := []string{
		`conf/config\.yaml`,
		`data/workspace/[\w\-\.]+`,
		`[\w\-]+\.(txt|md|yaml|json|go|py|js|log)`,
	}
	for _, pattern := range filePatterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindString(goal); match != "" {
			return match
		}
	}

	return ""
}

// extractCommandFromGoal 从任务目标中提取命令
func extractCommandFromGoal(goal string) string {
	lower := strings.ToLower(goal)

	// 1. 自然语言 → 命令映射（最高优先级，先匹配再提取）
	type goalPattern struct {
		keywords []string
		cmd      string
		exact    bool   // 是否需要精确匹配（包含而非部分匹配）
		extract  func(goal string) string
	}
	patterns := []goalPattern{
		// 系统信息
		{[]string{"系统时间", "当前时间", "几点"}, "Get-Date", false, nil},
		{[]string{"whoami", "当前用户", "用户名"}, "whoami", false, nil},
		{[]string{"磁盘空间", "查询 C 盘磁盘"}, "Get-PSDrive C", false, nil},
		{[]string{"hostname", "计算机名", "主机名"}, "hostname", false, nil},
		{[]string{"环境变量", "查看系统环境变量"}, "Get-ChildItem env:", false, nil},
		{[]string{"列出当前运行的进程", "进程列表"}, "Get-Process | Select-Object -First 10", false, nil},
		{[]string{"网络状态", "ipconfig", "ip 地址"}, "ipconfig", false, nil},
		{[]string{"ping "}, "ping baidu.com", false, func(g string) string {
			idx := strings.Index(strings.ToLower(g), "ping ")
			if idx >= 0 {
				after := strings.TrimSpace(g[idx+5:])
				after = strings.TrimLeft(after, "：: 的")
				if after != "" {
					return "ping " + after
				}
			}
			return "ping baidu.com"
		}},
		// 文件操作（单步）
		{[]string{"读取文件", "查看文件", "读取配置"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "type " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取文件路径"
		}},
		{[]string{"列出目录", "查看目录", "目录内容"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path == "" {
				path = "."
			}
			return "dir " + strings.ReplaceAll(path, "/", "\\")
		}},
		{[]string{"检查文件存在", "文件是否存在"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "if exist " + strings.ReplaceAll(path, "/", "\\") + " echo EXISTS"
			}
			return "echo 无法提取文件路径"
		}},
		{[]string{"统计文件数", "有多少个文件"}, "powershell -Command \"(Get-ChildItem -File).Count\"", false, nil},
		{[]string{"列出子目录", "子目录"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path == "" {
				path = "."
			}
			return "dir /ad " + strings.ReplaceAll(path, "/", "\\")
		}},
		{[]string{"创建目录", "新建目录"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "mkdir " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取目录路径"
		}},
		{[]string{"创建文件", "新建文件"}, "", false, func(g string) string {
			// 尝试提取文件路径和内容
			path := extractFilePath(g)
			content := ""
			// 手动提取"内容为"后面的内容
			for _, kw := range []string{"内容为", "内容是"} {
				idx := strings.Index(g, kw)
				if idx >= 0 {
					after := g[idx+len(kw):]
					after = strings.TrimLeft(after, "：: 的")
					endIdx := len(after)
					for i, c := range after {
						if c == '。' || c == '\n' || c == '，' || c == '；' {
							endIdx = i
							break
						}
					}
					content = strings.TrimSpace(after[:endIdx])
					break
				}
			}
			if path != "" && content != "" {
				return "echo " + content + " > " + strings.ReplaceAll(path, "/", "\\")
			} else if path != "" {
				return "echo. > " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取文件路径"
		}},
		{[]string{"删除文件", "移除文件"}, "", false, func(g string) string {
			path := extractFilePath(g)
			if path != "" {
				return "del " + strings.ReplaceAll(path, "/", "\\")
			}
			return "echo 无法提取文件路径"
		}},
	}

	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				if p.extract != nil {
					return p.extract(goal)
				}
				return p.cmd
			}
		}
	}

	// 2. 显式命令前缀（"执行命令 xxx"）
	for _, prefix := range []string{"执行命令", "运行命令", "执行", "运行"} {
		if strings.HasPrefix(goal, prefix) {
			after := strings.TrimSpace(goal[len(prefix):])
			after = strings.TrimLeft(after, "：: 的 ")
			if after != "" {
				return after
			}
		}
	}

	// 3. 提取英文命令
	if idx := strings.IndexAny(goal, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"); idx >= 0 {
		// 找到连续的英文单词
		end := idx
		for end < len(goal) && goal[end] != ' ' && goal[end] != '\n' {
			end++
		}
		cmd := goal[idx:end]
		// 过滤掉常见的非命令英文词
		nonCommands := []string{"the", "and", "for", "with", "from", "to", "http", "https", "www"}
		isCmd := true
		for _, nc := range nonCommands {
			if strings.ToLower(cmd) == nc {
				isCmd = false
				break
			}
		}
		if isCmd && len(cmd) >= 2 {
			return cmd
		}
	}

	// 4. 尝试去掉乱码前缀
	for _, prefix := range []string{"???? ", "??? ", "?? ", "? "} {
		if strings.HasPrefix(goal, prefix) {
			after := strings.TrimSpace(goal[len(prefix):])
			if after != "" {
				return after
			}
		}
	}

	// 4. 如果是纯英文命令，直接使用
	if !containsChinese(goal) {
		return goal
	}

	// 5. 无法提取，返回 echo
	return "echo " + goal
}

// containsChinese 检查字符串是否包含中文字符
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// checkToolPermission 用 PermissionEngine 策略表判定工具权限。
// 键为 tool[:action]（args.action 存在时拼上，覆盖 fs.delete/windows.launch 等
// action 分发工具）；策略未命中默认 fail-closed（L2 需确认）。
// principal 缺失（后台任务）按 Level1Normal 处理。
func (l *Loop) checkToolPermission(ctx context.Context, tool string, args map[string]any) auth.PermissionDecision {
	if l.permEngine == nil {
		return auth.PermissionDecision{Allowed: true, Level: auth.Level0ReadOnly}
	}
	permKey := tool
	if action, ok := args["action"].(string); ok && action != "" {
		permKey = tool + ":" + action
	}
	userLevel := auth.Level1Normal
	if p := auth.PrincipalFromContext(ctx); p != nil {
		userLevel = auth.PermissionLevel(p.PermLevel)
	}
	return l.permEngine.Check(strings.ReplaceAll(permKey, ".", ":"), userLevel)
}

// RunTool 运行指定工具
func (l *Loop) RunTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	tool, ok := l.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	result, err := tool.Execute(ctx, args)
	if err != nil {
		return result, err
	}
	return result, nil
}

// Run 执行任务
func (l *Loop) Run(ctx context.Context, t *task.Task) error {
	l.logger.Infof("agent loop: starting task %s", t.ID)
	startTime := time.Now()

	// 初始化追踪记录器
	trace := NewTraceRecorder(l.traceDir, t.ID)

	var plan *Plan
	var err error

	// 检索相关长期记忆 + 知识库
	memoryContext := ""
	if l.memoryStore != nil {
		if mems, memErr := l.memoryStore.Search(t.Goal, 5); memErr == nil && len(mems) > 0 {
			var parts []string
			for _, m := range mems {
				if m.Confirmed {
					parts = append(parts, fmt.Sprintf("[记忆] %s", m.Content))
				}
			}
			memoryContext = stringListJoin(parts, "\n")
			if memoryContext != "" {
				l.logger.Infof("agent loop: task %s retrieved %d memories", t.ID, len(parts))
			}
		}
	}

	// 检索知识库，并入上下文
	kbContext := l.retrieveKnowledge(t.Goal)
	if kbContext != "" {
		if memoryContext != "" {
			memoryContext += "\n" + kbContext
		} else {
			memoryContext = kbContext
		}
		l.logger.Infof("agent loop: task %s retrieved knowledge for %s", t.ID, t.Goal)
	}

	// 使用上下文管理器裁剪记忆上下文（防止注入过多内容导致 Planner 超预算）
	if memoryContext != "" {
		_, memoryContext = l.ctxMgr.FitForPlanner(t.Goal, memoryContext, nil)
	}

	// 简化模式下也把知识输出到结果
	if l.planner == nil && kbContext != "" {
		l.store.SetResult(t.ID, "已检索相关用户知识。", "")
	}

	// 根据是否有 LLM provider 决定使用哪种模式
	// 断点恢复：若任务已有 checkpoint，加载已保存的计划并从断点继续
	var resumeStep int
	if storedPlan, perr := l.store.GetPlan(t.ID); perr == nil && len(storedPlan) > 0 {
		if p, derr := DecodePlan(storedPlan); derr == nil && len(p.Steps) > 0 {
			if rs, cerr := l.store.LoadCheckpoint(t.ID); cerr == nil && rs > 0 && rs < len(p.Steps) {
				resumeStep = rs
				plan = p
				l.logger.Infof("agent loop: task %s resuming from checkpoint step %d/%d", t.ID, resumeStep, len(p.Steps))
				// 预填 progress
				t.Progress = float64(resumeStep) / float64(len(p.Steps)) * 100
				t.CurrentStep = resumeStep
			}
		}
	}

	// 若无断点，则重新生成计划
	if plan == nil && l.planner != nil {
		// 1. 用 LLM 生成计划（注入记忆）
		endTrace := trace.Start(StagePlanner, t.Goal)
		plan, err = l.planner.Plan(ctx, t.Goal, memoryContext)
		if err != nil {
			endTrace(false, "", err)
			// 降级：LLM 计划失败时，对可提取命令的目标用简化单步，避免整体失败
			if cmd := extractCommandFromGoal(t.Goal); cmd != "" && cmd != t.Goal {
				l.logger.Warnf("agent loop: task %s LLM plan failed (%v), falling back to single shell step", t.ID, err)
				plan = &Plan{Steps: []PlanStep{{
					Description: "执行任务: " + t.Goal,
					Tool:        "shell.run",
					Args:        map[string]any{"command": cmd, "timeout": 30},
					MaxRetries:  2,
				}}}
			} else {
				l.emitStep("task.failed", map[string]any{"task_id": t.ID, "error": "plan: " + err.Error()})
				// 通过 EventBus 发射任务失败事件（触发主动恢复）
				if l.eventBus != nil {
					l.eventBus.EmitAsync(Event{
						Source:   "agent-loop",
						Type:     EventTaskFailed,
						Payload:  t.ID,
						Priority: 5,
					})
				}
				return fmt.Errorf("plan: %w", err)
			}
		} else {
			endTrace(true, fmt.Sprintf("%d steps", len(plan.Steps)), nil)
		}
	} else if plan == nil {
		// 简化模式：解析目标并直接执行
		l.logger.Warnf("agent loop: using simplified mode (no LLM)")
		cmd := extractCommandFromGoal(t.Goal)
		l.logger.Infof("agent loop: simplified command=%q", cmd)
		plan = &Plan{
			Steps: []PlanStep{
				{
					Description: "执行任务: " + t.Goal,
					Tool:        "shell.run",
					Args: map[string]any{
						"command": cmd,
						"timeout": 30,
					},
					MaxRetries: 2,
				},
			},
		}
	}

	if err := l.store.SavePlan(t.ID, plan); err != nil {
		l.logger.Warnf("failed to save plan: %v", err)
	}

	// 防止 0 步骤任务
	if len(plan.Steps) == 0 {
		l.store.SetResult(t.ID, "No steps generated", "")
		return fmt.Errorf("no steps generated for goal: %s", t.Goal)
	}

	l.logger.Infof("agent loop: task %s plan has %d steps", t.ID, len(plan.Steps))

	// 2. 执行步骤（支持从断点 resumeStep 继续）
	for i := resumeStep; i < len(plan.Steps); i++ {
		select {
		case <-ctx.Done():
			l.logger.Infof("agent loop: task %s cancelled", t.ID)
			return ctx.Err()
		default:
		}

		step := plan.Steps[i]
		totalSteps := len(plan.Steps)
		l.logger.Infof("agent loop: task %s step %d/%d - %s", t.ID, i+1, totalSteps, step.Description)

		progress := float64(i) / float64(totalSteps) * 100
		l.store.UpdateProgress(t.ID, progress, i)

		// 广播步骤开始
		l.emitStep("step.started", map[string]any{
			"task_id": t.ID, "step": i + 1, "total": totalSteps,
			"description": step.Description, "tool": step.Tool,
		})

		// 权限检查：PermissionEngine 策略表判定（键 = tool[:action]）。
		// 用户默认 Level1Normal（任务经 task manager 后台运行，principal 不入库）；
		// 策略等级 >= L2 且用户不足时进入确认流，直接拒绝则失败该步。
		if l.permEngine != nil {
			decision := l.checkToolPermission(ctx, step.Tool, step.Args)
			if !decision.Allowed && !decision.NeedConfirm {
				l.logger.Warnf("agent loop: task %s step %d denied by policy: %s", t.ID, i+1, decision.Reason)
				l.store.SetResult(t.ID, "", decision.Reason)
				return fmt.Errorf("step %d denied: %s", i+1, decision.Reason)
			}
			if decision.NeedConfirm {
				l.logger.Infof("agent loop: task %s step %d requires confirmation for %s (level=%d)", t.ID, i+1, step.Tool, decision.Level)
				approved, err := l.waitForConfirmation(ctx, t, &step, i, decision)
				if err != nil {
					l.logger.Warnf("confirmation error: %v, skipping step", err)
					continue
				}
				if !approved {
					l.logger.Warnf("agent loop: task %s step %d denied by user", t.ID, i+1)
					l.store.SetResult(t.ID, "", "Step denied by user confirmation")
					return fmt.Errorf("step %d denied by user", i+1)
				}
			}
		}

		// 破坏性命令检测：shell.run 的命令若被 ClassifyCommand 判定为破坏性，
		// 构造 NeedConfirm 决策走确认流（shell.go 的硬拒绝保留为最后防线）
		if step.Tool == "shell.run" {
			if cmd, ok := step.Args["command"].(string); ok {
				if ClassifyCommand(cmd) == CommandDestructive {
					l.logger.Infof("agent loop: task %s step %d shell.run destructive, requiring confirmation", t.ID, i+1)
					confirmDecision := auth.PermissionDecision{
						Allowed:    false,
						NeedConfirm: true,
						Level:      auth.Level2Dangerous,
						Reason:     fmt.Sprintf("破坏性命令需要确认（分类：%s）", CommandClassLabel(CommandDestructive)),
					}
					approved, err := l.waitForConfirmation(ctx, t, &step, i, confirmDecision)
					if err != nil {
						l.logger.Warnf("destructive confirmation error: %v, skipping step", err)
						continue
					}
					if !approved {
						l.logger.Warnf("agent loop: task %s step %d destructive command denied by user", t.ID, i+1)
						l.store.SetResult(t.ID, "", "破坏性命令被用户拒绝")
						return fmt.Errorf("step %d denied: destructive command rejected by user", i+1)
					}
				}
			}
		}

		// 执行步骤（含重试）
		var lastErr error
		var lastResult *ToolResult
		maxRetries := step.MaxRetries
		if maxRetries == 0 {
			maxRetries = 2
		}

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				l.logger.Infof("agent loop: task %s step %d retry %d", t.ID, i+1, attempt-1)
				time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
			}

			res, err := l.executeStep(ctx, t, &step, i)
			if err == nil {
				lastErr = nil
				lastResult = res
				break
			}
			lastErr = err
		}

		if lastErr != nil {
			l.logger.Errorf("agent loop: task %s step %d failed: %v", t.ID, i+1, lastErr)
			l.store.SetResult(t.ID, "", lastErr.Error())
			l.emitStep("step.failed", map[string]any{"task_id": t.ID, "step": i + 1, "error": lastErr.Error()})

			// 通过 EventBus 发射任务失败事件（触发主动恢复）
			if l.eventBus != nil {
				l.eventBus.EmitAsync(Event{
					Source:   "agent-loop",
					Type:     EventTaskFailed,
					Payload:  t.ID,
					Priority: 5,
				})
			}

			// 尝试 Replanner（替换剩余步骤，保留当前步骤位置）
			if i+1 < totalSteps {
				l.logger.Infof("agent loop: task %s trying replanner", t.ID)
				remaining := plan.Steps[i+1:]
				newPlan, replanErr := l.replanner.Replan(ctx, t.Goal, step, lastErr, remaining, lastErr.Error())
				if replanErr == nil && len(newPlan.Steps) > 0 {
					plan.Steps = append(plan.Steps[:i+1], newPlan.Steps...)
					l.logger.Infof("agent loop: task %s replanned, now %d steps", t.ID, len(plan.Steps))
					continue // 重新执行当前 i 位置
				}
			}
			// 收集失败反馈
			t.Status = task.StatusFailed
			t.Error = lastErr.Error()
			if l.feedback != nil {
				l.feedback.CollectFromTask(t, time.Since(startTime).Seconds(), i+1)
			}
			// 保存追踪记录
			trace.Save()
			return lastErr
		}

		// 广播步骤完成（带工具结果摘要，供前端实时展示）
		stepEvent := map[string]any{
			"task_id": t.ID, "step": i + 1, "total": totalSteps, "tool": step.Tool,
		}
		if lastResult != nil {
			if lastResult.Summary != "" {
				stepEvent["summary"] = lastResult.Summary
			}
			if lastResult.Raw != "" {
				stepEvent["result"] = truncateStepResult(lastResult.Raw)
			}
		}
		l.emitStep("step.completed", stepEvent)

		// 保存 Checkpoint（每步执行完后）
		l.store.SaveCheckpoint(t.ID, i)
	}

	l.store.UpdateProgress(t.ID, 100, len(plan.Steps))
	l.store.SetResult(t.ID, fmt.Sprintf("Completed %d steps", len(plan.Steps)), "")
	// 修正任务状态为 completed（SetResult 不更新 status）
	l.store.SetStatus(t.ID, task.StatusCompleted)
	l.emitStep("task.completed", map[string]any{"task_id": t.ID, "steps": len(plan.Steps)})
	l.logger.Infof("agent loop: task %s completed", t.ID)

	// 保存追踪记录
	trace.Save()

	// 收集反馈（成功率/工具/耗时等统计）
	if l.feedback != nil {
		l.feedback.CollectFromTask(t, time.Since(startTime).Seconds(), len(plan.Steps))
	}

	// 独立目标达成复核（bounded reviewer）：不污染主会话上下文

	// 任务完成，沉淀记忆（确认标记=0，等待用户确认）
	if l.memoryStore != nil && t.Goal != "" {
		mem := &memory.Memory{
			ID:         fmt.Sprintf("m-%s", t.ID),
			Type:       memory.MemoryLongTerm,
			Content:    fmt.Sprintf("任务执行成功: %s（已完成 %d 步）", t.Goal, len(plan.Steps)),
			Tags:       "task,completed",
			SourceTask: t.ID,
			Confirmed:  false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := l.memoryStore.Save(mem); err != nil {
			l.logger.Warnf("failed to save memory: %v", err)
		}
	}

	// 独立目标达成复核（bounded reviewer）：不污染主会话上下文
	l.reviewGoal(ctx, t, plan)

	return nil
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

// reviewGoal 用一次独立的有界 LLM 调用复核任务目标是否达成，不污染主会话上下文。
// 仅当 LLM 可用且 provider 支持 BoundedChat 时执行；失败不影响任务结果。
func (l *Loop) reviewGoal(ctx context.Context, t *task.Task, plan *Plan) {
	if l.planner == nil || l.provider == nil {
		return
	}
	op, ok := l.provider.(*llm.OpenAIProvider)
	if !ok {
		return
	}
	var stepsBrief []string
	for _, s := range plan.Steps {
		stepsBrief = append(stepsBrief, fmt.Sprintf("- %s (tool=%s)", s.Description, s.Tool))
	}
	system := "你是一个严格且简洁的目标达成审核员。只回答：达成情况(达成/部分达成/未达成) + 一句话理由。不要多余内容。"
	user := fmt.Sprintf("任务目标: %s\n\n执行步骤:\n%s\n\n请判定该目标是否达成。", t.Goal, strings.Join(stepsBrief, "\n"))
	verdict, err := op.BoundedChat(ctx, system, user, 128, 30)
	if err != nil {
		l.logger.Warnf("goal review failed for task %s: %v", t.ID, err)
		return
	}
	l.logger.Infof("agent loop: task %s goal review → %s", t.ID, strings.TrimSpace(verdict))
	l.emitStep("task.review", map[string]any{"task_id": t.ID, "verdict": strings.TrimSpace(verdict)})
}

// waitForConfirmation 等待人工确认
func (l *Loop) waitForConfirmation(ctx context.Context, t *task.Task, step *PlanStep, seq int, decision auth.PermissionDecision) (bool, error) {
	conf := &auth.Confirmation{
		ID:          fmt.Sprintf("c-%s-%d", t.ID, seq),
		TaskID:      t.ID,
		StepSeq:     seq,
		Operation:   step.Tool,
		Tool:        step.Tool,
		RiskLevel:   decision.Level,
		Reason:      decision.Reason,
		Status:      "pending",
		Requester:   "agent",
		CreatedAt:   time.Now(),
		TimeoutSecs: 60,
	}

	argsJSON, _ := json.Marshal(step.Args)
	conf.ArgsJSON = string(argsJSON)

	if err := l.confirmStore.Create(conf); err != nil {
		return false, fmt.Errorf("create confirmation: %w", err)
	}

	// 轮询等待确认结果
	timeout := time.Duration(conf.TimeoutSecs) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(1 * time.Second):
		}

		// 检查确认状态
		updated, err := l.confirmStore.Get(conf.ID)
		if err != nil {
			continue
		}
		if updated == nil {
			continue
		}

		switch updated.Status {
		case "approved":
			return true, nil
		case "rejected", "timeout":
			return false, fmt.Errorf("confirmation %s", updated.Status)
		}
	}

	// 超时，标记超时
	l.confirmStore.Reject(conf.ID, "system", "timeout")
	return false, fmt.Errorf("confirmation timeout")
}

// executeStep 执行单个步骤，返回工具结果
func (l *Loop) executeStep(ctx context.Context, t *task.Task, step *PlanStep, seq int) (*ToolResult, error) {
	tool, ok := l.tools[step.Tool]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", step.Tool)
	}

	// 审计日志：工具调用
	l.auditLogRecord(t.ID, "tool.call", fmt.Sprintf("step=%d tool=%s", seq, step.Tool), "")

	// 记录执行的参数
	l.logger.Infof("executeStep: tool=%s args=%v", step.Tool, step.Args)

	result, err := tool.Execute(ctx, step.Args)
	if err != nil {
		// 审计日志：工具失败
		l.auditLogRecord(t.ID, "tool.failed", fmt.Sprintf("step=%d tool=%s error=%v", seq, step.Tool, err), "error")
		return result, fmt.Errorf("tool %s execute: %w", step.Tool, err)
	}

	// 审计日志：工具成功
	l.auditLogRecord(t.ID, "tool.success", fmt.Sprintf("step=%d tool=%s", seq, step.Tool), "ok")

	now := time.Now()
	stepRecord := &task.Step{
		ID:          fmt.Sprintf("s-%s-%d", t.ID, seq),
		TaskID:      t.ID,
		Seq:         seq,
		Description: step.Description,
		Status:      "completed",
		Tool:        step.Tool,
		Result:      result.Raw,
		Summary:     result.Summary,
		StartedAt:   &now,
		FinishedAt:  &now,
	}

	if err := l.store.SaveStep(stepRecord); err != nil {
		l.logger.Warnf("failed to save step: %v", err)
	}
	return result, nil
}

// truncateStepResult 截断步骤结果（供 SSE 事件推送，避免大输出撑爆事件流）
func truncateStepResult(s string) string {
	if len(s) <= 500 {
		return s
	}
	return s[:500] + "\n...（结果过长已截断）"
}

// auditLogRecord 记录审计日志（带错误处理）
func (l *Loop) auditLogRecord(target, action, detail, result string) {
	if l.auditLog == nil {
		return
	}
	_, err := l.auditLog.Exec(
		`INSERT INTO audit_logs (ts, actor, action, target, detail, result) VALUES (?, ?, ?, ?, ?, ?)`,
		time.Now(), "agent", action, target, detail, result,
	)
	if err != nil {
		l.logger.Warnf("failed to write audit log: %v", err)
	}
}

// registerBuiltinTools 注册内置工具
func (l *Loop) registerBuiltinTools() {
	sandbox := true // 默认启用沙箱
	if cfg := config.Get(); cfg != nil {
		sandbox = cfg.Workspace.Sandbox
	}
	l.RegisterTool(&ShellTool{sandbox: sandbox})
	l.RegisterTool(NewFilesystemTool("./data/workspace", sandbox))
	l.RegisterTool(NewFileGrepTool("./data/workspace", sandbox))
	l.RegisterTool(NewBrowserTool("./data/browser-profile", false))
	l.RegisterTool(NewSystemTool())
	l.RegisterTool(&delegateTool{l: l}) // 子代理委派
	l.RegisterTool(&safetyTool{})        // 命令安全分类
	l.RegisterTool(NewComputerTool("./data/workspace")) // Computer Use
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
