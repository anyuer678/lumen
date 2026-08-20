// Package toolrepair 提供工具调用的宽容解析能力。
// 灵感来自 OpenClaw tool-call-repair：从 LLM 文本中宽容提取工具调用、
// 修复常见 JSON 问题（截断、尾逗号、未转义字符等），坏块跳过而非整体失败。
package toolrepair

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RepairResult 修复结果
type RepairResult struct {
	// JSON 是否修复成功
	Fixed bool
	// 修复后的 JSON 字符串
	FixedJSON string
	// 修复描述
	Fixes []string
}

// RepairJSON 修复常见的 LLM JSON 输出问题。
// 返回修复后的 JSON 字符串和修复描述列表。
func RepairJSON(raw string) RepairResult {
	result := RepairResult{FixedJSON: raw}
	if raw == "" {
		return result
	}

	input := raw

	// 1. 提取 markdown 代码块中的 JSON
	if fixed, ok := extractFromCodeBlock(input); ok {
		result.Fixes = append(result.Fixes, "extracted from markdown code block")
		input = fixed
	}

	// 2. 去除前后多余文本（找到第一个 { 和最后一个 }）
	if fixed, ok := extractJSONBounds(input); ok {
		if fixed != input {
			result.Fixes = append(result.Fixes, "extracted JSON from surrounding text")
			input = fixed
		}
	}

	// 3. 修复尾逗号 (trailing commas)
	if fixed, ok := fixTrailingCommas(input); ok {
		result.Fixes = append(result.Fixes, "removed trailing commas")
		input = fixed
	}

	// 4. 修复截断的 JSON（补全未闭合的括号）
	if fixed, ok := fixTruncatedJSON(input); ok {
		result.Fixes = append(result.Fixes, "completed truncated JSON brackets")
		input = fixed
	}

	// 5. 去除控制字符
	if fixed, ok := cleanControlChars(input); ok {
		result.Fixes = append(result.Fixes, "cleaned control characters")
		input = fixed
	}

	// 验证修复后的 JSON 是否有效
	var test json.RawMessage
	if err := json.Unmarshal([]byte(input), &test); err == nil {
		result.FixedJSON = input
		result.Fixed = true
	}

	return result
}

// extractFromCodeBlock 从 markdown 代码块中提取 JSON。
func extractFromCodeBlock(s string) (string, bool) {
	// ```json ... ```
	if idx := strings.Index(s, "```json"); idx != -1 {
		rest := s[idx+7:]
		if endIdx := strings.Index(rest, "```"); endIdx != -1 {
			return strings.TrimSpace(rest[:endIdx]), true
		}
		return strings.TrimSpace(rest), true
	}
	// ``` ... ```
	if idx := strings.Index(s, "```"); idx != -1 {
		rest := s[idx+3:]
		if endIdx := strings.Index(rest, "```"); endIdx != -1 {
			return strings.TrimSpace(rest[:endIdx]), true
		}
		return strings.TrimSpace(rest), true
	}
	return s, false
}

// extractJSONBounds 从文本中提取 JSON 对象（找到最外层的 {} 或 []）。
func extractJSONBounds(s string) (string, bool) {
	// 找第一个 { 或 [
	start := -1
	for i, c := range s {
		if c == '{' || c == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return s, false
	}

	// 找最后一个 } 或 ]
	end := -1
	openChar := s[start]
	closeChar := byte('}')
	if openChar == '[' {
		closeChar = ']'
	}
	for i := len(s) - 1; i > start; i-- {
		if s[i] == closeChar {
			end = i
			break
		}
	}
	if end == -1 || end <= start {
		return s, false
	}

	extracted := s[start : end+1]
	if extracted != s {
		return extracted, true
	}
	return s, false
}

// fixTrailingCommas 修复 JSON 中的尾逗号。
// {"a": 1,} → {"a": 1}
// [1, 2,] → [1, 2]
func fixTrailingCommas(s string) (string, bool) {
	// 匹配 , 后跟 ] 或 } （可选空格/换行）
	re := regexp.MustCompile(`,\s*([\]}])`)
	fixed := re.ReplaceAllString(s, "$1")
	if fixed != s {
		return fixed, true
	}
	return s, false
}

// fixTruncatedJSON 修复被截断的 JSON（补全未闭合的括号和引号）。
func fixTruncatedJSON(s string) (string, bool) {
	if s == "" {
		return s, false
	}

	original := s

	// 统计未闭合的括号
	stack := []byte{}
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{', '[':
			stack = append(stack, c)
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// 如果字符串在引号内被截断，先闭合引号
	if inString {
		s += `"`
	}

	// 反向补全未闭合的括号
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			s += "}"
		} else if stack[i] == '[' {
			s += "]"
		}
	}

	if s != original {
		return s, true
	}
	return s, false
}

