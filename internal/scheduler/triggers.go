package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// FileWatcher 文件监听触发器
type FileWatcher struct {
	scheduler *Scheduler
	watcher   *fsnotify.Watcher
	logger    *zap.SugaredLogger
}

// NewFileWatcher 创建文件监听器
func NewFileWatcher(sched *Scheduler, logger *zap.Logger) *FileWatcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Sugar().Errorf("failed to create file watcher: %v", err)
		return nil
	}

	return &FileWatcher{
		scheduler: sched,
		watcher:   watcher,
		logger:    logger.Sugar(),
	}
}

// Start 启动文件监听
func (fw *FileWatcher) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				fw.watcher.Close()
				return
			case event, ok := <-fw.watcher.Events:
				if !ok {
					return
				}
				fw.handleEvent(event)
			case err, ok := <-fw.watcher.Errors:
				if !ok {
					return
				}
				fw.logger.Errorf("file watcher error: %v", err)
			}
		}
	}()
}

// Watch 监听目录
func (fw *FileWatcher) Watch(path string) error {
	return fw.watcher.Add(path)
}

// Stop 停止监听
func (fw *FileWatcher) Stop() {
	fw.watcher.Close()
}

func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	// 查找匹配的 job
	fw.scheduler.mu.RLock()
	defer fw.scheduler.mu.RUnlock()

	for _, job := range fw.scheduler.jobs {
		if !job.Enabled || job.TriggerType != TriggerFileWatch {
			continue
		}

		if job.WatchPath == "" {
			continue
		}

		// 检查路径匹配
		matched, _ := filepath.Match(job.WatchPath, event.Name)
		if !matched {
			// 检查是否在监控目录下
			matched, _ = filepath.Match(filepath.Join(job.WatchPath, "*"), event.Name)
		}

		if matched {
			fw.logger.Infof("file watch triggered: %s (event: %s)", event.Name, event.Op)
			go fw.scheduler.fire(job, map[string]interface{}{
				"file": event.Name,
				"op":   event.Op.String(),
			})
		}
	}
}

// WebhookHandler Webhook 触发器处理器
type WebhookHandler struct {
	scheduler *Scheduler
	logger    *zap.SugaredLogger
}

// NewWebhookHandler 创建 Webhook 处理器
func NewWebhookHandler(sched *Scheduler, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		scheduler: sched,
		logger:    logger.Sugar(),
	}
}

// RegisterRoutes 注册 Webhook 路由
func (h *WebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/webhooks/", h.handleWebhook)
}

func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// 从路径提取 job ID
	// 格式：/v1/webhooks/{jobID}
	jobID := r.URL.Path[len("/v1/webhooks/"):]
	if jobID == "" {
		http.Error(w, "job ID required", http.StatusBadRequest)
		return
	}

	// 查找 job
	h.scheduler.mu.RLock()
	job, ok := h.scheduler.jobs[jobID]
	h.scheduler.mu.RUnlock()

	if !ok || job.TriggerType != TriggerWebhook {
		http.Error(w, "job not found or not webhook type", http.StatusNotFound)
		return
	}

	// 读取请求体
	var payload map[string]interface{}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&payload)
	}

	// 触发 job
	h.logger.Infof("webhook triggered for job: %s", jobID)
	go h.scheduler.fire(job, payload)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "triggered",
		"job_id": jobID,
	})
}
