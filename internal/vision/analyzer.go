// Package vision 提供截图→视觉模型→UI 理解的能力。
// 灵感来自 OpenClaw 的 media-understanding 和 Reasonix 的 observe→act→verify 循环。
// 核心流程：读取截图文件 → base64 编码 → 发送到视觉模型 → 返回结构化 UI 分析。
package vision

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent/internal/llm"
)

// Analyzer 视觉分析器
type Analyzer struct {
	provider llm.Provider // 支持视觉的 LLM provider
}

// NewAnalyzer 创建视觉分析器
func NewAnalyzer(provider llm.Provider) *Analyzer {
	return &Analyzer{provider: provider}
}

// UIElement 识别出的 UI 元素
type UIElement struct {
	Type    string `json:"type"`              // button/textbox/link/menu/window/icon/...
	Label   string `json:"label"`             // 文本标签
	BBox    string `json:"bbox,omitempty"`    // 位置描述（"x1,y1,x2,y2" 或 "左上角" 等自然语言）
	Notes   string `json:"notes,omitempty"`   // 附加说明
}

// AnalysisResult 视觉分析结果
type AnalysisResult struct {
	Summary    string      `json:"summary"`              // 屏幕整体描述
	Elements  []UIElement `json:"elements,omitempty"`    // 识别出的 UI 元素
	ActiveWindow string   `json:"active_window,omitempty"` // 当前活动窗口
	Suggestion string     `json:"suggestion,omitempty"`  // 建议下一步操作
}

// AnalyzeScreenshot 从文件路径读取截图并发送给视觉模型分析。
// 返回结构化的 UI 理解结果。
func (a *Analyzer) AnalyzeScreenshot(ctx context.Context, imagePath string, question string) (*AnalysisResult, error) {
	if a.provider == nil {
		return nil, fmt.Errorf("vision analyzer: no LLM provider configured")
	}

	// 读取图片并编码为 base64
	dataURI, err := ImageToDataURI(imagePath)
	if err != nil {
		return nil, fmt.Errorf("vision: read image: %w", err)
	}

	// 构建分析 prompt
	prompt := "分析这张屏幕截图的内容。"
	if question != "" {
		prompt = question
	}

	systemMsg := `你是一个精确的计算机屏幕视觉分析器。
分析截图并以 JSON 格式输出结构化结果。
输出格式:
{
  "summary": "屏幕整体描述",
  "elements": [
    {"type": "元素类型", "label": "文本标签", "bbox": "位置描述", "notes": "附加说明"}
  ],
  "active_window": "当前活动窗口标题",
  "suggestion": "建议下一步操作"
}
元素类型包括: button(按钮), textbox(输入框), link(链接), menu(菜单), window(窗口), icon(图标), text(文本), dialog(对话框), toolbar(工具栏)
位置描述用自然语言（如"屏幕中央"、"左上角"、"底部任务栏"等）。
只输出 JSON，不要其他文字。`

	// 构建多模态消息
	msg := llm.NewImageMessage("user", prompt, dataURI)

	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		msg,
	}

	resp, err := a.provider.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("vision: LLM call: %w", err)
	}

	// 解析 JSON 响应
	return ParseAnalysisResponse(resp.Content)
}

