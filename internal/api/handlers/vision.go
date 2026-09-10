package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"agent/internal/llm"
	"agent/internal/vision"
)

// VisionHandler 视觉分析 API 端点
type VisionHandler struct {
	analyzer     *vision.Analyzer
	logger       *zap.SugaredLogger
	workspaceRoot string
}

// NewVisionHandler 创建视觉分析处理器
func NewVisionHandler(provider llm.Provider, logger *zap.Logger, workspaceRoot string) *VisionHandler {
	return &VisionHandler{
		analyzer:      vision.NewAnalyzer(provider),
		logger:        logger.Sugar(),
		workspaceRoot: workspaceRoot,
	}
}

// validateImagePath 限制 image_path 只能读取工作空间内的文件，
// 防止通过任意路径读取 /etc/passwd 等敏感文件。
func (h *VisionHandler) validateImagePath(p string) error {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(absPath); rerr == nil {
		absPath = resolved
	}
	absWS, _ := filepath.Abs(h.workspaceRoot)
	if resolved, rerr := filepath.EvalSymlinks(absWS); rerr == nil {
		absWS = resolved
	}
	if runtime.GOOS == "windows" {
		absPath = strings.ToLower(absPath)
		absWS = strings.ToLower(absWS)
	}
	rel, err := filepath.Rel(absWS, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("access denied: image_path outside workspace")
	}
	return nil
}

// Routes 注册路由
func (h *VisionHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/analyze", h.Analyze)
	r.Post("/locate", h.Locate)
	r.Post("/screenshot", h.ScreenshotAndAnalyze)
	return r
}

// Analyze 分析指定截图
func (h *VisionHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImagePath string `json:"image_path"`
		Question  string `json:"question,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ImagePath == "" {
		http.Error(w, "image_path is required", http.StatusBadRequest)
		return
	}
	if err := h.validateImagePath(req.ImagePath); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	result, err := h.analyzer.AnalyzeScreenshot(r.Context(), req.ImagePath, req.Question)
	if err != nil {
		h.logger.Warnf("vision analyze failed: %v", err)
		http.Error(w, fmt.Sprintf("analysis failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Locate 在截图中定位元素
func (h *VisionHandler) Locate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImagePath   string `json:"image_path"`
		ElementDesc string `json:"element"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ImagePath == "" || req.ElementDesc == "" {
		http.Error(w, "image_path and element are required", http.StatusBadRequest)
		return
	}
	if err := h.validateImagePath(req.ImagePath); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	result, err := h.analyzer.LocateElement(r.Context(), req.ImagePath, req.ElementDesc)
	if err != nil {
		h.logger.Warnf("vision locate failed: %v", err)
		http.Error(w, fmt.Sprintf("locate failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ScreenshotAndAnalyze 截图并分析（一步完成）
func (h *VisionHandler) ScreenshotAndAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string `json:"question,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// 使用 PowerShell 截图到 artifacts 目录
	artifactsDir := filepath.Join(".", "data", "workspace", "artifacts")
	outputPath := filepath.Join(artifactsDir, fmt.Sprintf("screenshot_%d.png", r.Context().Value("ts")))

	// 简化：直接返回提示用户使用 computer 工具截图
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "请先使用 computer 工具截图，然后调用 /v1/vision/analyze 分析截图",
		"hint":    "POST /v1/tools/run {\"tool\": \"computer\", \"args\": {\"action\": \"screenshot\"}}",
		"path":    outputPath,
	})
}
