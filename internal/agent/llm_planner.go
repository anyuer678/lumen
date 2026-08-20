package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"agent/internal/llm"
	"agent/internal/toolrepair"
)

// LLMPlanner 基于 LLM 的 Planner
type LLMPlanner struct {
	provider llm.Provider
	tools    map[string]Tool
}

// NewLLMPlanner 创建 LLM Planner
func NewLLMPlanner(provider llm.Provider, tools map[string]Tool) *LLMPlanner {
	return &LLMPlanner{
		provider: provider,
		tools:    tools,
	}
}

// PlanResponse LLM 返回的计划格式
type PlanResponse struct {
	Steps          []PlanStep `json:"steps"`
	EstimatedSteps int        `json:"estimated_steps"`
	Risks          []string   `json:"risks"`
}

// Plan 用 LLM 生成执行计划
func (p *LLMPlanner) Plan(ctx context.Context, goal string, memory string) (*Plan, error) {
	// 运行环境
	osName := runtime.GOOS
	if osName == "windows" {
		osName = "Windows（PowerShell / cmd.exe）"
	} else {
		osName = "Linux/macOS"
	}

	// 构建工具描述（含参数示例）
	var toolDesc strings.Builder
	for _, t := range p.tools {
		toolDesc.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
	}

	systemMsg := fmt.Sprintf(`你是一个电脑操控 Agent 的规划器。
你的任务是把用户目标拆解成可执行步骤。

【重要】当前运行环境：%s
命令必须是该平台的命令。例如查磁盘空间用 disk 工具，不要用 df。

可用工具：
%s

工具用法说明（注意：工具名就是上方列出的名称本身）：
- shell.run: args: {"command": "具体命令", "timeout": 30}
  例: {"command": "echo hello", "timeout": 10}
- fs: args: {"action": "list|read|write", "path": "文件路径", "content": "写入内容(可选)"}
  例: {"action": "list", "path": "."}
- browser: args: {"action": "open|read|research|screenshot|title|navigate|click|type|search", "url": "网址", "query": "搜索词(仅search/research)"}
  例: {"action": "open", "url": "https://example.com"}
  例: {"action": "research", "query": "golang news"}
- system: args: {"action": "processes|services|network|disk|git", "target": "目标(可选)"}
  例: {"action": "disk"}
  例: {"action": "processes"}
- windows: args: {"action": "powershell|env|clipboard|notify|app_list|registry|launch|keyboard", "command": "PowerShell命令(仅powershell)", "name": "名称(仅env/notify/launch)"}
  例: {"action": "powershell", "command": "Get-Date"}
  例: {"action": "env", "name": "PATH"}
- subagent: args: {"objective": "子目标描述"}
  例: {"objective": "搜索并总结 Go 语言最新动态"}
- safety: args: {"action": "classify", "command": "要检查的命令"}
  例: {"action": "classify", "command": "del /s test.txt"}

规则：
1. 优先使用专用工具（system.disk 查磁盘），不要用 shell.run 执行系统查询
2. 一步只做一件事，步骤粒度要可验证
3. 若需要用户决策（如删除文件），单独注明 confirmation_needed: true
4. 不得编造工具名：tool 字段只能写上方"可用工具"里出现的名称，子操作放到 args.action
5. 输出严格的 JSON 格式
6. 对于简单任务（如执行一条命令），可以只有 1 个步骤
7. 每个工具的 args 必须包含该工具的必填参数（见上方示例），否则会执行失败`, osName, toolDesc.String())

	if memory != "" {
		systemMsg += "\n\n用户记忆：\n" + memory
	}

	userMsg := fmt.Sprintf(`用户目标: %s

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
}`, goal)

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}

	// 调用 LLM
	resp, err := p.provider.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM chat: %w", err)
	}

	// 解析响应
	return p.parseResponse(resp.Content)
}

// parseResponse 解析 LLM 响应为 Plan（使用 toolrepair 宽容解析）
func (p *LLMPlanner) parseResponse(content string) (*Plan, error) {
	// 使用 toolrepair 宽容修复 JSON（处理截断、尾逗号、markdown 包裹等）
	repairResult := toolrepair.RepairJSON(content)
	if !repairResult.Fixed {
		return nil, fmt.Errorf("parse LLM response: JSON repair failed\nraw: %s", content)
	}
	if len(repairResult.Fixes) > 0 {
		// 记录修复情况（便于调试）
		_ = repairResult.Fixes
	}

	var planResp PlanResponse
	if err := json.Unmarshal([]byte(repairResult.FixedJSON), &planResp); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w\nraw: %s", err, content)
	}

	// 验证工具名 + 修复参数
	for i := range planResp.Steps {
		planResp.Steps[i].Tool = normalizeToolName(p.tools, planResp.Steps[i].Tool)
		if _, ok := p.tools[planResp.Steps[i].Tool]; !ok {
			return nil, fmt.Errorf("unknown tool: %s", planResp.Steps[i].Tool)
		}
		if planResp.Steps[i].MaxRetries == 0 {
			planResp.Steps[i].MaxRetries = 2
		}
		// 修复常见参数问题（timeout 字符串→数字、缺少必填 action 等）
		planResp.Steps[i].Args = toolrepair.RepairToolArgs(planResp.Steps[i].Tool, planResp.Steps[i].Args, nil)
	}

	return &Plan{Steps: planResp.Steps}, nil
}

// normalizeToolName 把 LLM 可能写歪的工具名规整为真实工具名。
func normalizeToolName(tools map[string]Tool, name string) string {
	name = strings.TrimSpace(name)
	if _, ok := tools[name]; ok {
		return name
	}
	// 形如 "windows.powershell" / "system.disk" → 取基础名
	if idx := strings.IndexByte(name, '.'); idx > 0 {
		base := name[:idx]
		if _, ok := tools[base]; ok {
			return base
		}
	}
	return name
}
