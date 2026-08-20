package vision

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAnalysisResponse(t *testing.T) {
	input := `{
  "summary": "Windows 桌面，显示一个文件资源管理器窗口",
  "elements": [
    {"type": "button", "label": "关闭", "bbox": "右上角", "notes": "窗口关闭按钮"},
    {"type": "textbox", "label": "地址栏", "bbox": "顶部", "notes": "显示路径 C:\\Users"}
  ],
  "active_window": "文件资源管理器",
  "suggestion": "点击地址栏可以修改路径"
}`

	result, err := ParseAnalysisResponse(input)
	if err != nil {
		t.Fatalf("ParseAnalysisResponse error: %v", err)
	}
	if result.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if result.ActiveWindow != "文件资源管理器" {
		t.Errorf("ActiveWindow = %q, want %q", result.ActiveWindow, "文件资源管理器")
	}
	if result.Suggestion == "" {
		t.Error("Suggestion should not be empty")
	}
}

func TestParseAnalysisResponse_MarkdownWrapped(t *testing.T) {
	input := "```json\n{\"summary\": \"test\", \"active_window\": \"window\"}\n```"
	result, err := ParseAnalysisResponse(input)
	if err != nil {
		t.Fatalf("ParseAnalysisResponse error: %v", err)
	}
	if result.Summary != "test" {
		t.Errorf("Summary = %q, want %q", result.Summary, "test")
	}
}

func TestImageToDataURI(t *testing.T) {
	// 创建临时测试图片（最小 PNG）
	tmpDir := t.TempDir()
	pngPath := filepath.Join(tmpDir, "test.png")

	// 1x1 像素 PNG（最小合法 PNG）
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, // IDAT chunk
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, // IEND chunk
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	os.WriteFile(pngPath, pngData, 0644)

	dataURI, err := ImageToDataURI(pngPath)
	if err != nil {
		t.Fatalf("ImageToDataURI error: %v", err)
	}
	if len(dataURI) == 0 {
		t.Error("dataURI should not be empty")
	}
	if dataURI[:22] != "data:image/png;base64," {
		t.Errorf("dataURI prefix = %q, want %q", dataURI[:22], "data:image/png;base64,")
	}
}

func TestExtractJSONString(t *testing.T) {
	tests := []struct {
		json, key, want string
	}{
		{`{"summary": "hello world"}`, "summary", "hello world"},
		{`{"name": null}`, "name", ""},
		{`{"a": "b"}`, "missing", ""},
	}
	for _, tt := range tests {
		got := extractJSONString(tt.json, tt.key)
		if got != tt.want {
			t.Errorf("extractJSONString(%q, %q) = %q, want %q", tt.json, tt.key, got, tt.want)
		}
	}
}
