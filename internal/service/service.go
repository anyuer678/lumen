package service

import (
	"context"
	"dat abase/sql"
	"fmt"
	"net/http"
	"os"
	"os/sign al"
	"path/filepath"
	"syscall"
	"time"

	"gi thub.com/kardianos/service"
	"go.uber.org/zap "

	"agent/internal/api"
	"agent/internal/age nt"
	"agent/internal/config"
	"agent/internal /db"
	"agent/internal/llm"
	"agent/internal/m emory"
	"agent/internal/observability"
	"agen t/internal/scheduler"
	"agent/internal/task"
 	"agent/internal/vision"
)

var logger *zap.L ogger

// trajRecorders 全局轨迹记录管 理器（按 task 缓存 recorder）
var traj Recorders *trajManager

// eventPublisherAdap ter 适配 SSE 广播器
type eventPublisherA dapter struct {
	broadcaster *api.EventBroadc aster
}

func (a *eventPublisherAdapter) Publ ish(eventType string, data interface{}) {
	a. broadcaster.Publish(api.Event{Type: eventType , Data: data})
}

// taskFactoryAdapter 适� � TaskManager 为 scheduler.TaskFactory
type  taskFactoryAdapter struct {
	manager *task.Ma nager
}

func (a *taskFactoryAdapter) CreateT ask(goal string, priority int) error {
	retur n a.manager.CreateTaskFromScheduler(goal, pri ority)
}

// createLLMProvider 创建 LLM 提 供者（自动检测可用模型）
func cre ateLLMProvider() llm.Provider {
	return creat eLLMProviderFor(config.Get().LLM.DefaultProvi der)
}

// createLLMProviderFor 按名称创� �� provider（供路由器使用）
func crea teLLMProviderFor(name string) llm.Provider {
 	cfg := config.Get()
	p, ok := cfg.LLM.Provid ers[name]
	if !ok || p.BaseURL == "" {
		retu rn nil
	}
	apiKey := p.APIKey
	if apiKey == " " && p.APIKeyEnv != "" {
		apiKey = os.Getenv (p.APIKeyEnv)
	}
	return llm.NewOpenAIProvide r(llm.Config{
		Provider:  name,
		BaseURL:    p.BaseURL,
		APIKey:    apiKey,
		Model:      p.Model,
		MaxTokens: p.MaxTokens,
		Timeout :   120,
	})
}

