package llm

import (
	"database/sql"
	"strings"
	"time"
)

// TokenUsage 一条 token 用量记录
type TokenUsage struct {
	ID               int64   `json:"id"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Source           string  `json:"source"` // chat/task/tool_call/review
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	DurationMs       int     `json:"duration_ms"`
	TaskID           string  `json:"task_id"`
	CreatedAt        string  `json:"created_at"`
}

// TokenTracker 记录每次 LLM 调用的 token 用量和成本
type TokenTracker struct {
	db *sql.DB
}

// NewTokenTracker 创建追踪器
func NewTokenTracker(db *sql.DB) *TokenTracker {
	return &TokenTracker{db: db}
}

// Record 记录一次 LLM 调用
func (t *TokenTracker) Record(provider, model, source, taskID string, usage Usage, durationMs int) {
	costUSD := calculateCost(provider, model, usage)
	_, _ = t.db.Exec(`
		INSERT INTO token_usage (provider, model, source, prompt_tokens, completion_tokens, total_tokens, cost_usd, duration_ms, task_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		provider, model, source,
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		costUSD, durationMs, taskID,
	)
}

// GetSummary 获取指定时间范围内的汇总
func (t *TokenTracker) GetSummary(since time.Time) (*TokenSummary, error) {
	s := &TokenSummary{}
	err := t.db.QueryRow(`
		SELECT COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		       COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost_usd),0), COUNT(*)
		FROM token_usage WHERE created_at >= ?`, since).Scan(
		&s.PromptTokens, &s.CompletionTokens, &s.TotalTokens, &s.CostUSD, &s.Calls)
	return s, err
}

// GetByProvider 按 provider 分组
func (t *TokenTracker) GetByProvider(since time.Time) ([]ProviderStats, error) {
	rows, err := t.db.Query(`
		SELECT provider, SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens), SUM(cost_usd), COUNT(*)
		FROM token_usage WHERE created_at >= ? GROUP BY provider ORDER BY SUM(cost_usd) DESC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ProviderStats
	for rows.Next() {
		var ps ProviderStats
		if err := rows.Scan(&ps.Provider, &ps.PromptTokens, &ps.CompletionTokens, &ps.TotalTokens, &ps.CostUSD, &ps.Calls); err != nil {
			continue
		}
		stats = append(stats, ps)
	}
	return stats, nil
}

// GetByDay 按天分组
func (t *TokenTracker) GetByDay(since time.Time) ([]DailyStats, error) {
	rows, err := t.db.Query(`
		SELECT date(created_at), SUM(total_tokens), SUM(cost_usd), COUNT(*)
		FROM token_usage WHERE created_at >= ? GROUP BY date(created_at) ORDER BY date(created_at)`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DailyStats
	for rows.Next() {
		var ds DailyStats
		if err := rows.Scan(&ds.Date, &ds.TotalTokens, &ds.CostUSD, &ds.Calls); err != nil {
			continue
		}
		stats = append(stats, ds)
	}
	return stats, nil
}

// GetRecent 最近 N 条记录
func (t *TokenTracker) GetRecent(limit int) ([]TokenUsage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := t.db.Query(`
		SELECT id, provider, model, source, prompt_tokens, completion_tokens, total_tokens, cost_usd, duration_ms, task_id, created_at
		FROM token_usage ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []TokenUsage
	for rows.Next() {
		var u TokenUsage
		if err := rows.Scan(&u.ID, &u.Provider, &u.Model, &u.Source, &u.PromptTokens, &u.CompletionTokens, &u.TotalTokens, &u.CostUSD, &u.DurationMs, &u.TaskID, &u.CreatedAt); err != nil {
			continue
		}
		usages = append(usages, u)
	}
	return usages, nil
}

// TokenSummary 汇总数据
type TokenSummary struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Calls            int     `json:"calls"`
}

// ProviderStats 按 provider 分组统计
type ProviderStats struct {
	Provider         string  `json:"provider"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Calls            int     `json:"calls"`
}

// DailyStats 按天统计
type DailyStats struct {
	Date        string  `json:"date"`
	TotalTokens int     `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
	Calls       int     `json:"calls"`
}

// calculateCost 根据 provider/model 计算成本（USD）
// 定价参考各 provider 官方价格（2025 年数据）
func calculateCost(provider, model string, usage Usage) float64 {
	p := float64(usage.PromptTokens)
	c := float64(usage.CompletionTokens)

	switch {
	case provider == "deepseek" && strings.Contains(model, "chat"):
		return (p*0.00014 + c*0.00028) / 1000 // DeepSeek Chat
	case provider == "deepseek" && strings.Contains(model, "reasoner"):
		return (p*0.00055 + c*0.00219) / 1000 // DeepSeek Reasoner
	case provider == "openai" && strings.Contains(model, "gpt-4o"):
		return (p*0.0025 + c*0.01) / 1000 // GPT-4o
	case provider == "openai" && strings.Contains(model, "gpt-4o-mini"):
		return (p*0.00015 + c*0.0006) / 1000 // GPT-4o-mini
	case provider == "zhipu" && strings.Contains(model, "glm-4-flash"):
		return 0 // GLM-4-Flash 免费
	case provider == "zhipu" && strings.Contains(model, "glm-4"):
		return (p*0.0001 + c*0.0001) / 1000 // GLM-4
	case provider == "siliconflow":
		return (p*0.00014 + c*0.00028) / 1000 // SiliconFlow 兼容定价
	case provider == "ollama":
		return 0 // 本地免费
	default:
		return 0 // 未知 provider，不计费
	}
}
