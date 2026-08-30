package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// PetPushConfig 从 desktoppet 的 pushApi.json 读取的发现信息
type PetPushConfig struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
	Port    int    `json:"port"`
}

// PetPushRuntime 桌宠推送运行时：订阅 EventBus，将任务事件推到桌宠气泡
type PetPushRuntime struct {
	eventBus  *EventBus
	configDir string
	client    *http.Client
	log       func(level, msg string, args ...any)
}

// NewPetPushRuntime 创建桌宠推送运行时
func NewPetPushRuntime(eventBus *EventBus, log func(level, msg string, args ...any)) *PetPushRuntime {
	return &PetPushRuntime{
		eventBus:  eventBus,
		configDir: desktoppetConfigDir(),
		client:    &http.Client{Timeout: 2 * time.Second},
		log:       log,
	}
}

// Start 启动推送：订阅 EventBus 事件
func (p *PetPushRuntime) Start() {
	p.eventBus.Subscribe(EventTaskStarted, p.onTaskEvent)
	p.eventBus.Subscribe(EventTaskCompleted, p.onTaskEvent)
	p.eventBus.Subscribe(EventTaskFailed, p.onTaskEvent)
	p.log("info", "[petpush] started, config dir:", p.configDir)
}

// onTaskEvent 处理任务事件，推送到桌宠
func (p *PetPushRuntime) onTaskEvent(event Event) error {
	cfg := p.readConfig()
	if cfg == nil || !cfg.Enabled || cfg.Port == 0 {
		return nil
	}

	var title, message string
	switch event.Type {
	case EventTaskStarted:
		title = "任务开始"
		message = truncateRunes(event.Payload, 50)
	case EventTaskCompleted:
		title = "任务完成"
		message = "任务已完成"
	case EventTaskFailed:
		title = "任务失败"
		message = truncateRunes(event.Payload, 80)
	default:
		return nil
	}

	go p.postEvent(cfg, title, message)
	return nil
}

// postEvent POST 事件到桌宠（fire-and-forget）
func (p *PetPushRuntime) postEvent(cfg *PetPushConfig, title, message string) {
	body := map[string]string{"title": title, "message": message}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("http://127.0.0.1:%d/api/event", cfg.Port)
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := p.client.Do(req)
	if err != nil {
		return // 桌宠未运行，静默跳过
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

// readConfig 读取 desktoppet 的 pushApi.json
func (p *PetPushRuntime) readConfig() *PetPushConfig {
	path := filepath.Join(p.configDir, "pushApi.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg PetPushConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func desktoppetConfigDir() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "DesktopPet")
		}
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "DesktopPet")
	}
	return filepath.Join(home, ".config", "desktop-pet")
}
