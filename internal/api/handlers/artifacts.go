package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ArtifactsHandler 产物管理（列出/访问 workspace 内的文件）
type ArtifactsHandler struct {
	root string
}

// NewArtifactsHandler 创建产物处理器
func NewArtifactsHandler(root string) *ArtifactsHandler {
	return &ArtifactsHandler{root: root}
}

// Routes 注册路由
func (h *ArtifactsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Get("/{name}", h.Serve)
	return r
}

// List 列出产物目录下的文件
func (h *ArtifactsHandler) List(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(h.root, "artifacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"items": []interface{}{}, "count": 0})
		return
	}

	var items []map[string]interface{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, _ := e.Info()
		items = append(items, map[string]interface{}{
			"name":        e.Name(),
			"size":        info.Size(),
			"modified_at": info.ModTime().Format(time.RFC3339),
			"url":         "/v1/artifacts/" + e.Name(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["name"].(string) < items[j]["name"].(string)
	})
	writeJSON(w, 200, map[string]interface{}{"items": items, "count": len(items)})
}

// Serve 提供产物文件（设置正确 Content-Type，支持图片预览）
func (h *ArtifactsHandler) Serve(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, "/\\") {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	path := filepath.Join(h.root, "artifacts", name)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
