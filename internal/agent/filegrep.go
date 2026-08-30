package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileGrepTool 文本搜索工具（只读，RequiredLevel=0）
type FileGrepTool struct {
	workspaceRoot string
	sandbox       bool
}

// NewFileGrepTool 创建文本搜索工具
func NewFileGrepTool(workspaceRoot string, sandbox bool) *FileGrepTool {
	return &FileGrepTool{
		workspaceRoot: workspaceRoot,
		sandbox:       sandbox,
	}
}

func (t *FileGrepTool) Name() string        { return "fs.grep" }
func (t *FileGrepTool) Description() string { return "在工作区内正则搜索文本文件，返回匹配行" }
func (t *FileGrepTool) RequiredLevel() int  { return 0 }

// GrepResult 单条搜索结果
type GrepResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// GrepOutput 搜索输出
type GrepOutput struct {
	Results   []GrepResult `json:"results"`
	Truncated bool         `json:"truncated"`
	TotalHits int          `json:"total_hits"`
}

func (t *FileGrepTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	relPath, _ := args["path"].(string)
	maxResults := 50
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	// 解析搜索根目录
	root := t.workspaceRoot
	if relPath != "" {
		root = filepath.Join(t.workspaceRoot, relPath)
	}

	// 沙箱检查
	if t.sandbox {
		absRoot, _ := filepath.Abs(root)
		absWorkspace, _ := filepath.Abs(t.workspaceRoot)
		if !strings.HasPrefix(absRoot, absWorkspace) {
			return nil, fmt.Errorf("path escape: %s is outside workspace", relPath)
		}
	}

	// 遍历搜索
	var results []GrepResult
	totalHits := 0
	truncated := false

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过不可访问的文件
		}

		// 跳过目录和特殊目录
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}

		// 跳过超大文件（>1MB）
		if info.Size() > 1<<20 {
			return nil
		}

		// 跳过二进制文件（简单启发式：检查前 512 字节）
		if isBinary(path) {
			return nil
		}

		// 逐行搜索
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				totalHits++
				if len(results) < maxResults {
					rel, _ := filepath.Rel(t.workspaceRoot, path)
					results = append(results, GrepResult{
						File: rel,
						Line: lineNum,
						Text: strings.TrimSpace(line),
					})
				} else if !truncated {
					truncated = true
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk error: %w", err)
	}

	output := GrepOutput{
		Results:   results,
		Truncated: truncated,
		TotalHits: totalHits,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	summary := fmt.Sprintf("找到 %d 处匹配", totalHits)
	if truncated {
		summary += fmt.Sprintf("（已截断，显示前 %d 条）", maxResults)
	}

	return &ToolResult{
		Raw:     string(data),
		Kind:    "json",
		Summary: summary,
	}, nil
}

// isBinary 简单二进制文件检测
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return true
	}

	// 检查前 512 字节是否包含空字节
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}