// startCore 启动核心服� ��
func startCore(database *sql.DB, httpSrv * *http.Server, done chan struct{}) error {
	tr ajRecorders = newTrajManager(filepath.Join(co nfig.Get().Workspace.Root, "trajectories"))
	 recovered, err := task.RecoverTasks(database) 
	if err != nil {
		logger.Sugar().Warnf("fai led to recover tasks: %v", err)
	} else if re covered > 0 {
		logger.Sugar().Infof("recover ed %d interrupted tasks", recovered)
	}

	llm Provider := createLLMProvider()
	mcpRegistry  := agent.NewMcpRegistry()
	agentLoop := agent .NewLoop(database, logger, llmProvider)

	//  Token 追踪器：自动记录每次 LLM 调� ��的 token 用量和成本
	tokenTracker :=  llm.NewTokenTracker(database)
	if op, ok := l lmProvider.(*llm.OpenAIProvider); ok {
		op.S etTracker(tokenTracker)
	}

	// 智能路由� ��：根据任务类型自动选择最佳模� ��
	router := llm.NewRouter(
		map[string]llm .Provider{
			"zhipu":      createLLMProvider For("zhipu"),
			"deepseek":   createLLMProvi derFor("deepseek"),
			"ollama":     createLL MProviderFor("ollama"),
			"siliconflow": cre ateLLMProviderFor("siliconflow"),
		},
		llm. RouterConfig{Default: "zhipu", Simple: "ollam a", Complex: "zhipu", Vision: "zhipu", Local:  "ollama"},
	)
	agentLoop.SetRouter(router)

 	// 将已注册的 MCP 工具同步进 Agent  Loop（使工具可被 Agent 调用）
	mcpR egistry.AttachLoop(agentLoop)
	// 注入知� �库（Agent 规划时检索）
	agentLoop.Se tKnowledgeBase(memory.NewKBStore(database))
	 // 注入视觉分析器（截图后自动分 析 UI）
	if llmProvider != nil {
		agentLoo p.SetVisionAnalyzer(vision.NewAnalyzer(llmPro vider))
	}
	// 步骤事件 → SSE 广播 +  轨迹记录
	agentLoop.SetStepCallback(func( eventType string, data map[string]any) {
		ap i.GetBroadcaster().Publish(api.Event{Type: ev entType, Data: data})
		// 轨迹记录：把 每个任务的关键步骤事件写入 JSONL 
		if tid, _ := data["task_id"].(string); tid  != "" {
			if isTerminalEvent(eventType) {
	 			if rec := trajRecorders.recorder(tid); rec  != nil {
					_ = rec.Append(eventType, data )
				}
				trajRecorders.finalize(tid)
			}  else {
				if rec := trajRecorders.recorder(t id); rec != nil {
					_ = rec.Append(eventTy pe, data)
				}
			}
		}
	})
	publisher := &e ventPublisherAdapter{broadcaster: api.GetBroa dcaster()}
	taskManager := task.NewManager(da tabase, config.Get().Agent.MaxConcurrentTasks , logger, agentLoop, publisher)
	taskManager. Start()
	sched := scheduler.NewScheduler(data base, &taskFactoryAdapter{manager: taskManage r}, logger)
	sched.Start()

	// 初始化主� ��任务执行器（事件驱动的自动恢� �� + 每日摘要 + 文件观察）
	eventBus  := agent.NewEventBus(database)
	policyEngine  := agent.NewPolicyEngine()
	proactive := age nt.NewProactiveRunner(agentLoop, eventBus, po licyEngine)
	_ = proactive // 事件处理器 通过 eventBus.Subscribe 自动注册
	// � � EventBus 注入 Agent Loop（任务失败� �触发主动恢复）
	agentLoop.SetEventBus (eventBus)
	// 设置追踪记录目录
	agen tLoop.SetTraceDir("./data/workspace/traces")
 	// 初始化反馈收集器（任务完成/� ��败时自动统计）
	fbStore := agent.New FeedbackStore(database)
	_ = fbStore.InitSche ma()
	fbCollector := agent.NewFeedbackCollect or(fbStore, agentLoop)
	agentLoop.SetFeedback Collector(fbCollector)
	// 初始化记忆反 思引擎（自动从记忆中提炼用户画 像）
	reflection := agent.NewReflectionEngi ne(database, memory.NewStore(database), fbSto re)
	_ = reflection.InitSchema()

	// 初始� ��记忆生命周期管理器（active→cons olidated→archived→forgotten）
	lifecycle  := agent.NewMemoryLifecycle(database)
	_ = l ifecycle.InitSchema()
	// 每周运行一次� ��命周期检查（注册为定时任务）
 	lifecycleJob := &scheduler.Job{
		ID:            "memory-lifecycle",
		Name:         "记� ��生命周期清理",
		TriggerType:  schedu ler.TriggerInterval,
		IntervalSecs: 7 * 24 *  3600, // 每周
		GoalTemplate: "运行记� �生命周期检查：清理过期记忆，� �炼用户画像",
		Priority:     2,
		Enabl ed:      true,
		Concurrency:  "skip",
	}
	_  = sched.CreateJob(lifecycleJob)

	// 注册� �日摘要定时任务（每天 8:00 自动� �成 daily digest）
	dailyDigestJob := &sche duler.Job{
		ID:           "daily-digest",
		 Name:         "每日摘要",
		TriggerType:   scheduler.TriggerCron,
		GoalTemplate: "生� ��今日摘要：总结今天完成的任务� ��新增的记忆、失败的任务",
		Prior ity:     3,
		Enabled:      true,
		Concurren cy:  "skip",
	}
	_ = sched.CreateJob(dailyDig estJob)

	// 启动 FileWatcher 触发器（� ��听 file_watch 类型的 job）
	fileWatche r := scheduler.NewFileWatcher(sched, logger)
 	if fileWatcher != nil {
		for path := range  sched.FileWatchTargets() {
			if err := fileW atcher.Watch(path); err != nil {
				logger.S ugar().Warnf("failed to watch %s: %v", path,  err)
			} else {
				logger.Sugar().Infof("fi le watcher watching: %s", path)
			}
		}
		fw Ctx, fwCancel := context.WithCancel(context.B ackground())
		fileWatcher.Start(fwCtx)
		_ =  fwCancel
	}

	httpRouter := api.NewRouter(ta skManager, sched, database, mcpRegistry, agen tLoop, llmProvider, logger)
	port := config.G et().Server.Port
	*httpSrv = &http.Server{Add r: fmt.Sprintf(":%d", port), Handler: httpRou ter}

	go func() {
		logger.Sugar().Infof("HT TP server listening on port %d", port)
		if e rr := (*httpSrv).ListenAndServe(); err != nil  && err != http.ErrServerClosed {
			logger.S ugar().Errorf("HTTP server error: %v", err)
	 	}
		close(done)
	}()
	return nil
}

