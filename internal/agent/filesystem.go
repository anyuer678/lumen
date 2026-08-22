package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemTool 文件系统工具
type FilesystemTool struct {
	workspaceRoot string
	sandbox       bool
}

// NewFilesystemTool 创建文件系统工具
func NewFilesystemTool(workspaceRoot string, sandbox bool) *FilesystemTool {
	return &FilesystemTool{
		workspaceRoot: workspaceRoot,
		sandbox:       sandbox,
	}
}

func (t *FilesystemTool) Name() string        { return "fs" }
func (t *FilesystemTool) Description() string { return "文件系统操作（read/write/list/exists/mkdir/delete/organize）" }
func (t *FilesystemTool) RequiredLevel() int  { return 0 }

func (t *FilesystemTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	path, _ := args["path"].(string)

	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	// 沙箱检查
	if t.sandbox {
		if err := t.checkSandbox(path); err != nil {
			return nil, err
		}
	}

	switch action {
	case "read":
		return t.read(path)
	case "write":
		content, _ := args["content"].(string)
		return t.write(path, content)
	case "list":
		return t.list(path)
	case "exists":
		return t.exists(path)
	case "mkdir":
		return t.mkdir(path)
	case "organize":
		return t.organize(path)
	case "delete":
		return t.delete(path)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// 按扩展名分类目录
var organizeCategories = map[string][]string{
	"图片":  {".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".ico"},
	"文档":  {".doc", ".docx", ".pdf", ".txt", ".md", ".xls", ".xlsx", ".ppt", ".pptx", ".rtf", ".csv"},
	"视频":  {".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v"},
	"音频":  {".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a", ".wma"},
	"压缩包": {".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz"},
	"安装包": {".exe", ".msi", ".dmg", ".pkg", ".apk", ".appx"},
	"代码":  {".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".rs", ".html", ".css", ".json", ".yaml", ".sh", ".bat", ".ps1"},
}

// organize 按扩展名将文件归类到子文件夹
func (t *FilesystemTool) organize(dir string) (*ToolResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("organize: read dir: %w", err)
	}

	// 反查扩展名→分类
	extToCat := make(map[string]string)
	for cat, exts := range organizeCategories {
		for _, e := range exts {
			extToCat[strings.ToLower(e)] = cat
		}
	}

	var moved []string
	var skipped []string
	created := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		cat := extToCat[ext]
		if cat == "" {
			skipped = append(skipped, name)
			continue
		}

		destDir := filepath.Join(dir, cat)
		if !created[cat] {
			if err := os.MkdirAll(destDir, 0755); err != nil {
				return nil, fmt.Errorf("organize: mkdir %s: %w", cat, err)
			}
			created[cat] = true
		}

		destPath := filepath.Join(destDir, name)
		if _, err := os.Stat(destPath); err == nil {
			// 目标已存在，跳过
			skipped = append(skipped, name+" (已存在)")
			continue
		}
		if err := os.Rename(filepath.Join(dir, name), destPath); err != nil {
			return nil, fmt.Errorf("organize: move %s: %w", name, err)
		}
		moved = append(moved, cat+"/"+name)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("整理完成。共移动 %d 个文件，跳过 %d 个。\n", len(moved), len(skipped)))
	if len(moved) > 0 {
		sb.WriteString("移动明细：\n")
		for _, m := range moved {
			sb.WriteString("  - " + m + "\n")
		}
	}
	if len(skipped) > 0 {
		sb.WriteString("跳过（无法归类/已存在）：\n")
		for _, s := range skipped {
			sb.WriteString("  - " + s + "\n")
		}
	}

	return &ToolResult{
		Raw:     sb.String(),
		Kind:    "text",
		Summary: fmt.Sprintf("Organized %d files in %s (%d skipped)", len(moved), dir, len(skipped)),
	}, nil
}

func (t *FilesystemTool) checkSandbox(path string) error {
	// 解析为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// 解析符号链接，防止通过 workspace 内 symlink 逃逸到任意路径
	if resolved, rerr := filepath.EvalSymlinks(absPath); rerr == nil {
		absPath = resolved
	}

	// 检查是否在工作空间内（也解析 workspace 的 symlink）
	workspaceAbs, _ := filepath.Abs(t.workspaceRoot)
	if resolved, rerr := filepath.EvalSymlinks(workspaceAbs); rerr == nil {
		workspaceAbs = resolved
	}

	// 用 path 规范化比较，避免 /a/./b 或大小写绕过
	if !strings.HasPrefix(absPath, workspaceAbs) {
		return fmt.Errorf("access denied: path %s is outside workspace %s", path, t.workspaceRoot)
	}

	return nil
}

func (t *FilesystemTool) read(path string) (*ToolResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return &ToolResult{
		Raw:     string(data),
		Kind:    "text",
		Summary: fmt.Sprintf("Read %d bytes from %s", len(data), path),
	}, nil
}

func (t *FilesystemTool) write(path, content string) (*ToolResult, error) {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	// 输出内容预览（便于验证）
	preview := content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}

	return &ToolResult{
		Raw:     fmt.Sprintf("Written %d bytes to %s\nContent: %s", len(content), path, preview),
		Kind:    "text",
		Summary: fmt.Sprintf("Written %d bytes to %s", len(content), path),
	}, nil
}

func (t *FilesystemTool) list(path string) (*ToolResult, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("list dir: %w", err)
	}

	var lines []string
	for _, entry := range entries {
		info, _ := entry.Info()
		typeChar := "d"
		if !entry.IsDir() {
			typeChar = "-"
		}
		lines = append(lines, fmt.Sprintf("%s %10d %s %s", typeChar, info.Size(), info.ModTime().Format("2006-01-02 15:04"), entry.Name()))
	}

	return &ToolResult{
		Raw:     strings.Join(lines, "\n"),
		Kind:    "text",
		Summary: fmt.Sprintf("Listed %d entries in %s", len(entries), path),
	}, nil
}

func (t *FilesystemTool) exists(path string) (*ToolResult, error) {
	_, err := os.Stat(path)
	exists := err == nil

	return &ToolResult{
		Raw:     fmt.Sprintf("%v", exists),
		Kind:    "text",
		Summary: fmt.Sprintf("Path %s exists: %v", path, exists),
	}, nil
}

func (t *FilesystemTool) mkdir(path string) (*ToolResult, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	return &ToolResult{
		Raw:     fmt.Sprintf("Created directory: %s", path),
		Kind:    "text",
		Summary: fmt.Sprintf("Created directory: %s", path),
	}, nil
}

func (t *FilesystemTool) delete(path string) (*ToolResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("delete: %w", err)
	}

	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return nil, fmt.Errorf("delete dir: %w", err)
		}
		return &ToolResult{
			Raw:     fmt.Sprintf("Deleted directory: %s", path),
			Kind:    "text",
			Summary: fmt.Sprintf("Deleted directory: %s", path),
		}, nil
	}

	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("delete file: %w", err)
	}
	return &ToolResult{
		Raw:     fmt.Sprintf("Deleted file: %s", path),
		Kind:    "text",
		Summary: fmt.Sprintf("Deleted file: %s", path),
	}, nil
}
