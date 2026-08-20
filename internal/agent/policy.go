package agent

import (
	"strings"
	"time"
)

// Policy 决策规则
type Policy struct {
	Name               string   `json:"name"`
	EventType          EventType `json:"event_type"`
	Enabled            bool     `json:"enabled"`
	QuietHoursStart    int      `json:"quiet_hours_start"` // 24h format
	QuietHoursEnd      int      `json:"quiet_hours_end"`
	MaxActionsPerHour  int      `json:"max_actions_per_hour"`
	AllowedTools       []string `json:"allowed_tools"`
	RequireConfirm     bool     `json:"require_confirm"`
	MinPriority        int      `json:"min_priority"`
	Keywords           []string `json:"keywords"` // 需要匹配的关键词
	MaxCostPerDay      float64  `json:"max_cost_per_day"`
}

// PolicyEngine 决策引擎
type PolicyEngine struct {
	policies  []Policy
	actionLog []actionRecord
}

type actionRecord struct {
	timestamp time.Time
	policy    string
	action    string
}

// NewPolicyEngine 创建决策引擎
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
				AllowedTools:      []string{"fs", "browser"},
				RequireConfirm:    false,
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
				AllowedTools:      []string{"shell.run"},
				RequireConfirm:    false,
				MinPriority:       5,
			},
			{
				Name:              "system_alert",
				EventType:         EventSystemAlert,
				Enabled:           true,
				QuietHoursStart:   0,
				QuietHoursEnd:     0,
				MaxActionsPerHour: 10,
				AllowedTools:      []string{"windows", "shell.run", "system"},
				RequireConfirm:    false,
				MinPriority:       1,
			},
			{
				Name:              "webhook_research",
				EventType:         EventWebhookReceived,
				Enabled:           true,
				QuietHoursStart:   23,
				QuietHoursEnd:     8,
				MaxActionsPerHour: 2,
				AllowedTools:      []string{"browser", "shell.run"},
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

		// 静默时段检查
		if policy.QuietHoursStart != policy.QuietHoursEnd {
			hour := time.Now().Hour()
			if policy.QuietHoursStart > policy.QuietHoursEnd {
				// 跨午夜：23-8
				if hour >= policy.QuietHoursStart || hour < policy.QuietHoursEnd {
					return false, "quiet_hours", &policy
				}
			} else {
				// 不跨午夜：8-22
				if hour >= policy.QuietHoursStart && hour < policy.QuietHoursEnd {
					return false, "quiet_hours", &policy
				}
			}
		}

		// 频率限制
		if policy.MaxActionsPerHour > 0 {
			if e.countRecentActions(policy.Name, time.Hour) >= policy.MaxActionsPerHour {
				return false, "rate_limited", &policy
			}
		}

		// 关键词匹配
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

		e.recordAction(policy.Name)
		return true, "approved", &policy
	}
	return false, "no_matching_policy", nil
}

func (e *PolicyEngine) countRecentActions(policyName string, within time.Duration) int {
	cutoff := time.Now().Add(-within)
	count := 0
	for _, a := range e.actionLog {
		if a.policy == policyName && a.timestamp.After(cutoff) {
			count++
		}
	}
	return count
}

func (e *PolicyEngine) recordAction(policyName string) {
	e.actionLog = append(e.actionLog, actionRecord{
		timestamp: time.Now(),
		policy:    policyName,
	})
	// 只保留最近 100 条
	if len(e.actionLog) > 100 {
		e.actionLog = e.actionLog[len(e.actionLog)-100:]
	}
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
