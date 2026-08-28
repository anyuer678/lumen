package agent

import (
	"context"
	"fmt"
	"p ath/filepath"
	"strings"
	"time"

	"agent/int ernal/memory"
	"agent/internal/task"
)

// Pr oactiveRunner 主动任务执行器
type Proa ctiveRunner struct {
	loop     *Loop
	eventBu s *EventBus
	policy   *PolicyEngine
}

// New ProactiveRunner 创建主动任务执行器
f unc NewProactiveRunner(loop *Loop, eventBus * EventBus, policy *PolicyEngine) *ProactiveRun ner {
	r := &ProactiveRunner{
		loop:     loo p,
		eventBus: eventBus,
		policy:   policy,
 	}

	// 注册事件处理器
	eventBus.Subsc ribe(EventFileCreated, r.onFileCreated)
	even tBus.Subscribe(EventTaskFailed, r.onTaskFaile d)

	return r
}

// onFileCreated 文件创� �事件处理器
func (r *ProactiveRunner) on FileCreated(event Event) error {
	// 策略� �查
	shouldAct, reason, _ := r.policy.Evalua te(event)
	if !shouldAct {
		r.loop.logger.In fof("proactive: file.created ignored (%s)", r eason)
		return nil
	}

	// 自动整理文� �
	filePath := event.Payload
	ext := strings. ToLower(filepath.Ext(filePath))

	// 只处� �特定类型
	switch ext {
	case ".pdf", ".d ocx", ".xlsx", ".png", ".jpg", ".zip":
		r.lo op.logger.Infof("proactive: auto-organizing f ile %s", filePath)
		// 执行 organize
		res ult, err := r.loop.RunTool(context.Background (), "fs", map[string]any{
			"action": "organ ize",
			"path":   filepath.Dir(filePath),
		 })
		if err != nil {
			r.loop.logger.Errorf( "proactive: organize failed: %v", err)
			ret urn err
		}
		r.loop.logger.Infof("proactive:  organize result: %s", result.Summary)
		retu rn nil
	}

	return nil
}

// onTaskFailed 任 务失败事件处理器：分析失败原因  → 判断是否可恢复 → 自动重试
f unc (r *ProactiveRunner) onTaskFailed(event E vent) error {
	// 策略检查
	shouldAct, re ason, _ := r.policy.Evaluate(event)
	if !shou ldAct {
		r.loop.logger.Infof("proactive: tas k.failed ignored (%s)", reason)
		return nil
 	}

	taskID := event.Payload
	if taskID == ""  {
		return nil
	}

	r.loop.logger.Infof("pro active: auto-recovery for task %s", taskID)

 	// 1. 从 store 加载失败任务
	t, err : = r.loop.store.GetTask(taskID)
	if err != nil  || t == nil {
		r.loop.logger.Warnf("proacti ve: cannot load task %s: %v", taskID, err)
		 return nil
	}

	// 2. 分析失败原因，� �断是否可恢复
	errorMsg := t.Error
	if  errorMsg == "" {
		errorMsg = "unknown error" 
	}

	recoverable, reason := classifyFailure( errorMsg)
	if !recoverable {
		r.loop.logger. Infof("proactive: task %s failure not recover able (%s), skipping", taskID, reason)
		retur n nil
	}

	// 3. 检查重试次数（防止� ��限循环）
	if t.RetryCount >= 2 {
		r.lo op.logger.Infof("proactive: task %s already r etried %d times, giving up", taskID, t.RetryC ount)
		return nil
	}

	// 4. 创建新任务 （带改良目标）
	newGoal := fmt.Sprintf ("[自动恢复] %s（第 %d 次重试，前� ��失败原因: %s）", t.Goal, t.RetryCount+ 1, reason)
	r.loop.logger.Infof("proactive: c reating recovery task for %s → %s", taskID,  newGoal)

	// 通过 Agent Loop 执行
	ctx  := context.Background()
	newTask := &task.Tas k{
		ID:         fmt.Sprintf("recovery-%s-%d" , taskID, time.Now().UnixMilli()),
		Goal:        newGoal,
		Type:       "retry",
		Status:      task.StatusRunning,
		Priority:   t.Prio rity,
		RetryCount: t.RetryCount + 1,
	}
	if  err := r.loop.store.SaveTask(newTask); err !=  nil {
		r.loop.logger.Errorf("proactive: fai led to save recovery task: %v", err)
		return  nil
	}

	// 异步执行
	go func() {
		if e rr := r.loop.Run(ctx, newTask); err != nil {
 			r.loop.logger.Errorf("proactive: recovery  task %s failed: %v", newTask.ID, err)
			// � ��射失败事件（触发下一轮恢复）
 			r.eventBus.EmitAsync(Event{
				Source:    "proactive",
				Type:     EventTaskFailed,
	 			Payload:  newTask.ID,
				Priority: 5,
			 })
		} else {
			r.loop.logger.Infof("proacti ve: recovery task %s completed", newTask.ID)
 		}
	}()

	return nil
}

