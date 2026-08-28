package service

import (
	"context"
	"dat  abase/sql"
	"fmt"
	"net/http"
	"os"
	"os/sig n al"
	"path/filepath"
	"syscall"
	"time"

	" gi thub.com/kardianos/service"
	"go.uber.org/ zap "

	"agent/internal/api"
	"agent/internal /age nt"
	"agent/internal/config"
	"agent/int ernal /db"
	"agent/internal/llm"
	"agent/inte rnal/m emory"
	"agent/internal/observability" 
	"agen t/internal/scheduler"
	"agent/interna l/task"
 	"agent/internal/vision"
)

var logg er *zap.L ogger

// trajRecorders 全局轨� �记录管 理器（按 task 缓存 recorder� ��
var traj Recorders *trajManager

// eventP ublisherAdap ter 适配 SSE 广播器
type ev entPublisherA dapter struct {
	broadcaster *a pi.EventBroadc aster
}

func (a *eventPublish erAdapter) Publ ish(eventType string, data in terface{}) {
	a. broadcaster.Publish(api.Even t{Type: eventType , Data: data})
}

// taskFa ctoryAdapter 适� � TaskManager 为 sched uler.TaskFactory
type  taskFactoryAdapter str uct {
	manager *task.Ma nager
}

func (a *tas kFactoryAdapter) CreateT ask(goal string, pri ority int) error {
	retur n a.manager.CreateT askFromScheduler(goal, pri ority)
}

// creat eLLMProvider 创建 LLM 提 供者（自动� �测可用模型）
func cre ateLLMProvider()  llm.Provider {
	return creat eLLMProviderFor (config.Get().LLM.DefaultProvi der)
}

// cre ateLLMProviderFor 按名称创� �� prov ider（供路由器使用）
func crea teLLMP roviderFor(name string) llm.Provider {
 	cfg  := config.Get()
	p, ok := cfg.LLM.Provid ers[ name]
	if !ok || p.BaseURL == "" {
		retu rn  nil
	}
	apiKey := p.APIKey
	if apiKey == " "  && p.APIKeyEnv != "" {
		apiKey = os.Getenv ( p.APIKeyEnv)
	}
	return llm.NewOpenAIProvide  r(llm.Config{
		Provider:  name,
		BaseURL:     p.BaseURL,
		APIKey:    apiKey,
		Model:       p.Model,
		MaxTokens: p.MaxTokens,
		Timeo ut :   120,
	})
}

