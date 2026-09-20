package auth

import (
	"fmt"
	"sync"
)

// PermissionLevel 权限等级
type PermissionLevel int

const (
	Level0ReadOnly   PermissionLevel = 0 // 只读
	Level1Normal     PermissionLevel = 1 // 普通操作
	Level2Dangerous  PermissionLevel = 2 // 危险操作（需确认）
	Level3Critical   PermissionLevel = 3 // 高危操作（需确认+二次认证）
)

// PermissionDecision 权限判定结果
type PermissionDecision struct {
	Allowed     bool            `json:"allowed"`
	NeedConfirm bool            `json:"need_confirm"`
	Level       PermissionLevel `json:"level"`
	Reason      string          `json:"reason"`
}

// Policy 权限策略
type Policy struct {
	Pattern string          `json:"pattern"`
	Level   PermissionLevel `json:"level"`
}

// PermissionEngine 权限引擎
type PermissionEngine struct {
	policies []Policy
	mu       sync.RWMutex
}

// NewPermissionEngine 创建权限引擎
func NewPermissionEngine() *PermissionEngine {
	return &PermissionEngine{
		policies: defaultPolicies(),
	}
}

// defaultPolicies 默认策略。
// 安全硬化：
// - shell:run 默认 L2（需确认），避免 L1 token 静默开 shell
// - fs 只读 L0；写 L1；delete/organize L2
// - 未命中策略 fail-closed 为 L2
func defaultPolicies() []Policy {
	return []Policy{
		{Pattern: "shell:*", Level: Level2Dangerous},
		{Pattern: "shell:run", Level: Level2Dangerous},
		{Pattern: "shell:install", Level: Level3Critical},
		{Pattern: "shell:admin", Level: Level3Critical},
		{Pattern: "fs:read", Level: Level0ReadOnly},
		{Pattern: "fs:list", Level: Level0ReadOnly},
		{Pattern: "fs:exists", Level: Level0ReadOnly},
		{Pattern: "fs:write", Level: Level1Normal},
		{Pattern: "fs:mkdir", Level: Level1Normal},
		{Pattern: "fs:organize", Level: Level2Dangerous},
		{Pattern: "fs:delete", Level: Level2Dangerous}, // os.RemoveAll
		{Pattern: "fs:*", Level: Level1Normal},
		{Pattern: "browser:*", Level: Level0ReadOnly},
		{Pattern: "browser:download", Level: Level1Normal},
		{Pattern: "system:*", Level: Level1Normal},
		{Pattern: "windows:*", Level: Level1Normal},
		{Pattern: "windows:launch", Level: Level2Dangerous},
		{Pattern: "computer:*", Level: Level2Dangerous},
		{Pattern: "mcp:*", Level: Level2Dangerous},
		{Pattern: "subagent", Level: Level1Normal},
		{Pattern: "safety:*", Level: Level0ReadOnly},
	}
}

// Check 检查权限
func (e *PermissionEngine) Check(tool string, userLevel PermissionLevel) PermissionDecision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matched *Policy
	for i := range e.policies {
		p := &e.policies[i]
		if matchPattern(tool, p.Pattern) {
			if matched == nil || len(p.Pattern) > len(matched.Pattern) {
				matched = p
			}
		}
	}
	var matchedLevel PermissionLevel = Level2Dangerous // 默认 fail-closed
	if matched != nil {
		matchedLevel = matched.Level
	}

	if userLevel >= matchedLevel {
		return PermissionDecision{
			Allowed:     true,
			NeedConfirm: false,
			Level:       matchedLevel,
			Reason:      "permission granted",
		}
	}

	if matchedLevel >= Level2Dangerous {
		return PermissionDecision{
			Allowed:     false,
			NeedConfirm: true,
			Level:       matchedLevel,
			Reason:      fmt.Sprintf("tool %s requires level %d, user has level %d", tool, matchedLevel, userLevel),
		}
	}

	return PermissionDecision{
		Allowed:     false,
		NeedConfirm: false,
		Level:       matchedLevel,
		Reason:      fmt.Sprintf("permission denied: tool %s requires level %d, user has level %d", tool, matchedLevel, userLevel),
	}
}

// AddPolicy 添加策略
func (e *PermissionEngine) AddPolicy(policy Policy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies = append(e.policies, policy)
}

// matchPattern 匹配模式
func matchPattern(tool, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(tool) >= len(prefix) && tool[:len(prefix)] == prefix
	}
	return tool == pattern
}
