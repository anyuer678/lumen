// Package contextmgr 提供上下文窗口管理能力：token 估算、消息历史压缩、预算控制。
// 灵感来自 OpenClaw context-engine 和 Reasonix boundedllm 的分层管理思想。
package contextmgr

import (
	"strings"
	"unicode"
)

// Estimator 无外部依赖的 token 估算器。
// 对于中文为主的混合文本，误差通常在 ±15% 以内，足以做预算管理。
type Estimator struct {
	// MaxTokens 模型的上下文窗口大小
	MaxTokens int
	// ReservedForSystem 预留给 system prompt + 工具定义的 token 数
	ReservedForSystem int
	// ReservedForOutput 预留给模型输出的 token 数
	ReservedForOutput int
}

// NewEstimator 创建估算器，适用于典型中文 Agent 场景。
// 默认为 8K 上下文窗口（GLM-4-Flash / DeepSeek-Chat），预留 2K system + 2K output。
func NewEstimator(maxTokens int) *Estimator {
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	return &Estimator{
		MaxTokens:         maxTokens,
		ReservedForSystem: 2048,
		ReservedForOutput: 2048,
	}
}

// AvailableForHistory 返回可用于消息历史的 token 预算。
func (e *Estimator) AvailableForHistory() int {
	avail := e.MaxTokens - e.ReservedForSystem - e.ReservedForOutput
	if avail < 512 {
		return 512 // 最少保留 512 给历史
	}
	return avail
}

// EstimateTokens 估算一段文本的 token 数。
// 规则：
//   - 中文字符：~1.5 token/字（CLIP tokenizer 经验值）
//   - ASCII 单词：~1.3 token/word（GPT 系列经验值）
//   - 标点/空格/换行：~0.5 token/个
//   - JSON 结构字符（{}[]:,）：~0.3 token/个
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	tokens := 0
	runes := []rune(text)
	asciiWordLen := 0

	for i, r := range runes {
		switch {
		case r >= 0x4e00 && r <= 0x9fff: // CJK 统一汉字
			// 结算之前的 ASCII word
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4 // ~4 chars per token
				asciiWordLen = 0
			}
			tokens += 2 // 中文字符 ~1.5-2 token，取 2 保安全

		case r >= 0x3040 && r <= 0x309f || r >= 0x30a0 && r <= 0x30ff: // 日文假名
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4
				asciiWordLen = 0
			}
			tokens += 2

		case r >= 0xac00 && r <= 0xd7af: // 韩文
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4
				asciiWordLen = 0
			}
			tokens += 2

		case r >= 0x2000 && r <= 0x2bff: // 标点符号区域
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4
				asciiWordLen = 0
			}
			tokens++

		case unicode.IsSpace(r):
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4
				asciiWordLen = 0
			}
			// 连续空格合并
			if i > 0 && unicode.IsSpace(runes[i-1]) {
				continue
			}
			tokens++

		case r == '{' || r == '}' || r == '[' || r == ']' || r == ',' || r == ':':
			// JSON 结构字符较轻
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4
				asciiWordLen = 0
			}
			tokens++

		case r < 128:
			// ASCII 字符（字母/数字/符号）
			asciiWordLen++

		default:
			// 其他 Unicode
			if asciiWordLen > 0 {
				tokens += (asciiWordLen + 3) / 4
				asciiWordLen = 0
			}
			tokens += 2
		}
	}

	// 结尾的 ASCII word
	if asciiWordLen > 0 {
		tokens += (asciiWordLen + 3) / 4
	}

	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

// EstimateMessages 估算一条消息的 token 开销（含 role overhead）。
func EstimateMessages(msgs []struct {
	Role    string
	Content string
}) int {
	total := 0
	for _, m := range msgs {
		// role 标记 + 分隔符 ~4 tokens
		total += 4
		total += EstimateTokens(m.Content)
	}
	return total
}

// EstimateMessagesLLM 估算 llm.Message 数组的 token 开销。
func EstimateMessagesLLM(msgs []interface{ GetContent() string }) int {
	total := 0
	for _, m := range msgs {
		total += 4
		total += EstimateTokens(m.GetContent())
	}
	return total
}

// TruncateToBudget 截断消息列表到 token 预算内。
// 保留 system message（第一条）和最近的消息，中间按需丢弃最旧的。
// 返回截断后的消息索引范围 [start, end)。
func TruncateToBudget(texts []string, roles []string, budget int) (start, end int) {
	n := len(texts)
	if n == 0 {
		return 0, 0
	}

	// 先算 system message 的 token（如果第一条是 system）
	systemTokens := 0
	startIdx := 0
	if len(roles) > 0 && roles[0] == "system" {
		systemTokens = EstimateTokens(texts[0])
		startIdx = 1
	}

	remaining := budget - systemTokens
	if remaining <= 0 {
		// system 已经超预算，只保留 system
		return 0, 1
	}

	// 从最新的消息开始往回塞
	totalUsed := 0
	end = n
	for i := n - 1; i >= startIdx; i-- {
		msgTokens := EstimateTokens(texts[i]) + 4 // +4 for role overhead
		if totalUsed+msgTokens > remaining {
			break
		}
		totalUsed += msgTokens
		end = i
	}

	// 如果是 system message 开头，保留 system + 从 end 到末尾
	if startIdx > 0 {
		return 0, end  // system 保留
	}
	return end, n
}

// NeedsTruncation 检查消息列表是否需要截断。
func NeedsTruncation(texts []string, roles []string, budget int) bool {
	total := 0
	for i, text := range texts {
		total += EstimateTokens(text) + 4
		if len(roles) > i && roles[i] == "system" {
			// system message 不计入可丢弃部分
			continue
		}
		if total > budget {
			return true
		}
	}
	return total > budget
}

// SummarizeHint 生成一个提示，告诉 LLM 哪些历史被截断了。
func SummarizeHint(droppedCount int) string {
	if droppedCount <= 0 {
		return ""
	}
	return strings.TrimSpace(`
[上下文管理] 为避免上下文溢出，已省略最近 ` + strings.TrimSpace(runeToString(droppedCount)) + ` 条历史消息的详细内容。
请基于当前可见的上下文继续工作。如需更多信息，用户可以重新提及。`)
}

func runeToString(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	digits := "0123456789"
	for n > 0 {
		result = string(digits[n%10]) + result
		n /= 10
	}
	return result
}