// startCore 启动核心� ��� ��
func startCore(database *sql.DB,  httpSrv * *http.Server, done chan struct{})  error {
	tr ajRecorders = newTrajManager(file path.Join(co nfig.Get().Workspace.Root, "traj ectories"))
	 recovered, err := task.RecoverT asks(database) 
	if err != nil {
		logger.Sug ar().Warnf("fai led to recover tasks: %v", er r)
	} else if re covered > 0 {
		logger.Sugar ().Infof("recover ed %d interrupted tasks", r ecovered)
	}

	llm Provider := createLLMProvi der()
	mcpRegistry  := agent.NewMcpRegistry() 
	agentLoop := agent .NewLoop(database, logge r, llmProvider)

	//  Token 追踪器：自� �记录每次 LLM 调� ��的 token 用� ��和成本
	tokenTracker :=  llm.NewTokenTra cker(database)
	if op, ok := l lmProvider.(*l lm.OpenAIProvider); ok {
		op.S etTracker(tok enTracker)
	}

	// 智能路由� ��：� ��据任务类型自动选择最佳模� � �
	router := llm.NewRouter(
		map[string]ll m .Provider{
			"zhipu":      createLLMProvid er For("zhipu"),
			"deepseek":   createLLMPr ovi derFor("deepseek"),
			"ollama":     crea teLL MProviderFor("ollama"),
			"siliconflow" : cre ateLLMProviderFor("siliconflow"),
		},
 		llm. RouterConfig{Default: "zhipu", Simple:  "ollam a", Complex: "zhipu", Vision: "zhipu" , Local:  "ollama"},
	)
	agentLoop.SetRouter( router)

 	// 将已注册的 MCP 工具同� �进 Agent  Loop（使工具可被 Agent 调� ��）
	mcpR egistry.AttachLoop(agentLoop)
	//  注入知� �库（Agent 规划时检索� ��
	agentLoop.Se tKnowledgeBase(memory.NewKBS tore(database))
	 // 注入视觉分析器（ 截图后自动分 析 UI）
	if llmProvider  != nil {
		agentLoo p.SetVisionAnalyzer(visio n.NewAnalyzer(llmPro vider))
	}
	// 步骤事 件 → SSE 广播 +  轨迹记录
	agentLoop .SetStepCallback(func( eventType string, data  map[string]any) {
		ap i.GetBroadcaster().Pu blish(api.Event{Type: ev entType, Data: data} )
		// 轨迹记录：把 每个任务的关� ��步骤事件写入 JSONL 
		if tid, _ := da ta["task_id"].(string); tid  != "" {
			if is TerminalEvent(eventType) {
	 			if rec := tra jRecorders.recorder(tid); rec  != nil {
					 _ = rec.Append(eventType, data )
				}
				tr ajRecorders.finalize(tid)
			}  else {
				if  rec := trajRecorders.recorder(t id); rec !=  nil {
					_ = rec.Append(eventTy pe, data)
	 			}
			}
		}
	})
	publisher := &e ventPublis herAdapter{broadcaster: api.GetBroa dcaster() }
	taskManager := task.NewManager(da tabase,  config.Get().Agent.MaxConcurrentTasks , logge r, agentLoop, publisher)
	taskManager. Start( )
	sched := scheduler.NewScheduler(data base,  &taskFactoryAdapter{manager: taskManage r},  logger)
	sched.Start()

	// 初始化主� � ���任务执行器（事件驱动的自动� ��� �� + 每日摘要 + 文件观察） 
	eventBus  := agent.NewEventBus(database)
	p olicyEngine  := agent.NewPolicyEngine()
	proa ctive := age nt.NewProactiveRunner(agentLoop,  eventBus, po licyEngine)
	_ = proactive // � ��件处理器 通过 eventBus.Subscribe 自� ��注册
	// � � EventBus 注入 Agent Lo op（任务失败� �触发主动恢复） 
	agentLoop.SetEventBus (eventBus)
	// 设置 追踪记录目录
	agen tLoop.SetTraceDir(". /data/workspace/traces")
 	// 初始化反馈 收集器（任务完成/� ��败时自� ��统计）
	fbStore := agent.New FeedbackSto re(database)
	_ = fbStore.InitSche ma()
	fbCo llector := agent.NewFeedbackCollect or(fbStor e, agentLoop)
	agentLoop.SetFeedback Collecto r(fbCollector)
	// 初始化记忆反 思引� ��（自动从记忆中提炼用户画 像） 
	reflection := agent.NewReflectionEngi ne(da tabase, memory.NewStore(database), fbSto re)
 	_ = reflection.InitSchema()

	// 初始� � ���记忆生命周期管理器（active→c ons olidated→archived→forgotten）
	lifec ycle  := agent.NewMemoryLifecycle(database)
	 _ = l ifecycle.InitSchema()
	// 每周运行� ��次� ��命周期检查（注册为定 时任务）
 	lifecycleJob := &scheduler.Job {
		ID:            "memory-lifecycle",
		Name :         "记� ��生命周期清理",
 		TriggerType:  schedu ler.TriggerInterval,
	 	IntervalSecs: 7 * 24 *  3600, // 每周
		Go alTemplate: "运行记� �生命周期检� ��：清理过期记忆，� �炼用户画 像",
		Priority:     2,
		Enabl ed:      tru e,
		Concurrency:  "skip",
	}
	_  = sched.Cre ateJob(lifecycleJob)

	// 注册� �日摘 要定时任务（每天 8:00 自动� �� � daily digest）
	dailyDigestJob := &sche du ler.Job{
		ID:           "daily-digest",
		 N ame:         "每日摘要",
		TriggerType:    scheduler.TriggerCron,
		GoalTemplate: "生� �� ��今日摘要：总结今天完成的 任务� ��新增的记忆、失败的� �务",
		Prior ity:     3,
		Enabled:      tr ue,
		Concurren cy:  "skip",
	}
	_ = sched.Cr eateJob(dailyDig estJob)

	// 启动 FileWatc her 触发器（� ��听 file_watch 类� ��的 job）
	fileWatche r := scheduler.NewFi leWatcher(sched, logger)
 	if fileWatcher !=  nil {
		for path := range  sched.FileWatchTar gets() {
			if err := fileW atcher.Watch(path ); err != nil {
				logger.S ugar().Warnf("fa iled to watch %s: %v", path,  err)
			} else  {
				logger.Sugar().Infof("fi le watcher wat ching: %s", path)
			}
		}
		fw Ctx, fwCancel  := context.WithCancel(context.B ackground()) 
		fileWatcher.Start(fwCtx)
		_ =  fwCancel
	 }

	httpRouter := api.NewRouter(ta skManager,  sched, database, mcpRegistry, agen tLoop, ll mProvider, logger)
	port := config.G et().Ser ver.Port
	*httpSrv = &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, port), Handler: httpRou ter}

 	go func() {
		logger.Sugar().Infof("HT TP se rver listening on port %d", port)
		if e rr : = (*httpSrv).ListenAndServe(); err != nil  &&  err != http.ErrServerClosed {
			logger.S ug ar().Errorf("HTTP server error: %v", err)
	 	 }
		close(done)
	}()
	return nil
}