// cleanControlChars 清理 JSON 字符串值中的非法控制字符。
func cleanControlChars(s string) (string, bool) {
	// 只清理字符串值中的控制字符（保留 \n \t \r 等转义序列）
	var result strings.Builder
	inString := false
	escaped := false
	changed := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			result.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && inString {
			result.WriteByte(c)
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			result.WriteByte(c)
			continue
		}
		if inString && c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			// 替换非法控制字符为空格
			result.WriteByte(' ')
			changed = true
			continue
		}
		result.WriteByte(c)
	}

	if changed {
		return result.String(), true
	}
	return s, false
}

// ExtractToolCallsFromText 从 LLM 纯文本回复中提取工具调用。
// 支持格式：
//   - <tool_call>{"name": "shell.run", "args": {"command": "ls"}}</tool_call>
//   - {"tool_call": {"name": "...", "arguments": {...}}}
//   - 散落在文本中的 JSON 块
func ExtractToolCallsFromText(text string) []map[string]any {
	var calls []map[string]any

	// 1. 提取 <tool_call>...</tool_call> 围栏
	re := regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	matches := re.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			var call map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(match[1])), &call); err == nil {
				calls = append(calls, call)
			}
		}
	}

	// 2. 提取 function_call 或 tool_call 格式的 JSON
	re2 := regexp.MustCompile(`"(?:function_call|tool_call)":\s*(\{[^}]+\})`)
	matches2 := re2.FindAllStringSubmatch(text, -1)
	for _, match := range matches2 {
		if len(match) > 1 {
			var call map[string]any
			if err := json.Unmarshal([]byte(match[1]), &call); err == nil {
				calls = append(calls, call)
			}
		}
	}

	return calls
}

// RepairToolName 修复 LLM 可能写歪的工具名。
// 支持：
//   - "windows.powershell" → "windows"
//   - "system.disk" → "system"
//   - "shell" → "shell.run"（如果只有 "shell" 这个前缀）
//   - 模糊匹配：去空格、小写化
func RepairToolName(name string, knownTools map[string]bool) string {
	name = strings.TrimSpace(name)
	// 直接匹配
	if knownTools[name] {
		return name
	}
	lower := strings.ToLower(name)
	if knownTools[lower] {
		return lower
	}

	// 点号分割取基础名
	if idx := strings.IndexByte(name, '.'); idx > 0 {
		base := name[:idx]
		if knownTools[base] {
			return base
		}
		if knownTools[strings.ToLower(base)] {
			return strings.ToLower(base)
		}
	}

	// 模糊匹配：查找已知工具中包含该名称的
	for tool := range knownTools {
		if strings.Contains(tool, lower) || strings.Contains(lower, tool) {
			return tool
		}
	}

	return name
}

// RepairToolArgs 修复 LLM 可能填错的工具参数。
// 常见问题：参数值类型错误（字符串 vs 数字）、缺少必填参数。
func RepairToolArgs(toolName string, args map[string]any, schema map[string]any) map[string]any {
	if args == nil {
		args = make(map[string]any)
	}

	// 确保 timeout 是数字
	if v, ok := args["timeout"]; ok {
		if s, ok := v.(string); ok {
			// 尝试转换字符串到数字
			if n, err := parseInt(s); err == nil {
				args["timeout"] = n
			}
		}
	}

	// 确保 command 不为空（shell.run 的必填参数）
	if toolName == "shell.run" {
		if cmd, ok := args["command"]; ok {
			if cmdStr, ok := cmd.(string); ok && cmdStr == "" {
				args["command"] = "echo 'empty command'"
			}
		}
	}

	// 确保 action 不为空（多操作工具的必填参数）
	if _, ok := args["action"]; !ok {
		switch toolName {
		case "fs":
			args["action"] = "list"
		case "browser":
			args["action"] = "open"
		case "system":
			args["action"] = "processes"
		case "windows":
			args["action"] = "powershell"
		}
	}

	// browser open 缺 url 时补默认值（避免 LLM 参数不全导致失败）
	if toolName == "browser" {
		if action, _ := args["action"].(string); action == "open" {
			if u, _ := args["url"].(string); u == "" {
				args["url"] = "https://www.example.com"
			}
		}
	}

	return args
}

// parseInt 简单的字符串转整数（不依赖 strconv）。
func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid digit: %c", c)
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}
