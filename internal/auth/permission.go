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
// 键为 tool[:action]（action 由调用方从 args 取出拼上），
// 与真实工具注册表对齐：fs/windows 是单工具多 action 分发。
func defaultPolicies() []Policy {
	return []Policy{
		{Pattern: "shell:*", Level: Level1Normal},
		{Pattern: "shell:install", Level: Level2Dangerous},
		{Pattern: "shell:admin", Level: Level3Critical},
		{Pattern: "fs:*", Level: Level1Normal},
		{Pattern: "fs:delete", Level: Level2Dangerous}, // os.RemoveAll
		{Pattern: "browser:*", Level: Level0ReadOnly},
		{Pattern: "browser:download", Level: Level1Normal},
		{Pattern: "system:*", Level: Level1Normal},
		{Pattern: "windows:*", Level: Level1Normal},
		{Pattern: "windows:launch", Level: Level2Dangerous}, // 启动外部程序
		{Pattern: "computer:*", Level: Level2Dangerous},     // 键鼠控制
		{Pattern: "mcp:*", Level: Level2Dangerous},
		{Pattern: "subagent", Level: Level1Normal},
		{Pattern: "safety:*", Level: Level0ReadOnly},
	}
}

// Check 检查权限
func (e *PermissionEngine) Check(tool string, userLevel PermissionLevel) PermissionDecision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 查找匹配的策略：最具体（最长 pattern）优先，
	// 否则 "fs:*"(L1) 会按声明顺序先于 "fs:delete"(L2) 命中，架空细化策略
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