type pro  gram struct {
	ctx    context.Context
	cancel   context.CancelFunc
	srv    *http.Server
}

 func (p *program) Start(s service.Service) e rr or {
	p.ctx, p.cancel = context.WithCancel (co ntext.Background())
	logger.Info("startin g Op enAgent Agent service")

	database, err  := db .Init(config.Get().DB.Path)
	if err !=  nil {
 		return fmt.Errorf("init db: %w", err )
	}

	 interval, _ := time.ParseDuration(con fig.Get( ).Service.HeartbeatInterval)
	if int erval ==  0 {
		interval = 30 * time.Second
	 }
	hb := o bservability.NewHeartbeat(interval , config.Ge t().Server.Port, 3, logger)
	go h b.Run(p.ctx) 

	done := make(chan struct{})
	 if err := sta rtCore(database, &p.srv, done);  err != nil {
 		return err
	}

	go func() {
 		sigCh := make (chan os.Signal, 1)
		signal. Notify(sigCh, sy scall.SIGINT, syscall.SIGTER M)
		<-sigCh
		lo gger.Info("received shutdow n signal")
		p.Sto p(s)
	}()
	return nil
}

func (p *program) St op(s service.Service) err or {
	logger.Info("s topping OpenAgent Agent  service")
	if p.srv ! = nil {
		p.srv.Close() 
	}
	db.Close()
	obser vability.Sync()
	if p. cancel != nil {
		p.can cel()
	}
	return nil
 }

// Run 前台运行
func Run() error {
	v ar err error
	logger, err  = observability.In it(config.Get().Observabil ity.LogFile, confi g.Get().Observability.LogLe vel)
	if err != n il {
		return fmt.Errorf("in it logger: %w",  err)
	}
	defer observability. Sync()

	databa se, err := db.Init(config.Get( ).DB.Path)
	if  err != nil {
		return fmt.Erro rf("init db:  %w", err)
	}
	defer database.Clo se()

	inter val, _ := time.ParseDuration(conf ig.Get().Se rvice.HeartbeatInterval)
	if inter val == 0 { 
		interval = 30 * time.Second
	}
	 hb := obs ervability.NewHeartbeat(interval, co nfig.Get ().Server.Port, 3, logger)
	ctx, canc el := c ontext.WithCancel(context.Background() )
	def er cancel()
	go hb.Run(ctx)

	done := m ake(c han struct{})
	var srv *http.Server
	if  err  := startCore(database, &srv, done); err ! = n il {
		return err
	}

	go func() {
		sigCh  : = make(chan os.Signal, 1)
		signal.Notify(s i gCh, syscall.SIGINT, syscall.SIGTERM)
		<-si  gCh
		logger.Info("shutting down...")
		srv.C  lose()
	}()
	<-done
	return nil
}

func Inst a ll() error {
	cfg := &service.Config{
		Nam e:  config.Get().Service.Name, DisplayName: c onf ig.Get().Service.DisplayName,
		Descripti on:  "OpenAgent Agent - 24/7 autonomous compu ter-u se agent",
		Dependencies: []string{},  Workin gDirectory: ".",
	}
	svc, err := servi ce.New( &program{}, cfg)
	if err != nil {
		r eturn fm t.Errorf("create service: %w", err)
 	}
	if er r := svc.Install(); err != nil {
		 return fmt .Errorf("install service: %w", err )
	}
	fmt.P rintf("Service '%s' installed suc cessfully\n" , cfg.DisplayName)
	return nil
} 

func Uninst all() error {
	cfg := &service. Config{Name: c onfig.Get().Service.Name}
	svc , err := servic e.New(&program{}, cfg)
	if er r != nil {
		ret urn fmt.Errorf("create servi ce: %w", err)
	}
 	if err := svc.Uninstall();  err != nil {
		re turn fmt.Errorf("uninstall  service: %w", err) 
	}
	fmt.Printf("Service  '%s' uninstalled suc cessfully\n", cfg.Name)
 	return nil
}

func S tatus() error {
	cfg :=  &service.Config{Name:  config.Get().Service. Name}
	svc, err := serv ice.New(&program{}, c fg)
	if err != nil {
		r eturn fmt.Errorf("cr eate service: %w", err)
	 }
	status, err := s vc.Status()
	if err != nil  {
		return fmt.Er rorf("get status: %w", err) 
	}
	fmt.Printf(" Service '%s' status: %v\n",  cfg.Name, status )
	return nil
}
  