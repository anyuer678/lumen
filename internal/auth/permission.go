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

// defaultPolicies 默认策略
func defaultPolicies() []Policy {
	return []Policy{
		{Pattern: "shell:*", Level: Level1Normal},
		{Pattern: "shell:install", Level: Level2Dangerous},
		{Pattern: "shell:admin", Level: Level3Critical},
		{Pattern: "fs:*", Level: Level1Normal},
		{Pattern: "fs:delete", Level: Level2Dangerous},
		{Pattern: "browser:*", Level: Level0ReadOnly},
		{Pattern: "browser:download", Level: Level1Normal},
		{Pattern: "system:*", Level: Level2Dangerous},
		{Pattern: "system:critical", Level: Level3Critical},
		{Pattern: "mcp:*", Level: Level2Dangerous},
	}
}

// Check 检查权限
func (e *PermissionEngine) Check(tool string, userLevel PermissionLevel) PermissionDecision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 查找匹配的策略
	var matchedLevel PermissionLevel = Level2Dangerous // 默认 fail-closed
	for _, policy := range e.policies {
		if matchPattern(tool, policy.Pattern) {
			matchedLevel = policy.Level
			break
		}
	}

	// 判定
	if userLevel >= matchedLevel {
		return PermissionDecision{
			Allowed:     true,
			NeedConfirm: false,
			Level:       matchedLevel,
			Reason:      "permission granted",
		}
	}

	// 需要确认
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
	// 简化版：支持通配符
	if pattern == "*" {
		return true
	}

	// 支持 shell:* 格式
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(tool) >= len(prefix) && tool[:len(prefix)] == prefix
	}

	return tool == pattern
}
