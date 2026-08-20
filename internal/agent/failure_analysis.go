package agent

import (
	"fmt"
	"strings"
)

// FailureCategory 失败类别
type FailureCategory string

const (
	FailPlanner    FailureCategory = "planner"     // Planner 生成错误
	FailToolSelect FailureCategory = "tool_select"  // 工具选择错误
	FailToolExec   FailureCategory = "tool_exec"    // 工具执行失败
	FailParam      FailureCategory = "param"        // 参数错误
	FailRecovery   FailureCategory = "recovery"     // 恢复失败
	FailEvaluator  FailureCategory = "evaluator"    // 评估错误
	FailTimeout    FailureCategory = "timeout"      // 超时
	FailSandbox    FailureCategory = "sandbox"      // 沙箱拦截
	FailNetwork    FailureCategory = "network"      // 网络错误
	FailUnknown    FailureCategory = "unknown"      // 未知
)

// FailureAnalysis 单个失败的分析
type FailureAnalysis struct {
	TaskID    string           `json:"task_id"`
	Name      string           `json:"name"`
	Category  FailureCategory `json:"category"`
	Reason    string           `json:"reason"`
	Suggestion string          `json:"suggestion"`
	Error     string           `json:"error"`
}

// FailureReport 失败分析报告
type FailureReport struct {
	Total      int                `json:"total"`
	Failed     int                `json:"failed"`
	ByCategory map[string]int     `json:"by_category"`
	Analyses   []FailureAnalysis `json:"analyses"`
	Summary    string             `json:"summary"`
}

// AnalyzeFailures 分析 Benchmark 结果中的失败
func AnalyzeFailures(results []BenchResultV2) *FailureReport {
	report := &FailureReport{
		Total:      len(results),
		ByCategory: make(map[string]int),
	}

	for _, r := range results {
		if r.Status == "pass" {
			continue
		}
		report.Failed++

		analysis := FailureAnalysis{
			TaskID: r.TaskID,
			Name:   r.Name,
			Error:  r.Error,
		}

		// 分类失败原因
		analysis.Category, analysis.Reason = categorizeFailure(r)
		analysis.Suggestion = suggestFix(analysis.Category, analysis.Reason)

		report.Analyses = append(report.Analyses, analysis)
		report.ByCategory[string(analysis.Category)]++
	}

	// 生成摘要
	report.Summary = generateSummary(report)

	return report
}

// categorizeFailure 分类失败原因
func categorizeFailure(r BenchResultV2) (FailureCategory, string) {
	err := strings.ToLower(r.Error)

	// 超时
	if strings.Contains(err, "timeout") || strings.Contains(err, "deadline") {
		return FailTimeout, "任务执行超时"
	}

	// 网络错误
	if strings.Contains(err, "connection") || strings.Contains(err, "network") || strings.Contains(err, "i/o timeout") {
		return FailNetwork, "网络连接失败"
	}

	// 沙箱拦截
	if strings.Contains(err, "sandbox") || strings.Contains(err, "安全策略阻止") {
		return FailSandbox, "沙箱安全策略拦截"
	}

	// 工具不存在
	if strings.Contains(err, "tool not found") || strings.Contains(err, "unknown tool") {
		return FailToolSelect, "选择了不存在的工具"
	}

	// 工具执行失败
	if strings.Contains(err, "tool") && strings.Contains(err, "execute") {
		return FailToolExec, "工具执行出错"
	}

	// 工具参数错误
	if strings.Contains(err, "required") || strings.Contains(err, "is required") {
		return FailParam, "缺少必填参数"
	}

	// Planner 错误
	if strings.Contains(err, "plan") || strings.Contains(err, "json") || strings.Contains(err, "parse") {
		return FailPlanner, "LLM 输出解析失败"
	}

	// 评估错误
	if strings.Contains(err, "evaluator") || strings.Contains(err, "review") {
		return FailEvaluator, "评估器判断异常"
	}

	// 未知错误
	return FailUnknown, err
}

// suggestFix 根据失败类别生成修复建议
func suggestFix(category FailureCategory, reason string) string {
	suggestions := map[FailureCategory]string{
		FailPlanner:    "优化 Planner prompt，增加 JSON 格式示例；启用 toolrepair 宽容解析",
		FailToolSelect: "检查工具注册是否完整；在 prompt 中明确列出可用工具",
		FailToolExec:   "检查工具实现；增加错误重试；优化错误信息",
		FailParam:      "在 prompt 中明确参数要求；启用 RepairToolArgs 自动修复",
		FailRecovery:   "检查 Replanner 逻辑；增加恢复策略",
		FailEvaluator:  "调整 Evaluator prompt；增加容错逻辑",
		FailTimeout:    "增加超时时间；优化工具执行效率",
		FailSandbox:    "检查沙箱规则是否过于严格；调整安全策略",
		FailNetwork:    "检查网络连接；增加重试机制",
		FailUnknown:    "需要人工分析具体错误原因",
	}

	if suggestion, ok := suggestions[category]; ok {
		return suggestion
	}
	return "需要进一步分析"
}

// generateSummary 生成失败分析摘要
func generateSummary(report *FailureReport) string {
	if report.Failed == 0 {
		return "🎉 全部通过，无需修复"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个失败，分类如下：\n\n", report.Failed))

	// 按数量排序
	type catCount struct {
		cat   string
		count int
	}
	var sorted []catCount
	for cat, count := range report.ByCategory {
		sorted = append(sorted, catCount{cat, count})
	}
	// 简单冒泡排序
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	for _, cc := range sorted {
		sb.WriteString(fmt.Sprintf("- %s: %d 个\n", cc.cat, cc.count))
	}

	// 最常见的失败
	if len(sorted) > 0 {
		sb.WriteString(fmt.Sprintf("\n最大问题：%s（%d 个）→ %s\n",
			sorted[0].cat, sorted[0].count,
			suggestFix(FailureCategory(sorted[0].cat), "")))
	}

	return sb.String()
}
