package service

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kardianos/service"
	"go.uber.org/zap"

	"agent/internal/api"
	"agent/internal/agent"
	"agent/internal/config"
	"agent/internal/db"
	"agent/internal/llm"
	"agent/internal/memory"
	"agent/internal/observability"
	"agent/internal/scheduler"
	"agent/internal/task"
	"agent/internal/vision"
)

var logger *zap.Logger

// trajRecorders 全局轨迹记录管理器（按 task 缓存 recorder）
var trajRecorders *trajManager

// eventPublisherAdapter 适配 SSE 广播器
type eventPublisherAdapter struct {
	broadcaster *api.EventBroadcaster
}

func (a *eventPublisherAdapter) Publish(eventType string, data interface{}) {
	a.broadcaster.Publish(api.Event{Type: eventType, Data: data})
}

// taskFactoryAdapter 适配 TaskManager 为 scheduler.TaskFactory
type taskFactoryAdapter struct {
	manager *task.Manager
}

func (a *taskFactoryAdapter) CreateTask(goal string, priority int) error {
	return a.manager.CreateTaskFromScheduler(goal, priority)
}

// createLLMProvider 创建 LLM 提供者（自动检测可用模型）
func createLLMProvider() llm.Provider {
	return createLLMProviderFor(config.Get().LLM.DefaultProvider)
}

// createLLMProviderFor 按名称创建 provider（供路由器使用）
func createLLMProviderFor(name string) llm.Provider {
	cfg := config.Get()
	p, ok := cfg.LLM.Providers[name]
	if !ok || p.BaseURL == "" {
		return nil
	}
	apiKey := p.APIKey
	if apiKey == "" && p.APIKeyEnv != "" {
		apiKey = os.Getenv(p.APIKeyEnv)
	}
	return llm.NewOpenAIProvider(llm.Config{
		Provider:  name,
		BaseURL:   p.BaseURL,
		APIKey:    apiKey,
		Model:     p.Model,
		MaxTokens: p.MaxTokens,
		Timeout:   120,
	})
}

// startCore 启动核心服务
func startCore(database *sql.DB, httpSrv **http.Server, done chan struct{}) error {
	trajRecorders = newTrajManager(filepath.Join(config.Get().Workspace.Root, "trajectories"))
	recovered, err := task.RecoverTasks(database)
	if err != nil {
		logger.Sugar().Warnf("failed to recover tasks: %v", err)
	} else if recovered > 0 {
		logger.Sugar().Infof("recovered %d interrupted tasks", recovered)
	}

	llmProvider := createLLMProvider()
	mcpRegistry := agent.NewMcpRegistry()
	agentLoop := agent.NewLoop(database, logger, llmProvider)

	// Token 追踪器：自动记录每次 LLM 调用的 token 用量和成本
	tokenTracker := llm.NewTokenTracker(database)
	if op, ok := llmProvider.(*llm.OpenAIProvider); ok {
		op.SetTracker(tokenTracker)
	}

	// 智能路由器：根据任务类型自动选择最佳模型
	router := llm.NewRouter(
		map[string]llm.Provider{
			"zhipu":      createLLMProviderFor("zhipu"),
			"deepseek":   createLLMProviderFor("deepseek"),
			"ollama":     createLLMProviderFor("ollama"),
			"siliconflow": createLLMProviderFor("siliconflow"),
		},
		llm.RouterConfig{Default: "zhipu", Simple: "ollama", Complex: "zhipu", Vision: "zhipu", Local: "ollama"},
	)
	agentLoop.SetRouter(router)

	// 将已注册的 MCP 工具同步进 Agent Loop（使工具可被 Agent 调用）
	mcpRegistry.AttachLoop(agentLoop)
	// 注入知识库（Agent 规划时检索）
	agentLoop.SetKnowledgeBase(memory.NewKBStore(database))
	// 注入视觉分析器（截图后自动分析 UI）
	if llmProvider != nil {
		agentLoop.SetVisionAnalyzer(vision.NewAnalyzer(llmProvider))
	}
	// 步骤事件 → SSE 广播 + 轨迹记录
	agentLoop.SetStepCallback(func(eventType string, data map[string]any) {
		api.GetBroadcaster().Publish(api.Event{Type: eventType, Data: data})
		// 轨迹记录：把每个任务的关键步骤事件写入 JSONL
		if tid, _ := data["task_id"].(string); tid != "" {
			if isTerminalEvent(eventType) {
				if rec := trajRecorders.recorder(tid); rec != nil {
					_ = rec.Append(eventType, data)
				}
				trajRecorders.finalize(tid)
			} else {
				if rec := trajRecorders.recorder(tid); rec != nil {
					_ = rec.Append(eventType, data)
				}
			}
		}
	})
	publisher := &eventPublisherAdapter{broadcaster: api.GetBroadcaster()}
	taskManager := task.NewManager(database, config.Get().Agent.MaxConcurrentTasks, logger, agentLoop, publisher)
	taskManager.Start()
	sched := scheduler.NewScheduler(database, &taskFactoryAdapter{manager: taskManager}, logger)
	sched.Start()

	// 初始化主动任务执行器（事件驱动的自动恢复 + 每日摘要 + 文件观察）
	eventBus := agent.NewEventBus(database)
	policyEngine := agent.NewPolicyEngine()
	proactive := agent.NewProactiveRunner(agentLoop, eventBus, policyEngine)
	_ = proactive // 事件处理器通过 eventBus.Subscribe 自动注册
	// 将 EventBus 注入 Agent Loop（任务失败时触发主动恢复）
	agentLoop.SetEventBus(eventBus)
	// 设置追踪记录目录
	agentLoop.SetTraceDir("./data/workspace/traces")
	// 初始化反馈收集器（任务完成/失败时自动统计）
	fbStore := agent.NewFeedbackStore(database)
	_ = fbStore.InitSchema()
	fbCollector := agent.NewFeedbackCollector(fbStore, agentLoop)
	agentLoop.SetFeedbackCollector(fbCollector)
	// 初始化记忆反思引擎（自动从记忆中提炼用户画像）
	reflection := agent.NewReflectionEngine(database, memory.NewStore(database), fbStore)
	_ = reflection.InitSchema()

	// 初始化记忆生命周期管理器（active→consolidated→archived→forgotten）
	lifecycle := agent.NewMemoryLifecycle(database)
	_ = lifecycle.InitSchema()
	// 每周运行一次生命周期检查（注册为定时任务）
	lifecycleJob := &scheduler.Job{
		ID:           "memory-lifecycle",
		Name:         "记忆生命周期清理",
		TriggerType:  scheduler.TriggerInterval,
		IntervalSecs: 7 * 24 * 3600, // 每周
		GoalTemplate: "运行记忆生命周期检查：清理过期记忆，提炼用户画像",
		Priority:     2,
		Enabled:      true,
		Concurrency:  "skip",
	}
	_ = sched.CreateJob(lifecycleJob)

	// 注册每日摘要定时任务（每天 8:00 自动生成 daily digest）
	dailyDigestJob := &scheduler.Job{
		ID:           "daily-digest",
		Name:         "每日摘要",
		TriggerType:  scheduler.TriggerCron,
		GoalTemplate: "生成今日摘要：总结今天完成的任务、新增的记忆、失败的任务",
		Priority:     3,
		Enabled:      true,
		Concurrency:  "skip",
	}
	_ = sched.CreateJob(dailyDigestJob)

	// 启动 FileWatcher 触发器（监听 file_watch 类型的 job）
	fileWatcher := scheduler.NewFileWatcher(sched, logger)
	if fileWatcher != nil {
		for path := range sched.FileWatchTargets() {
			if err := fileWatcher.Watch(path); err != nil {
				logger.Sugar().Warnf("failed to watch %s: %v", path, err)
			} else {
				logger.Sugar().Infof("file watcher watching: %s", path)
			}
		}
		fwCtx, fwCancel := context.WithCancel(context.Background())
		fileWatcher.Start(fwCtx)
		_ = fwCancel
	}

	httpRouter := api.NewRouter(taskManager, sched, database, mcpRegistry, agentLoop, llmProvider, logger)
	port := config.Get().Server.Port
	*httpSrv = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: httpRouter}

	go func() {
		logger.Sugar().Infof("HTTP server listening on port %d", port)
		if err := (*httpSrv).ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar().Errorf("HTTP server error: %v", err)
		}
		close(done)
	}()
	return nil
}