type pro gram struct {
	ctx    context.Context
	cancel  context.CancelFunc
	srv    *http.Server
}

f unc (p *program) Start(s service.Service) err or {
	p.ctx, p.cancel = context.WithCancel(co ntext.Background())
	logger.Info("starting Op enAgent Agent service")

	database, err := db .Init(config.Get().DB.Path)
	if err != nil {
 		return fmt.Errorf("init db: %w", err)
	}

	 interval, _ := time.ParseDuration(config.Get( ).Service.HeartbeatInterval)
	if interval ==  0 {
		interval = 30 * time.Second
	}
	hb := o bservability.NewHeartbeat(interval, config.Ge t().Server.Port, 3, logger)
	go hb.Run(p.ctx) 

	done := make(chan struct{})
	if err := sta rtCore(database, &p.srv, done); err != nil {
 		return err
	}

	go func() {
		sigCh := make (chan os.Signal, 1)
		signal.Notify(sigCh, sy scall.SIGINT, syscall.SIGTERM)
		<-sigCh
		lo gger.Info("received shutdown signal")
		p.Sto p(s)
	}()
	return nil
}

func (p *program) St op(s service.Service) error {
	logger.Info("s topping OpenAgent Agent service")
	if p.srv ! = nil {
		p.srv.Close()
	}
	db.Close()
	obser vability.Sync()
	if p.cancel != nil {
		p.can cel()
	}
	return nil
}

// Run 前台运行
f unc Run() error {
	var err error
	logger, err  = observability.Init(config.Get().Observabil ity.LogFile, config.Get().Observability.LogLe vel)
	if err != nil {
		return fmt.Errorf("in it logger: %w", err)
	}
	defer observability. Sync()

	database, err := db.Init(config.Get( ).DB.Path)
	if err != nil {
		return fmt.Erro rf("init db: %w", err)
	}
	defer database.Clo se()

	interval, _ := time.ParseDuration(conf ig.Get().Service.HeartbeatInterval)
	if inter val == 0 {
		interval = 30 * time.Second
	}
	 hb := observability.NewHeartbeat(interval, co nfig.Get().Server.Port, 3, logger)
	ctx, canc el := context.WithCancel(context.Background() )
	defer cancel()
	go hb.Run(ctx)

	done := m ake(chan struct{})
	var srv *http.Server
	if  err := startCore(database, &srv, done); err ! = nil {
		return err
	}

	go func() {
		sigCh  := make(chan os.Signal, 1)
		signal.Notify(s igCh, syscall.SIGINT, syscall.SIGTERM)
		<-si gCh
		logger.Info("shutting down...")
		srv.C lose()
	}()
	<-done
	return nil
}

func Insta ll() error {
	cfg := &service.Config{
		Name:  config.Get().Service.Name, DisplayName: conf ig.Get().Service.DisplayName,
		Description:  "OpenAgent Agent - 24/7 autonomous computer-u se agent",
		Dependencies: []string{}, Workin gDirectory: ".",
	}
	svc, err := service.New( &program{}, cfg)
	if err != nil {
		return fm t.Errorf("create service: %w", err)
	}
	if er r := svc.Install(); err != nil {
		return fmt .Errorf("install service: %w", err)
	}
	fmt.P rintf("Service '%s' installed successfully\n" , cfg.DisplayName)
	return nil
}

func Uninst all() error {
	cfg := &service.Config{Name: c onfig.Get().Service.Name}
	svc, err := servic e.New(&program{}, cfg)
	if err != nil {
		ret urn fmt.Errorf("create service: %w", err)
	}
 	if err := svc.Uninstall(); err != nil {
		re turn fmt.Errorf("uninstall service: %w", err) 
	}
	fmt.Printf("Service '%s' uninstalled suc cessfully\n", cfg.Name)
	return nil
}

func S tatus() error {
	cfg := &service.Config{Name:  config.Get().Service.Name}
	svc, err := serv ice.New(&program{}, cfg)
	if err != nil {
		r eturn fmt.Errorf("create service: %w", err)
	 }
	status, err := svc.Status()
	if err != nil  {
		return fmt.Errorf("get status: %w", err) 
	}
	fmt.Printf("Service '%s' status: %v\n",  cfg.Name, status)
	return nil
}
 