package contextmgr

import (
	"fmt"
	"strings"

	"agent/internal/llm"
)

// Manager 上下文窗口管理器。
// 职责：管理消息历史的 token 预算，防止 context window 溢出。
// 灵感来自 OpenClaw context-engine 的分层预算管理。
type Manager struct {
	estimator *Estimator
	maxSteps  int // 历史消息最大条数（硬上限，防极端情况）
}

// NewManager 创建上下文管理器。
func NewManager(maxTokens int) *Manager {
	return &Manager{
		estimator: NewEstimator(maxTokens),
		maxSteps:  50, // 硬上限
	}
}

// SetMaxTokens 动态调整上下文窗口大小（如切换模型时）。
func (m *Manager) SetMaxTokens(maxTokens int) {
	m.estimator = NewEstimator(maxTokens)
}

// Budget 返回当前可用于消息历史的 token 预算。
func (m *Manager) Budget() int {
	return m.estimator.AvailableForHistory()
}

// FitMessages 将消息列表裁剪到上下文预算内。
// 策略：
//  1. 保留 system message（第一条，如果 role=system）
//  2. 保留最近的消息（越新越重要）
//  3. 中间部分按需丢弃最旧的
//  4. 如果总 token 仍然超限，从最旧的非 system 消息开始截断内容
func (m *Manager) FitMessages(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	budget := m.estimator.AvailableForHistory()

	// 分离 system 和非 system 消息
	var systemMsgs []llm.Message
	var convMsgs []llm.Message
	for _, msg := range msgs {
		if msg.Role == "system" {
			systemMsgs = append(systemMsgs, msg)
		} else {
			convMsgs = append(convMsgs, msg)
		}
	}

	// 估算 system messages 的 token 开销
	systemTokens := 0
	for _, sm := range systemMsgs {
		systemTokens += EstimateTokens(sm.Content) + 4
	}

	remaining := budget - systemTokens
	if remaining < 256 {
		// system 已经占了大部分，只保留 system
		return systemMsgs
	}

	// 从最新消息开始往回，贪心地保留尽可能多的消息
	selected := make([]llm.Message, 0, len(convMsgs))
	usedTokens := 0
	for i := len(convMsgs) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(convMsgs[i].Content) + 4
		// 加上 tool_calls 的 token 估算（如果有）
		// ToolCalls 在 Message 中没有直接存储，但 Content 通常已包含信息
		if usedTokens+msgTokens > remaining {
			break
		}
		usedTokens += msgTokens
		selected = append(selected, convMsgs[i])
	}

	// 反转 selected 使其按时间顺序
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	// 硬上限检查
	if len(selected) > m.maxSteps {
		selected = selected[len(selected)-m.maxSteps:]
	}

	// 如果丢弃了消息，在选中的最早消息前插入提示
	droppedCount := len(convMsgs) - len(selected)
	result := make([]llm.Message, 0, len(systemMsgs)+len(selected)+1)
	result = append(result, systemMsgs...)

	if hint := SummarizeHint(droppedCount); hint != "" {
		result = append(result, llm.Message{
			Role:    "system",
			Content: hint,
		})
	}

	result = append(result, selected...)
	return result
}

// FitMessagesWithTools 带工具定义的版本，同时裁剪工具定义到合理数量。
func (m *Manager) FitMessagesWithTools(msgs []llm.Message, tools []llm.ToolDef) ([]llm.Message, []llm.ToolDef) {
	fittedMsgs := m.FitMessages(msgs)

	// 工具定义通常不大（~100 tokens/工具），但如果工具有很多也需裁剪
	if len(tools) > 20 {
		// 保留前 20 个工具（最常用的）
		tools = tools[:20]
	}

	return fittedMsgs, tools
}

// EstimateConversationTokens 估算完整对话的 token 数。
func (m *Manager) EstimateConversationTokens(msgs []llm.Message) int {
	total := 0
	for _, msg := range msgs {
		total += EstimateTokens(msg.Content) + 4
	}
	return total
}

// FitForPlanner 专门为 Planner 裁剪上下文。
// Planner 的 system prompt 很长（含工具描述），需要更多预算给 system。
func (m *Manager) FitForPlanner(goal string, memory string, tools []llm.ToolDef) (string, string) {
	budget := m.estimator.AvailableForHistory()

	// 估算工具描述的 token
	toolTokens := 0
	for _, t := range tools {
		toolTokens += EstimateTokens(t.Name) + EstimateTokens(t.Description) + 8
	}

	// 估算 goal 的 token
	goalTokens := EstimateTokens(goal)

	// 剩余给 memory
	memoryBudget := budget - toolTokens - goalTokens - 100 // -100 for overhead
	if memoryBudget <= 0 {
		return goal, "" // 内存放不下了
	}

	// 截断 memory 到预算
	if EstimateTokens(memory) > memoryBudget {
		// 粗略截断：按比例保留
		runes := []rune(memory)
		keepRatio := float64(memoryBudget) / float64(EstimateTokens(memory))
		keepChars := int(float64(len(runes)) * keepRatio)
		if keepChars < 50 {
			return goal, ""
		}
		memory = string(runes[:keepChars]) + "\n...[记忆已截断]"
	}

	return goal, memory
}

// FitForEvaluator 专门为 Evaluator 裁剪上下文。
// Evaluator 需要看步骤输出，但输出可能很大（如 shell 输出）。
func (m *Manager) FitForEvaluator(description, tool, output string, maxOutputTokens int) (string, string, string) {
	budget := m.estimator.AvailableForHistory()

	// 预估 description + tool 的 token
	headerTokens := EstimateTokens(description) + EstimateTokens(tool) + 20

	// 输出的预算
	outputBudget := budget - headerTokens - 100
	if outputBudget <= 0 {
		outputBudget = 256
	}

	// 如果有 maxOutputTokens 限制，取较小值
	if maxOutputTokens > 0 && outputBudget > maxOutputTokens {
		outputBudget = maxOutputTokens
	}

	// 截断输出
	if EstimateTokens(output) > outputBudget {
		runes := []rune(output)
		keepRatio := float64(outputBudget) / float64(EstimateTokens(output))
		keepChars := int(float64(len(runes)) * keepRatio)
		if keepChars < 100 {
			keepChars = 100
		}
		output = string(runes[:keepChars]) + fmt.Sprintf("\n...[输出已截断，原始 %d 字符]", len(runes))
	}

	return description, tool, output
}

// FormatContextReport 生成上下文使用报告（用于日志/调试）。
func (m *Manager) FormatContextReport(msgs []llm.Message) string {
	total := m.EstimateConversationTokens(msgs)
	budget := m.estimator.AvailableForHistory()
	usedPercent := 0
	if budget > 0 {
		usedPercent = total * 100 / (budget + m.estimator.ReservedForSystem + m.estimator.ReservedForOutput)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Context: %d/%d tokens (%d%%)", total, m.estimator.MaxTokens, usedPercent))
	sb.WriteString(fmt.Sprintf(" | system≈%d", m.estimator.ReservedForSystem))
	sb.WriteString(fmt.Sprintf(" | history≈%d/%d", total, budget))
	sb.WriteString(fmt.Sprintf(" | output≈%d", m.estimator.ReservedForOutput))
	sb.WriteString(fmt.Sprintf(" | msgs=%d", len(msgs)))

	return sb.String()
}