type program struct {
	ctx    context.Context
	cancel context.CancelFunc
	srv    *http.Server
}

func (p *program) Start(s service.Service) error {
	p.ctx, p.cancel = context.WithCancel(context.Background())
	logger.Info("starting OpenAgent Agent service")

	database, err := db.Init(config.Get().DB.Path)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}

	interval, _ := time.ParseDuration(config.Get().Service.HeartbeatInterval)
	if interval == 0 {
		interval = 30 * time.Second
	}
	hb := observability.NewHeartbeat(interval, config.Get().Server.Port, 3, logger)
	go hb.Run(p.ctx)

	done := make(chan struct{})
	if err := startCore(database, &p.srv, done); err != nil {
		return err
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("received shutdown signal")
		p.Stop(s)
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	logger.Info("stopping OpenAgent Agent service")
	if p.srv != nil {
		p.srv.Close()
	}
	db.Close()
	observability.Sync()
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// Run 前台运行
func Run() error {
	var err error
	logger, err = observability.Init(config.Get().Observability.LogFile, config.Get().Observability.LogLevel)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer observability.Sync()

	database, err := db.Init(config.Get().DB.Path)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer database.Close()

	interval, _ := time.ParseDuration(config.Get().Service.HeartbeatInterval)
	if interval == 0 {
		interval = 30 * time.Second
	}
	hb := observability.NewHeartbeat(interval, config.Get().Server.Port, 3, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hb.Run(ctx)

	done := make(chan struct{})
	var srv *http.Server
	if err := startCore(database, &srv, done); err != nil {
		return err
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down...")
		srv.Close()
	}()
	<-done
	return nil
}

func Install() error {
	cfg := &service.Config{
		Name: config.Get().Service.Name, DisplayName: config.Get().Service.DisplayName,
		Description: "OpenAgent Agent - 24/7 autonomous computer-use agent",
		Dependencies: []string{}, WorkingDirectory: ".",
	}
	svc, err := service.New(&program{}, cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if err := svc.Install(); err != nil {
		return fmt.Errorf("install service: %w", err)
	}
	fmt.Printf("Service '%s' installed successfully\n", cfg.DisplayName)
	return nil
}

func Uninstall() error {
	cfg := &service.Config{Name: config.Get().Service.Name}
	svc, err := service.New(&program{}, cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}
	fmt.Printf("Service '%s' uninstalled successfully\n", cfg.Name)
	return nil
}

func Status() error {
	cfg := &service.Config{Name: config.Get().Service.Name}
	svc, err := service.New(&program{}, cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	status, err := svc.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}
	fmt.Printf("Service '%s' status: %v\n", cfg.Name, status)
	return nil
}