// classifyFailure � ��类失败原因，判断是否可恢复
//  返回 (可恢复, 供人类阅读的原因)
 func classifyFailure(errorMsg string) (bool,  string) {
	lower := strings.ToLower(errorMsg) 

	// 永久性错误（不可恢复）
	perm anent := []string{
		"permission denied", "ac cess denied", "access violation",
		"安全� �截", "blocked", "denied",
		"no such file o r directory", "文件不存在",
		"unknown t ool", "tool not found",
		"subagents cannot n est",
	}
	for _, kw := range permanent {
		if  strings.Contains(lower, kw) {
			return fals e, "permanent: " + kw
		}
	}

	// 可恢复� �误（网络/超时/临时不可用）
	reco verable := []string{
		"timeout", "deadline e xceeded", "connection refused",
		"context ca nceled", "i/o timeout",
		"exit status 1", "e xit status 2",
		"retry", "temporary",
		"net work", "unreachable",
	}
	for _, kw := range  recoverable {
		if strings.Contains(lower, kw ) {
			return true, "transient: " + kw
		}
	} 

	// 未知错误：默认可恢复（给一 次机会）
	return true, "unknown error"
}
 
// RunDailyResearch 定时研究任务
func  (r *ProactiveRunner) RunDailyResearch(query s tring) error {
	// 策略检查
	event := Eve nt{
		Source:   "scheduler",
		Type:     Even tScheduleTrigger,
		Payload:  "daily_research ",
		Priority: 5,
	}
	shouldAct, reason, _ :=  r.policy.Evaluate(event)
	if !shouldAct {
		 r.loop.logger.Infof("proactive: daily researc h skipped (%s)", reason)
		return nil
	}

	r. loop.logger.Infof("proactive: running daily r esearch: %s", query)

	// 执行搜索
	resul t, err := r.loop.RunTool(context.Background() , "browser", map[string]any{
		"action": "res earch",
		"query":  query,
	})
	if err != nil  {
		return fmt.Errorf("research failed: %w",  err)
	}

	// 保存到知识库
	if r.loop.k b != nil {
		kb := &memory.Knowledge{
			ID:         fmt.Sprintf("research-%d", time.Now(). UnixNano()),
			Title:     "Daily Research: "  + query,
			Content:   result.Raw,
			Tags:       "research,daily",
			Source:    "proacti ve",
			CreatedAt: time.Now(),
		}
		r.loop.k b.Add(kb)
	}

	return nil
}

// GenerateDaily Digest 生成每日摘要
func (r *ProactiveR unner) GenerateDailyDigest() (string, error)  {
	if r.loop.memoryStore == nil {
		return "" , fmt.Errorf("memory store not initialized")
 	}

	// 获取今天完成的任务
	today :=  time.Now().Format("2006-01-02")
	var sb stri ngs.Builder
	sb.WriteString(fmt.Sprintf("## % s Daily Digest\n\n", today))

	// 获取最� �的记忆
	mems, _ := r.loop.memoryStore.Get Recent(20)
	sb.WriteString(fmt.Sprintf("### � ��近记忆 (%d 条)\n", len(mems)))
	for _,  m := range mems {
		sb.WriteString(fmt.Sprint f("- %s: %s\n", m.Type, truncateString(m.Cont ent, 60)))
	}

	// 获取事件
	events, _ :=  r.eventBus.GetRecent(10)
	sb.WriteString(fmt .Sprintf("\n### 最近事件 (%d 条)\n", len (events)))
	for _, e := range events {
		sb.W riteString(fmt.Sprintf("- [%s] %s: %s\n", e.T ype, e.Source, truncateString(e.Payload, 60)) )
	}

	// 获取统计
	memCount, _ := r.loop .memoryStore.GetCount()
	sb.WriteString(fmt.S printf("\n### 统计\n- 总记忆数: %d\n",  memCount))

	digest := sb.String()

	// 保� �到知识库
	if r.loop.kb != nil {
		kb :=  &memory.Knowledge{
			ID:        fmt.Sprintf( "digest-%s", today),
			Title:     fmt.Sprint f("Daily Digest: %s", today),
			Content:   d igest,
			Tags:      "digest,daily",
			Sourc e:    "proactive",
			CreatedAt: time.Now(),
 		}
		r.loop.kb.Add(kb)
	}

	return digest, n il
}

func truncateString(s string, n int) st ring {
	if len(s) <= n {
		return s
	}
	retur n s[:n] + "..."
}
 