package llm

import (
	"strings"
	"text/template"
)

// PromptTemplate prompt 模板
type PromptTemplate struct {
	System    string
	User      string
	Tools     string
	Memory    string
	ToolResult string
}

// PlannerPrompt Planner 的 prompt 模板
var PlannerPrompt = PromptTemplate{
	System: `你是一个电脑操控 Agent 的规划器。
你的任务是把用户目标拆解成可执行步骤。

可用工具：
{{.Tools}}

规则：
1. 优先使用底层能力：Shell > 文件系统 > 浏览器 > Computer Use
2. 一步只做一件事，步骤粒度要可验证
3. 若需要用户决策（如删除文件），单独注明 confirmation_needed: true
4. 不编造不存在的工具名
5. 输出严格的 JSON 格式

{{if .Memory}}
用户记忆：
{{.Memory}}
{{end}}`,

	User: `用户目标: {{.Goal}}

请生成执行计划。输出 JSON 格式:
{
  "steps": [
    {
      "description": "步骤描述（含预期结果）",
      "tool": "工具名",
      "args": { ... },
      "max_retries": 2
    }
  ],
  "estimated_steps": 5,
  "risks": ["可能的风险点"]
}`,
}

// EvaluatorPrompt Evaluator 的 prompt 模板
var EvaluatorPrompt = PromptTemplate{
	System: `你是一个电脑操作结果的评估器。
判断步骤是否成功。`,

	User: `步骤目标: {{.Description}}
工具: {{.Tool}}
工具输出:
{{.Output}}

输出 JSON:
{"success": true/false, "reason": "简短原因", "next_step_suggestion": "如果失败，下一步建议"}`,
}

// ReplannerPrompt Replanner 的 prompt 模板
var ReplannerPrompt = PromptTemplate{
	System: `你是一个电脑操控 Agent 的计划修正器。
前面的步骤执行失败了，请修正剩余计划。`,

	User: `用户目标: {{.Goal}}
失败步骤: {{.FailedStep}}
失败原因: {{.Error}}
剩余计划: {{.RemainingSteps}}
最新观察: {{.Observation}}

输出新的 JSON 计划（覆盖剩余部分），要求:
1. 分析失败根因，换一种方式达成同一目标
2. 若需用户介入，插入 confirmation_needed 步骤
3. 保持与已完成步骤的衔接`,
}

// SystemSummaryPrompt 系统消息摘要
var SystemSummaryPrompt = PromptTemplate{
	System: `你是一个任务执行的摘要器。
将任务执行结果压缩为简短摘要。`,

	User: `任务目标: {{.Goal}}
执行步骤:
{{.Steps}}

结果: {{.Result}}
错误: {{.Error}}

请输出一段 200 字以内的中文摘要，说明任务执行情况。`,
}

// Render 渲染模板
func (t PromptTemplate) Render(data map[string]interface{}) string {
	tmplStr := t.System + "\n\n" + t.User
	tmpl, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return tmplStr
	}

	var buf strings.Builder
	tmpl.Execute(&buf, data)
	return buf.String()
}

// RenderWithTools 渲染带工具定义的模板
func RenderPlannerPrompt(goal string, toolDefs []ToolDef, memory string) string {
	var toolDesc strings.Builder
	for _, t := range toolDefs {
		toolDesc.WriteString("- ")
		toolDesc.WriteString(t.Name)
		toolDesc.WriteString(": ")
		toolDesc.WriteString(t.Description)
		toolDesc.WriteString("\n")
	}

	data := map[string]interface{}{
		"Goal":   goal,
		"Tools":  toolDesc.String(),
		"Memory": memory,
	}

	tmpl, _ := template.New("p").Parse(PlannerPrompt.System + "\n\n" + PlannerPrompt.User)
	var buf strings.Builder
	tmpl.Execute(&buf, data)
	return buf.String()
}

// RenderEvaluatorPrompt 渲染 Evaluator prompt
func RenderEvaluatorPrompt(description, tool, output string) string {
	data := map[string]interface{}{
		"Description": description,
		"Tool":        tool,
		"Output":      output,
	}
	tmpl, _ := template.New("e").Parse(EvaluatorPrompt.System + "\n\n" + EvaluatorPrompt.User)
	var buf strings.Builder
	tmpl.Execute(&buf, data)
	return buf.String()
}

// RenderReplannerPrompt 渲染 Replanner prompt
func RenderReplannerPrompt(goal, failedStep, error, remainingSteps, observation string) string {
	data := map[string]interface{}{
		"Goal":           goal,
		"FailedStep":     failedStep,
		"Error":          error,
		"RemainingSteps": remainingSteps,
		"Observation":    observation,
	}
	tmpl, _ := template.New("r").Parse(ReplannerPrompt.System + "\n\n" + ReplannerPrompt.User)
	var buf strings.Builder
	tmpl.Execute(&buf, data)
	return buf.String()
}
