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

// trajRecorders global trajectory recorder manager (per-task recorder cache)
var trajRecorders *trajManager

// eventPublisherAdapter adapts to SSE broadcaster
type eventPublisherAdapter struct {
	broadcaster *api.EventBroadcaster
}

func (a *eventPublisherAdapter) Publish(eventType string, data interface{}) {
	a.broadcaster.Publish(api.Event{Type: eventType, Data: data})
}

// taskFactoryAdapter adapts TaskManager to scheduler.TaskFactory
type taskFactoryAdapter struct {
	manager *task.Manager
}

func (a *taskFactoryAdapter) CreateTask(goal string, priority int) error {
	return a.manager.CreateTaskFromScheduler(goal, priority)
}

// createLLMProvider creates LLM provider (auto-detect available models)
func createLLMProvider() llm.Provider {
	return createLLMProviderFor(config.Get().LLM.DefaultProvider)
}

// createLLMProviderFor creates provider by name (for router use)
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

// startCore starts the core service
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

	// Token tracker: auto-record token usage and cost per LLM call
	tokenTracker := llm.NewTokenTracker(database)
	if op, ok := llmProvider.(*llm.OpenAIProvider); ok {
		op.SetTracker(tokenTracker)
	}

	// Smart router: auto-select best model by task type
	router := llm.NewRouter(
		map[string]llm.Provider{
			"zhipu":       createLLMProviderFor("zhipu"),
			"deepseek":    createLLMProviderFor("deepseek"),
			"ollama":      createLLMProviderFor("ollama"),
			"siliconflow": createLLMProviderFor("siliconflow"),
		},
		llm.RouterConfig{Default: "zhipu", Simple: "ollama", Complex: "zhipu", Vision: "zhipu", Local: "ollama"},
	)
	agentLoop.SetRouter(router)

	// Sync registered MCP tools into Agent Loop (make tools callable by Agent)
	mcpRegistry.AttachLoop(agentLoop)
	// Inject knowledge base (for agent planning retrieval)
	agentLoop.SetKnowledgeBase(memory.NewKBStore(database))
	// Inject vision analyzer (auto-analyze UI after screenshot)
	if llmProvider != nil {
		agentLoop.SetVisionAnalyzer(vision.NewAnalyzer(llmProvider))
	}
	// Step events -> SSE broadcast + trajectory recording
	agentLoop.SetStepCallback(func(eventType string, data map[string]any) {
		api.GetBroadcaster().Publish(api.Event{Type: eventType, Data: data})
		// Trajectory recording: write key step events to JSONL per task
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

	// Initialize main task executor (event-driven automation + daily digest + file watcher)
	eventBus := agent.NewEventBus(database)
	policyEngine := agent.NewPolicyEngine()
	proactive := agent.NewProactiveRunner(agentLoop, eventBus, policyEngine)
	_ = proactive // event handlers register themselves via eventBus.Subscribe
	// Inject EventBus into Agent Loop (trigger proactive recovery on task failure)
	agentLoop.SetEventBus(eventBus)
	// Set trace recording directory
	agentLoop.SetTraceDir("./data/workspace/traces")
	// Initialize feedback collector (auto-stats on task completion/failure)
	fbStore := agent.NewFeedbackStore(database)
	_ = fbStore.InitSchema()
	fbCollector := agent.NewFeedbackCollector(fbStore, agentLoop)
	agentLoop.SetFeedbackCollector(fbCollector)
	// Initialize memory reflection engine (auto-distill user profile from memory)
	reflection := agent.NewReflectionEngine(database, memory.NewStore(database), fbStore)
	_ = reflection.InitSchema()

	// Initialize memory lifecycle manager (active -> consolidated -> archived -> forgotten)
	lifecycle := agent.NewMemoryLifecycle(database)
	_ = lifecycle.InitSchema()
	// Run lifecycle check weekly (registered as scheduled job)
	lifecycleJob := &scheduler.Job{
		ID:           "memory-lifecycle",
		Name:         "Memory Lifecycle Cleanup",
		TriggerType:  scheduler.TriggerInterval,
		IntervalSecs: 7 * 24 * 3600, // weekly
		GoalTemplate: "Run memory lifecycle check: clean expired memories, distill user profile",
		Priority:     2,
		Enabled:      true,
		Concurrency:  "skip",
	}
	_ = sched.CreateJob(lifecycleJob)

	// Register daily digest scheduled job (auto-generate daily digest at 8:00)
	dailyDigestJob := &scheduler.Job{
		ID:           "daily-digest",
		Name:         "Daily Digest",
		TriggerType:  scheduler.TriggerCron,
		GoalTemplate: "Generate today's digest: summarize completed tasks, new memories, failed tasks",
		Priority:     3,
		Enabled:      true,
		Concurrency:  "skip",
	}
	_ = sched.CreateJob(dailyDigestJob)

	// Start FileWatcher trigger (listen for file_watch type jobs)
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
	*httpSrv = &http.Server{Addr: fmt.Sprintf("%s:%d", config.Get().Server.Host, port), Handler: httpRouter}

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

// Run runs the service in foreground mode
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
		Name:             config.Get().Service.Name,
		DisplayName:      config.Get().Service.DisplayName,
		Description:      "OpenAgent Agent - 24/7 autonomous computer-use agent",
		Dependencies:     []string{},
		WorkingDirectory: ".",
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