// AnalyzeRegion 分析截图的指定区域（裁剪后分析）。
func (a *Analyzer) AnalyzeRegion(ctx context.Context, imagePath, region, question string) (*AnalysisResult, error) {
	if a.provider == nil {
		return nil, fmt.Errorf("vision analyzer: no LLM provider configured")
	}

	dataURI, err := ImageToDataURI(imagePath)
	if err != nil {
		return nil, fmt.Errorf("vision: read image: %w", err)
	}

	prompt := fmt.Sprintf("分析这张截图中位于「%s」区域的内容。", region)
	if question != "" {
		prompt = fmt.Sprintf("分析这张截图中位于「%s」区域的内容。问题：%s", region, question)
	}

	systemMsg := `你是一个精确的计算机屏幕视觉分析器。
分析指定区域的截图内容并以 JSON 格式输出。
输出格式:
{"summary": "描述", "elements": [{"type": "类型", "label": "标签", "bbox": "位置", "notes": "说明"}], "active_window": "窗口", "suggestion": "建议"}
只输出 JSON。`

	msg := llm.NewImageMessage("user", prompt, dataURI)
	messages := []llm.Message{
		{Role: "system", Content: systemMsg},
		msg,
	}

	resp, err := a.provider.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("vision: LLM call: %w", err)
	}

	return ParseAnalysisResponse(resp.Content)
}

// LocateElement 在截图中定位特定元素。
// 返回最可能匹配的元素及其位置描述。
func (a *Analyzer) LocateElement(ctx context.Context, imagePath, elementDesc string) (*UIElement, error) {
	result, err := a.AnalyzeScreenshot(ctx, imagePath,
		fmt.Sprintf("在截图中定位「%s」元素。如果找到，描述它的精确位置和周围环境。如果没找到，说明原因。", elementDesc))
	if err != nil {
		return nil, err
	}

	// 在识别出的元素中查找匹配
	descLower := strings.ToLower(elementDesc)
	for _, el := range result.Elements {
		if strings.Contains(strings.ToLower(el.Label), descLower) ||
			strings.Contains(strings.ToLower(el.Type), descLower) {
			return &el, nil
		}
	}

	// 没有精确匹配，返回建议
	return &UIElement{
		Type:  "unknown",
		Label: elementDesc,
		Notes: result.Suggestion,
	}, nil
}

// ImageToDataURI 读取图片文件并转换为 base64 data URI。
func ImageToDataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	// 根据扩展名确定 MIME 类型
	ext := strings.ToLower(filepath.Ext(path))
	mimeType := "image/png"
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

// ParseAnalysisResponse 从 LLM 响应中解析视觉分析结果。
func ParseAnalysisResponse(content string) (*AnalysisResult, error) {
	content = strings.TrimSpace(content)

	// 去除 markdown 代码块包裹
	if idx := strings.Index(content, "```json"); idx != -1 {
		rest := content[idx+7:]
		if endIdx := strings.Index(rest, "```"); endIdx != -1 {
			content = strings.TrimSpace(rest[:endIdx])
		}
	} else if idx := strings.Index(content, "```"); idx != -1 {
		rest := content[idx+3:]
		if endIdx := strings.Index(rest, "```"); endIdx != -1 {
			content = strings.TrimSpace(rest[:endIdx])
		}
	}

	// 简单的 JSON 解析（不依赖 json.Unmarshal 的严格模式）
	result := &AnalysisResult{}

	// 提取 summary
	if v := extractJSONString(content, "summary"); v != "" {
		result.Summary = v
	}
	// 提取 active_window
	if v := extractJSONString(content, "active_window"); v != "" {
		result.ActiveWindow = v
	}
	// 提取 suggestion
	if v := extractJSONString(content, "suggestion"); v != "" {
		result.Suggestion = v
	}

	return result, nil
}

// extractJSONString 从 JSON 字符串中提取指定字段的值（简化版，避免引入 json 依赖的复杂性）。
func extractJSONString(json, key string) string {
	searchKey := fmt.Sprintf(`"%s":`, key)
	idx := strings.Index(json, searchKey)
	if idx == -1 {
		return ""
	}
	rest := json[idx+len(searchKey):]
	rest = strings.TrimSpace(rest)

	if len(rest) == 0 {
		return ""
	}

	// 字符串值
	if rest[0] == '"' {
		end := strings.Index(rest[1:], `"`)
		if end == -1 {
			return ""
		}
		return rest[1 : end+1]
	}

	// null
	if strings.HasPrefix(rest, "null") {
		return ""
	}

	return ""
}
