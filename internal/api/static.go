package api

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

// StaticHandler 静态文件处理器
func StaticHandler() http.Handler {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}

	indexHTML, err := fs.ReadFile(staticFiles, "static/index.html")
	if err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API 请求不处理
		if strings.HasPrefix(path, "/v1/") {
			http.NotFound(w, r)
			return
		}

		// 根路径返回 index.html
		if path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Write(indexHTML)
			return
		}

		// 尝试查找文件
		filePath := strings.TrimPrefix(path, "/")
		f, err := staticFS.Open(filePath)
		if err != nil {
			// 文件不存在，返回 index.html（SPA 路由）
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Write(indexHTML)
			return
		}
		defer f.Close()

		// 根据文件类型设置 Content-Type 和缓存策略
		contentType := "application/octet-stream"
		if strings.HasSuffix(filePath, ".html") {
			contentType = "text/html; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".css") {
			contentType = "text/css; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".js") {
			contentType = "application/javascript; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".json") {
			contentType = "application/json; charset=utf-8"
		}

		w.Header().Set("Content-Type", contentType)
		// 静态资源文件名带哈希，可以长期缓存
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))

		io.Copy(w, f)
	})
}
