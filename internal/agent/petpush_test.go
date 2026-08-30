package agent

import (
	"fmt"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPetPush_NormalPush(t *testing.T) {
	// 模拟桌宠 push API
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test123" {
			w.WriteHeader(401)
			return
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// 写 discovery 文件
	dir := t.TempDir()
	cfg := PetPushConfig{Enabled: true, Token: "test123", Port: 0}
	// 从 srv.URL 解析端口
	var port int
	fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port)
	cfg.Port = port
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "pushApi.json"), data, 0o644)

	// 创建 runtime 并触发事件
	eventBus := NewEventBus(nil)
	runtime := &PetPushRuntime{
		eventBus:  eventBus,
		configDir: dir,
		client:    srv.Client(),
		log:       func(level, msg string, args ...any) {},
	}

	err := runtime.onTaskEvent(Event{
		Type:    EventTaskCompleted,
		Payload: "test task completed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// postEvent 在 goroutine 中异步执行，轮询等待送达
	deadline := time.Now().Add(2 * time.Second)
	for received == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if received == nil {
		t.Error("expected to receive event, got nil")
	}
}

func TestPetPush_PortNotListening(t *testing.T) {
	dir := t.TempDir()
	cfg := PetPushConfig{Enabled: true, Token: "test123", Port: 1}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "pushApi.json"), data, 0o644)

	eventBus := NewEventBus(nil)
	runtime := &PetPushRuntime{
		eventBus:  eventBus,
		configDir: dir,
		client:    &http.Client{},
		log:       func(level, msg string, args ...any) {},
	}

	// 端口不通不应 panic
	err := runtime.onTaskEvent(Event{
		Type:    EventTaskFailed,
		Payload: "test error",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPetPush_DisabledOrMissing(t *testing.T) {
	eventBus := NewEventBus(nil)

	// 不存在的目录
	runtime := &PetPushRuntime{
		eventBus:  eventBus,
		configDir: "/nonexistent/path",
		client:    &http.Client{},
		log:       func(level, msg string, args ...any) {},
	}

	// 文件不存在 → 静默返回
	err := runtime.onTaskEvent(Event{Type: EventTaskCompleted})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"你好世界", 2, "你好..."},
	}
	for _, tt := range tests {
		got := truncateRunes(tt.input, tt.max)
		if got != tt.expected {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expected)
		}
	}
}
