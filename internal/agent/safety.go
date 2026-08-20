package agent

import (
	"context"
	"fmt"
	"strings"
)

// safetyTool 提供命令安全分类查询，帮助 Agent/用户预判某命令是否危险。
type safetyTool struct{}

func (t *safetyTool) Name() string        { return "safety" }
func (t *safetyTool) Description() string { return "查询命令的安全分类（只读/读写/破坏性），及列出破坏性命令清单" }
func (t *safetyTool) RequiredLevel() int  { return 0 }

func (t *safetyTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	switch action {
	case "classify":
		command, _ := args["command"].(string)
		if strings.TrimSpace(command) == "" {
			return nil, fmt.Errorf("command is required for classify")
		}
		class := ClassifyCommand(command)
		return &ToolResult{
			Raw:     fmt.Sprintf("命令 [%s] 分类：%s", command, CommandClassLabel(class)),
			Kind:    "text",
			Summary: "class: " + CommandClassLabel(class),
		}, nil
	case "list_destructive":
		return &ToolResult{
			Raw:     "破坏性命令前缀：\n- " + strings.Join(destructiveVerbs, "\n- "),
			Kind:    "text",
			Summary: fmt.Sprintf("%d 个破坏性命令模式", len(destructiveVerbs)),
		}, nil
	default:
		return nil, fmt.Errorf("unknown action: %s (支持 classify|list_destructive)", action)
	}
}
