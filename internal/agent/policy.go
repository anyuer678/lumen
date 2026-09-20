package agent

import (
	"strings"
	"sync"
	"time"
)

// Policy 决策规则
type Policy struct {
	Name              string    `json:"name"`
	EventType         EventType `json:"event_type"`
	Enabled           bool      `json:"enabled"`
	QuietHoursStart   int       `json:"quiet_hours_start"`
	QuietHoursEnd     int       `json:"quiet_hours_end"`
	MaxActionsPerHour int       `json:"max_actions_per_hour"`
	AllowedTools      []string  `json:"allowed_tools"`
	RequireConfirm    bool      `json:"require_confirm"`
	MinPriority       int       `json:"min_priority"`
	Keywords          []string  `json:"keywords"`
	MaxCostPerDay     float64   `json:"max_cost_per_day"`
}

// PolicyEngine 决策引擎
type PolicyEngine struct {
	policies  []Policy
	mu        sync.Mutex
	actionLog []actionRecord
}

type actionRecord struct {
	timestamp time.Time
	policy    string
	action    string
}

// NewPolicyEngine 创建决策引擎。
// 安全默认与 conf/policy.yaml 对齐：
// - 一切会调用 shell.run 的策略 RequireConfirm=true
// - 不在代码默认里提供「静默 shell」路径
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: []Policy{
			{
				Name:              "file_organize",
				EventType:         EventFileCreated,
				Enabled:           true,
				QuietHoursStart:   23,
				QuietHoursEnd:     8,
				MaxActionsPerHour: 5,
				AllowedTools:      []string{"fs"},
				RequireConfirm:    true, // 移动用户文件需确认（与 yaml auto_execute:false 一致）
				MinPriority:       3,
				Keywords:          []string{".pdf", ".docx", ".xlsx", ".png", ".jpg", ".zip"},
			},
			{
				Name:              "task_auto_retry",
				EventType:         EventTaskFailed,
				Enabled:           true,
				QuietHoursStart:   23,
				QuietHoursEnd:     8,
				MaxActionsPerHour: 3,
				// 默认不自动 shell 重试；仅允许通知类，避免失败事件触发命令执行
				AllowedTools:   []string{},
				RequireConfirm: true,
				MinPriority:    5,
			},
			{
				Name:              "system_alert",
				EventType:         EventSystemAlert,
				Enabled:           true,
				QuietHoursStart:   0,
				QuietHoursEnd:     0,
				MaxActionsPerHour: 10,
				AllowedTools:      []string{"windows", "system"},
				RequireConfirm:    true,
				MinPriority:       1,
			},
			{
				Name:              "webhook_research",
				EventType:         EventWebhookReceived,
				Enabled:           true,
				QuietHoursStart:   23,
				QuietHoursEnd:     8,
				MaxActionsPerHour: 2,
				AllowedTools:      []string{"browser"},
				RequireConfirm:    true,
				MinPriority:       7,
			},
		},
	}
}

// Evaluate 评估事件是否应该触发行动
func (e *PolicyEngine) Evaluate(event Event) (bool, string, *Policy) {
	for _, policy := range e.policies {
		if !policy.Enabled {
			continue
		}
		if policy.EventType != event.Type {
			continue
		}
		if event.Priority < policy.MinPriority {
			continue
		}

		if policy.QuietHoursStart != policy.QuietHoursEnd {
			hour := time.Now().Hour()
			if policy.QuietHoursStart > policy.QuietHoursEnd {
				if hour >= policy.QuietHoursStart || hour < policy.QuietHoursEnd {
					return false, "quiet_hours", &policy
				}
			} else {
				if hour >= policy.QuietHoursStart && hour < policy.QuietHoursEnd {
					return false, "quiet_hours", &policy
				}
			}
		}

		if policy.MaxActionsPerHour > 0 {
			if e.countRecentActions(policy.Name, time.Hour) >= policy.MaxActionsPerHour {
				return false, "rate_limited", &policy
			}
		}

		if len(policy.Keywords) > 0 {
			payload := strings.ToLower(event.Payload)
			matched := false
			for _, kw := range policy.Keywords {
				if strings.Contains(payload, strings.ToLower(kw)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// 安全：需要确认的策略不得静默放行执行
		if policy.RequireConfirm {
			// 调用方（proactive/loop）必须尊重 returned policy.RequireConfirm 并进入确认流
			e.recordAction(policy.Name)
			return true, "approved_require_confirm", &policy
		}

		e.recordAction(policy.Name)
		return true, "approved", &policy
	}
	return false, "no_matching_policy", nil
}

func (e *PolicyEngine) countRecentActions(policyName string, within time.Duration) int {
	cutoff := time.Now().Add(-within)
	count := 0
	e.mu.Lock()
	for _, a := range e.actionLog {
		if a.policy == policyName && a.timestamp.After(cutoff) {
			count++
		}
	}
	e.mu.Unlock()
	return count
}

func (e *PolicyEngine) recordAction(policyName string) {
	e.mu.Lock()
	e.actionLog = append(e.actionLog, actionRecord{
		timestamp: time.Now(),
		policy:    policyName,
	})
	if len(e.actionLog) > 100 {
		e.actionLog = e.actionLog[len(e.actionLog)-100:]
	}
	e.mu.Unlock()
}

// GetActivePolicies 获取启用的策略
func (e *PolicyEngine) GetActivePolicies() []Policy {
	var active []Policy
	for _, p := range e.policies {
		if p.Enabled {
			active = append(active, p)
		}
	}
	return active
}
