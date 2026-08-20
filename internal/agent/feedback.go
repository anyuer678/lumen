package agent

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agent/internal/task"
)

// FeedbackEntry 一条反馈记录
type FeedbackEntry struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	Goal       string    `json:"goal"`
	Category   string    `json:"category"`   // task_type 分类
	ToolUsed   string    `json:"tool_used"`  // 使用的工具
	Success    bool      `json:"success"`    // 是否成功
	Duration   float64   `json:"duration"`   // 耗时（秒）
	ErrorType  string    `json:"error_type"` // 失败原因分类
	StepsCount int       `json:"steps_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// FeedbackStore 反馈存储
type FeedbackStore struct {
	db *sql.DB
}

// NewFeedbackStore 创建反馈存储
func NewFeedbackStore(db *sql.DB) *FeedbackStore {
	return &FeedbackStore{db: db}
}

// InitSchema 初始化数据库表
func (s *FeedbackStore) InitSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS feedback (
			id TEXT PRIMARY KEY,
			task_id TEXT,
			goal TEXT,
			category TEXT,
			tool_used TEXT,
			success INTEGER,
			duration REAL,
			error_type TEXT,
			steps_count INTEGER,
			created_at DATETIME
		)
	`)
	return err
}

// Record 记录一条反馈
func (s *FeedbackStore) Record(entry *FeedbackEntry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("fb-%d", time.Now().UnixNano())
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	_, err := s.db.Exec(`
		INSERT INTO feedback (id, task_id, goal, category, tool_used, success, duration, error_type, steps_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.TaskID, entry.Goal, entry.Category, entry.ToolUsed,
		entry.Success, entry.Duration, entry.ErrorType, entry.StepsCount, entry.CreatedAt)
	return err
}

// GetToolSuccessRate 获取工具成功率
func (s *FeedbackStore) GetToolSuccessRate(toolName string) (float64, int) {
	var total, success int
	s.db.QueryRow(`SELECT COUNT(*), SUM(success) FROM feedback WHERE tool_used = ?`, toolName).Scan(&total, &success)
	if total == 0 {
		return 0, 0
	}
	return float64(success) / float64(total) * 100, total
}

// GetCommonErrors 获取常见错误类型
func (s *FeedbackStore) GetCommonErrors(limit int) []struct {
	ErrorType string
	Count     int
} {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT error_type, COUNT(*) as cnt 
		FROM feedback 
		WHERE success = 0 AND error_type != '' 
		GROUP BY error_type 
		ORDER BY cnt DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []struct {
		ErrorType string
		Count     int
	}
	for rows.Next() {
		var r struct {
			ErrorType string
			Count     int
		}
		rows.Scan(&r.ErrorType, &r.Count)
		results = append(results, r)
	}
	return results
}

// GetSuccessPatterns 获取成功模式（哪些目标类型+工具组合成功率高）
func (s *FeedbackStore) GetSuccessPatterns() []struct {
	Category string
	Tool     string
	Rate     float64
	Count    int
} {
	rows, err := s.db.Query(`
		SELECT category, tool_used, 
			CAST(SUM(success) AS REAL) / COUNT(*) * 100 as rate,
			COUNT(*) as cnt
		FROM feedback 
		WHERE category != '' AND tool_used != ''
		GROUP BY category, tool_used
		HAVING cnt >= 2
		ORDER BY rate DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []struct {
		Category string
		Tool     string
		Rate     float64
		Count    int
	}
	for rows.Next() {
		var r struct {
			Category string
			Tool     string
			Rate     float64
			Count    int
		}
		rows.Scan(&r.Category, &r.Tool, &r.Rate, &r.Count)
		results = append(results, r)
	}
	return results
}

// GenerateHint 为 Planner 生成基于历史反馈的建议
func (s *FeedbackStore) GenerateHint(goal string) string {
	var hints []string

	// 查找类似目标的历史成功模式
	patterns := s.GetSuccessPatterns()
	if len(patterns) > 0 {
		best := patterns[0]
		if best.Rate > 80 {
			hints = append(hints, fmt.Sprintf("历史数据显示：使用 %s 工具处理 %s 类任务成功率 %.0f%%", best.Tool, best.Category, best.Rate))
		}
	}

	// 查找常见错误，提示避免
	errors := s.GetCommonErrors(3)
	if len(errors) > 0 {
		var errList []string
		for _, e := range errors {
			errList = append(errList, fmt.Sprintf("%s(%d次)", e.ErrorType, e.Count))
		}
		hints = append(hints, fmt.Sprintf("常见失败原因：%s", strings.Join(errList, ", ")))
	}

	if len(hints) == 0 {
		return ""
	}
	return "[历史反馈] " + strings.Join(hints, "；")
}

// FeedbackCollector 反馈收集器：从任务执行结果自动收集反馈
type FeedbackCollector struct {
	store *FeedbackStore
	loop  *Loop
}

// NewFeedbackCollector 创建反馈收集器
func NewFeedbackCollector(store *FeedbackStore, loop *Loop) *FeedbackCollector {
	return &FeedbackCollector{store: store, loop: loop}
}

// CollectFromTask 从任务执行结果收集反馈
func (c *FeedbackCollector) CollectFromTask(t *task.Task, duration float64, stepsCount int) {
	if c.store == nil {
		return
	}

	// 分类目标
	category := categorizeGoal(t.Goal)

	// 提取使用的工具
	toolUsed := ""
	if t.Result != "" {
		// 简单启发式：从结果中提取工具名
		for _, tool := range []string{"shell.run", "fs", "browser", "system", "windows", "subagent"} {
			if strings.Contains(t.Result, tool) {
				toolUsed = tool
				break
			}
		}
	}

	// 分类错误
	errorType := ""
	if !isTaskSuccess(t) && t.Error != "" {
		errorType = classifyErrorType(t.Error)
	}

	entry := &FeedbackEntry{
		TaskID:     t.ID,
		Goal:       t.Goal,
		Category:   category,
		ToolUsed:   toolUsed,
		Success:    isTaskSuccess(t),
		Duration:   duration,
		ErrorType:  errorType,
		StepsCount: stepsCount,
	}

	c.store.Record(entry)
	c.loop.logger.Infof("feedback: recorded task %s (success=%v, tool=%s, category=%s)", t.ID, entry.Success, toolUsed, category)
}

// categorizeGoal 根据目标文本分类任务类型
func categorizeGoal(goal string) string {
	lower := strings.ToLower(goal)
	switch {
	case strings.Contains(lower, "命令") || strings.Contains(lower, "command") || strings.Contains(lower, "exec"):
		return "shell"
	case strings.Contains(lower, "文件") || strings.Contains(lower, "file") || strings.Contains(lower, "目录"):
		return "filesystem"
	case strings.Contains(lower, "系统") || strings.Contains(lower, "system") || strings.Contains(lower, "进程"):
		return "system"
	case strings.Contains(lower, "网页") || strings.Contains(lower, "browser") || strings.Contains(lower, "url"):
		return "browser"
	case strings.Contains(lower, "创建") || strings.Contains(lower, "create") || strings.Contains(lower, "任务"):
		return "task"
	case strings.Contains(lower, "搜索") || strings.Contains(lower, "search") || strings.Contains(lower, "查"):
		return "search"
	default:
		return "general"
	}
}

// classifyErrorType 分类错误类型
func classifyErrorType(errMsg string) string {
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline"):
		return "timeout"
	case strings.Contains(lower, "permission") || strings.Contains(lower, "denied") || strings.Contains(lower, "拦截"):
		return "permission"
	case strings.Contains(lower, "tool") && strings.Contains(lower, "not found"):
		return "tool_not_found"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "不存在"):
		return "not_found"
	case strings.Contains(lower, "network") || strings.Contains(lower, "connection"):
		return "network"
	default:
		return "other"
	}
}

// isTaskSuccess 判断任务是否成功
func isTaskSuccess(t *task.Task) bool {
	return t.Status == task.StatusCompleted
}
