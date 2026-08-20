package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"agent/internal/llm"
	"agent/internal/toolrepair"
)

// LLMEvaluator 基于 LLM 的评估器
type LLMEvaluator struct {
	provider llm.Provider
}

// NewLLMEvaluator 创建 LLM Evaluator
func NewLLMEvaluator(provider llm.Provider) *LLMEvaluator {
	return &LLMEvaluator{provider: provider}
}

// EvalResponse 评估结果
type EvalResponse struct {
	Success              bool   `json:"success"`
	Reason               string `json:"reason"`
	NextStepSuggestion   string `json:"next_step_suggestion"`
}

// Evaluate 评估步骤执行结果
func (e *LLMEvaluator) Evaluate(ctx context.Context, description, tool, output string) (*EvalResponse, error) {
	systemMsg := `你是一个电脑操作结果的评估器。
判断步骤是否成功。
输出严格的 JSON 格式。`

	userMsg := fmt.Sprintf(`步骤目标: %s
工具: %s
工具输出:
%s

输出 JSON:
{"success": true/false, "reason": "简短原因", "next_step_suggestion": "如果失败，下一步建议"}`,
		description, tool, output)

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}

	resp, err := e.provider.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM chat: %w", err)
	}

	return e.parseResponse(resp.Content)
}

func (e *LLMEvaluator) parseResponse(content string) (*EvalResponse, error) {
	// 使用 toolrepair 宽容修复 JSON
	repairResult := toolrepair.RepairJSON(content)
	if !repairResult.Fixed {
		return nil, fmt.Errorf("parse LLM response: JSON repair failed\nraw: %s", content)
	}

	var evalResp EvalResponse
	if err := json.Unmarshal([]byte(repairResult.FixedJSON), &evalResp); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w\nraw: %s", err, content)
	}
	return &evalResp, nil
}

// LLMReplanner 基于 LLM 的计划修正器
type LLMReplanner struct {
	provider llm.Provider
	tools    map[string]Tool
}

// NewLLMReplanner 创建 LLM Replanner
func NewLLMReplanner(provider llm.Provider, tools map[string]Tool) *LLMReplanner {
	return &LLMReplanner{provider: provider, tools: tools}
}

// Replan 修正剩余计划
func (r *LLMReplanner) Replan(ctx context.Context, goal string, failedStep PlanStep, err error, remaining []PlanStep, observation string) (*Plan, error) {
	toolDesc := ""
	for _, t := range r.tools {
		toolDesc += fmt.Sprintf("- %s: %s\n", t.Name(), t.Description())
	}

	remainingJSON, _ := json.Marshal(remaining)

	systemMsg := fmt.Sprintf(`你是一个电脑操控 Agent 的计划修正器。
前面的步骤执行失败了，请修正剩余计划。

可用工具：
%s

规则：
1. 输出 JSON 格式
2. 分析失败根因，换一种方式达成目标
3. 若需用户介入，插入 confirmation_needed 步骤`, toolDesc)

	userMsg := fmt.Sprintf(`用户目标: %s
失败步骤: %s
失败原因: %s
剩余计划: %s
最新观察: %s

输出新的 JSON 计划（覆盖剩余部分）:
{
  "steps": [
    {
      "description": "步骤描述",
      "tool": "工具名",
      "args": { ... },
      "max_retries": 2
    }
  ]
}`,
		goal, failedStep.Description, err.Error(), string(remainingJSON), observation)

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}

	resp, err2 := r.provider.Chat(ctx, messages, nil)
	if err2 != nil {
		return nil, fmt.Errorf("LLM chat: %w", err2)
	}

	// 使用 toolrepair 宽容修复 JSON
	repairResult := toolrepair.RepairJSON(resp.Content)
	if !repairResult.Fixed {
		return nil, fmt.Errorf("parse LLM response: JSON repair failed\nraw: %s", resp.Content)
	}

	var planResp PlanResponse
	if err3 := json.Unmarshal([]byte(repairResult.FixedJSON), &planResp); err3 != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err3)
	}

	// 验证工具名（用 normalizeToolName 归一化，和 Planner 一致）
	for i := range planResp.Steps {
		planResp.Steps[i].Tool = normalizeToolName(r.tools, planResp.Steps[i].Tool)
		if _, ok := r.tools[planResp.Steps[i].Tool]; !ok {
			return nil, fmt.Errorf("unknown tool: %s", planResp.Steps[i].Tool)
		}
		if planResp.Steps[i].MaxRetries == 0 {
			planResp.Steps[i].MaxRetries = 2
		}
		// 修复常见参数问题
		planResp.Steps[i].Args = toolrepair.RepairToolArgs(planResp.Steps[i].Tool, planResp.Steps[i].Args, nil)
	}

	return &Plan{Steps: planResp.Steps}, nil
}
