package agent

import (
	"encoding/json"
	"fmt"
	"time"
)

// ParameterDef 参数定义
type ParameterDef struct {
	Type        string   `json:"type"`        // string|number|boolean|array|object
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// ToolDef 工具完整定义
type ToolDef struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Parameters  map[string]ParameterDef `json:"parameters"`
	Required    []string                `json:"required"`
	Permission  int                     `json:"permission"` // 0-3
	Timeout     int                     `json:"timeout"`    // seconds
	SideEffect  bool                    `json:"side_effect"`
	Risk        string                  `json:"risk"`       // low|medium|high
}

// ValidateArgs 验证参数是否符合 schema
func (d *ToolDef) ValidateArgs(args map[string]any) error {
	// 检查必填参数
	for _, req := range d.Required {
		if _, ok := args[req]; !ok {
			return fmt.Errorf("missing required parameter: %s", req)
		}
	}

	// 检查参数类型
	for name, val := range args {
		param, ok := d.Parameters[name]
		if !ok {
			return fmt.Errorf("unknown parameter: %s", name)
		}
		if err := checkType(name, val, param.Type); err != nil {
			return err
		}
	}

	return nil
}

func checkType(name string, val any, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("parameter %s must be string, got %T", name, val)
		}
	case "number":
		switch val.(type) {
		case float64, int, int64:
			// ok
		default:
			return fmt.Errorf("parameter %s must be number, got %T", name, val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("parameter %s must be boolean, got %T", name, val)
		}
	}
	return nil
}

// ToolResultV2 标准化工具结果
type ToolResultV2 struct {
	Success   bool            `json:"success"`
	Error     string          `json:"error,omitempty"`
	Stdout    string          `json:"stdout"`
	Stderr    string          `json:"stderr,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Duration  time.Duration   `json:"duration"`
	ExitCode  *int            `json:"exit_code,omitempty"`
}

// ToJSON 序列化为 JSON
func (r *ToolResultV2) ToJSON() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// 从 ToolResult 转换
func ConvertToV2(result *ToolResult, toolName string, duration time.Duration) *ToolResultV2 {
	v2 := &ToolResultV2{
		Success:  result.Raw != "",
		Stdout:   result.Raw,
		Metadata: map[string]any{"tool": toolName},
		Duration: duration,
	}
	if result.Kind == "error" {
		v2.Success = false
		v2.Error = result.Raw
	}
	return v2
}

// 预定义工具 Schema
var ToolSchemas = map[string]ToolDef{
	"shell.run": {
		Name:        "shell.run",
		Description: "执行 shell 命令",
		Parameters: map[string]ParameterDef{
			"command": {Type: "string", Required: true, Description: "要执行的命令"},
			"timeout": {Type: "number", Default: 30, Description: "超时秒数"},
			"workdir": {Type: "string", Description: "工作目录"},
		},
		Required:   []string{"command"},
		Permission: 1,
		Timeout:    300,
		SideEffect: true,
		Risk:       "medium",
	},
	"fs.read": {
		Name:        "fs.read",
		Description: "读取文件内容",
		Parameters: map[string]ParameterDef{
			"path": {Type: "string", Required: true, Description: "文件路径"},
		},
		Required:   []string{"path"},
		Permission: 0,
		Timeout:    10,
		SideEffect: false,
		Risk:       "low",
	},
	"fs.write": {
		Name:        "fs.write",
		Description: "写入文件",
		Parameters: map[string]ParameterDef{
			"path":    {Type: "string", Required: true, Description: "文件路径"},
			"content": {Type: "string", Required: true, Description: "文件内容"},
		},
		Required:   []string{"path", "content"},
		Permission: 1,
		Timeout:    10,
		SideEffect: true,
		Risk:       "medium",
	},
	"fs.list": {
		Name:        "fs.list",
		Description: "列出目录内容",
		Parameters: map[string]ParameterDef{
			"path": {Type: "string", Required: true, Description: "目录路径"},
		},
		Required:   []string{"path"},
		Permission: 0,
		Timeout:    10,
		SideEffect: false,
		Risk:       "low",
	},
	"fs.grep": {
		Name:        "fs.grep",
		Description: "在工作区内正则搜索文本文件，返回匹配行",
		Parameters: map[string]ParameterDef{
			"pattern":    {Type: "string", Required: true, Description: "正则表达式"},
			"path":       {Type: "string", Description: "搜索相对路径（默认 .）"},
			"max_results": {Type: "number", Default: 50, Description: "最大返回条数"},
		},
		Required:   []string{"pattern"},
		Permission: 0,
		Timeout:    30,
		SideEffect: false,
		Risk:       "low",
	},
	"browser.read": {
		Name:        "browser.read",
		Description: "读取网页内容",
		Parameters: map[string]ParameterDef{
			"url": {Type: "string", Required: true, Description: "网页 URL"},
		},
		Required:   []string{"url"},
		Permission: 0,
		Timeout:    30,
		SideEffect: false,
		Risk:       "low",
	},
	"browser.screenshot": {
		Name:        "browser.screenshot",
		Description: "屏幕截图",
		Parameters:  map[string]ParameterDef{},
		Permission:  0,
		Timeout:     15,
		SideEffect:  false,
		Risk:        "low",
	},
	"system.processes": {
		Name:        "system.processes",
		Description: "列出运行中的进程",
		Parameters:  map[string]ParameterDef{},
		Permission:  1,
		Timeout:     15,
		SideEffect:  false,
		Risk:        "low",
	},
	"system.disk": {
		Name:        "system.disk",
		Description: "查看磁盘空间",
		Parameters:  map[string]ParameterDef{},
		Permission:  1,
		Timeout:     15,
		SideEffect:  false,
		Risk:        "low",
	},
	"system.network": {
		Name:        "system.network",
		Description: "网络诊断（ping）",
		Parameters: map[string]ParameterDef{
			"target": {Type: "string", Default: "baidu.com", Description: "目标地址"},
		},
		Permission: 1,
		Timeout:    15,
		SideEffect: false,
		Risk:       "low",
	},
}

// GetToolSchema 获取工具 schema
func GetToolSchema(name string) (*ToolDef, bool) {
	s, ok := ToolSchemas[name]
	return &s, ok
}

// SchemaToJSON 序列化所有工具 schema
func SchemaToJSON() string {
	b, _ := json.Marshal(ToolSchemas)
	return string(b)
}
